package app

import (
	"cmp"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	helloTimeout = 5 * time.Second
	readTimeout  = 60 * time.Second // Clients ping every 25 seconds.
	writeTimeout = 8 * time.Second
	maxPending   = 4096
)

// Frame is one realtime message. Notifications carry the delivery sequence.
type Frame struct {
	T   string `json:"t"`
	ID  string `json:"id,omitempty"`
	Seq int64  `json:"seq,omitempty"`
	TS  int64  `json:"ts,omitempty"`
	D   any    `json:"d"`
}

type wire struct {
	T string          `json:"t"`
	D json.RawMessage `json:"d"`
}

func pong() Frame { return Frame{T: "pong", D: map[string]any{"server_time": time.Now().Unix()}} }

// accept upgrades the connection and waits briefly for the client's hello.
func accept(w http.ResponseWriter, r *http.Request) (*Peer, wire, error) {
	var hello wire
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return nil, hello, err
	}
	conn.SetReadLimit(maxBodyBytes)
	p := newPeer(r.Context(), conn)
	deadline, done := context.WithTimeout(p.ctx, helloTimeout)
	defer done()
	if err = wsjson.Read(deadline, conn, &hello); err != nil || hello.T != "hello" {
		p.close()
		return nil, hello, context.Canceled
	}
	return p, hello, nil
}

// readFrames calls handle for every frame until the client goes quiet,
// disconnects, or handle returns false.
func readFrames(p *Peer, handle func(wire) bool) {
	for {
		var frame wire
		ctx, cancel := context.WithTimeout(p.ctx, readTimeout)
		err := wsjson.Read(ctx, p.conn, &frame)
		cancel()
		if err != nil || !handle(frame) {
			return
		}
	}
}

// notify builds the frame that shows message m on the screen of target t.
func (a *App) notify(m Message, t Target) Frame {
	volume := defaultVolume
	if c, exists := a.state.Classes[t.ClassID]; exists {
		volume = c.Volume
	}
	audio := ""
	if clip, exists := a.state.Audio[m.AudioID]; m.Kind == "voice" && exists && clip.UserID == m.SenderID {
		audio = a.audioURL(m.AudioID)
	}
	return Frame{T: "notify", ID: m.ID, Seq: t.Seq, TS: time.Now().Unix(), D: map[string]any{
		"message_id":   m.ID,
		"style":        m.Style,
		"text":         m.Text,
		"student_name": m.StudentName,
		"signature":    m.Signature,
		"class_name":   t.ClassName,
		"audio_url":    audio,
		"chime":        m.Chime,
		"repeat":       m.Repeat,
		"display_sec":  m.DisplaySec,
		"volume":       volume,
		"sent_at":      m.CreatedAt,
		"expires_at":   m.ExpiresAt,
	}}
}

type pendingStatus int

const (
	pendingMissing pendingStatus = iota
	pendingRecalled
	pendingAvailable
)

// pendingForDevice looks up a notification the screen still holds. A normal
// reconnect treats a missing message as recalled; a migration check rejects
// it instead, because its absence may mean the backup is stale.
func (a *App) pendingForDevice(d Device, c Class, messageID string) (Message, Target, pendingStatus) {
	m, exists := a.state.Messages[messageID]
	if !exists {
		return Message{}, Target{}, pendingMissing
	}
	for _, target := range m.Targets {
		if target.DeviceID != d.ID || target.ClassID != d.ClassID {
			continue
		}
		if !m.Live() || !a.licensed(c) {
			return m, target, pendingRecalled
		}
		return m, target, pendingAvailable
	}
	return Message{}, Target{}, pendingMissing
}

// queuedFor lists the notifications the screen has not finished, in order.
func (a *App) queuedFor(d Device) []Frame {
	frames := []Frame{}
	for _, m := range a.state.Messages {
		if !m.Live() {
			continue
		}
		for _, t := range m.Targets {
			if t.DeviceID == d.ID && t.Status.Pending() {
				frames = append(frames, a.notify(m, t))
			}
		}
	}
	slices.SortFunc(frames, func(x, y Frame) int { return cmp.Compare(x.Seq, y.Seq) })
	return frames
}

// teachers lists the active accounts that see the class.
func (a *App) teachers(classID string) []string {
	c, exists := a.state.Classes[classID]
	if !exists {
		return nil
	}
	out := []string{}
	for _, u := range a.state.Users {
		if u.Active() && a.canClass(&u, c) {
			out = append(out, u.ID)
		}
	}
	return out
}

func (a *App) tellTeachers(classID string, f Frame) {
	for _, userID := range a.teachers(classID) {
		a.hub.Teacher(userID, f)
	}
}

func (a *App) receipt(m Message, t Target) {
	a.tellTeachers(t.ClassID, Frame{T: "receipt", D: map[string]any{"message_id": m.ID, "class_id": t.ClassID, "device_id": t.DeviceID, "status": t.Status, "at": now()}})
}

func (a *App) presence(d Device, online bool) {
	a.tellTeachers(d.ClassID, Frame{T: "presence", D: map[string]any{"class_id": d.ClassID, "device_id": d.ID, "online": online}})
}

func (a *App) deviceWS(w http.ResponseWriter, r *http.Request) {
	p, raw, err := accept(w, r)
	if err != nil {
		return
	}
	defer p.close()
	var hello deviceHello
	if json.Unmarshal(raw.D, &hello) != nil || len(hello.PendingIDs) > maxPending {
		return
	}
	d, queued, ok := a.connectDevice(p, hello, a.clientIP(r))
	if !ok {
		return
	}
	go p.write()
	defer a.disconnectDevice(p, d.ID)
	for _, frame := range queued {
		if !p.enqueue(frame) {
			return
		}
	}
	readFrames(p, func(frame wire) bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		current, bound := a.state.Devices[d.ID]
		if !bound || current.TokenHash != d.TokenHash {
			return false
		}
		switch frame.T {
		case "ping":
			current.LastSeenAt = now()
			a.state.Devices[d.ID] = current
			p.send(pong())
		case "ack":
			a.ack(d, frame.D)
		}
		return true
	})
}

// connectDevice authenticates the screen, takes over any older connection,
// records its report and greets it. It returns the notifications to replay.
func (a *App) connectDevice(p *Peer, hello deviceHello, publicIP string) (Device, []Frame, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	d, err := a.deviceFromToken(hello.Token)
	if err != nil {
		_ = p.conn.Close(websocket.StatusPolicyViolation, "invalid device")
		return d, nil, false
	}
	c, exists := a.state.Classes[d.ClassID]
	if !exists {
		return d, nil, false
	}
	previous := d
	d.LastSeenAt = now()
	hello.record(&d, publicIP)
	a.state.Devices[d.ID] = d
	if err = a.Save(); err != nil {
		// The older socket, if any, keeps serving the screen.
		a.state.Devices[d.ID] = previous
		slog.Error("save device connection", "error", err)
		return d, nil, false
	}
	p.ID, p.ClassID = d.ID, d.ClassID
	a.hub.addDevice(p)

	recalled := []string{}
	for _, messageID := range hello.PendingIDs {
		if _, _, status := a.pendingForDevice(d, c, messageID); status != pendingAvailable {
			recalled = append(recalled, messageID)
		}
	}
	welcome := map[string]any{
		"class":             map[string]string{"id": c.ID, "name": c.Name},
		"device":            map[string]string{"id": d.ID, "name": d.Name, "class_id": d.ClassID},
		"volume":            c.Volume,
		"sequence":          d.Sequence,
		"server_time":       time.Now().Unix(),
		"recalled_ids":      recalled,
		"homeworks":         a.homeworkItems(c.ID),
		"homework_revision": c.HomeworkRevision,
	}
	if d.RemoteConfig != nil {
		welcome["remote_config"] = d.RemoteConfig
	}
	p.send(Frame{T: "welcome", D: welcome})
	var queued []Frame
	if a.licensed(c) {
		queued = a.queuedFor(d)
	}
	a.presence(d, true)
	return d, queued, true
}

// disconnectDevice marks the screen offline unless a newer connection took over.
func (a *App) disconnectDevice(p *Peer, deviceID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.hub.removeDevicePeer(p) {
		return
	}
	if d, exists := a.state.Devices[deviceID]; exists {
		d.LastSeenAt = now()
		a.state.Devices[deviceID] = d
		_ = a.Save()
		a.presence(d, false)
	}
}

// ack moves a delivery forward. Played notifications may still be dismissed;
// otherwise a status never goes back.
func (a *App) ack(d Device, raw json.RawMessage) {
	var in struct {
		MessageID string         `json:"message_id"`
		Status    DeliveryStatus `json:"status"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return
	}
	switch in.Status {
	case DeliveryDelivered, DeliveryPlaying, DeliveryPlayed, DeliveryDismissed, DeliveryFailed:
	default:
		return
	}
	m, exists := a.state.Messages[in.MessageID]
	if !exists || m.RecalledAt != nil {
		return
	}
	before := m
	m.Targets = slices.Clone(m.Targets)
	changed := -1
	for i, t := range m.Targets {
		advances := t.Status.progress() < in.Status.progress() || t.Status == DeliveryPlayed && in.Status == DeliveryDismissed
		if t.DeviceID != d.ID || !advances {
			continue
		}
		at := now()
		m.Targets[i].Status = in.Status
		if t.DeliveredAt == nil {
			m.Targets[i].DeliveredAt = &at
		}
		if in.Status == DeliveryPlayed {
			m.Targets[i].PlayedAt = &at
		}
		changed = i
	}
	if changed < 0 {
		return
	}
	a.state.Messages[m.ID] = m
	if err := a.Save(); err != nil {
		a.state.Messages[m.ID] = before
		slog.Error("save receipt", "error", err)
		return
	}
	a.receipt(m, m.Targets[changed])
}

func (a *App) teacherWS(w http.ResponseWriter, r *http.Request) {
	p, raw, err := accept(w, r)
	if err != nil {
		return
	}
	defer p.close()
	var hello struct {
		Token string `json:"token"`
	}
	if json.Unmarshal(raw.D, &hello) != nil {
		return
	}
	// The socket lives only as long as the access token that opened it.
	session, ok := a.connectTeacher(p, hello.Token)
	if !ok {
		return
	}
	go p.write()
	defer a.hub.removeTeacherPeer(p)
	readFrames(p, func(frame wire) bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		// A ping may carry a newer access token for the same account, which keeps
		// the socket open across token refreshes instead of dropping it every 15 minutes.
		if frame.T == "ping" {
			var in struct {
				Token string `json:"token"`
			}
			if json.Unmarshal(frame.D, &in) == nil && in.Token != "" {
				if c, err := a.verify(in.Token); err == nil && c.Subject == session.Subject && c.Expires > session.Expires {
					session = c
					a.hub.renewTeacher(p, c)
				}
			}
		}
		u, exists := a.state.Users[session.Subject]
		valid := exists && u.Active() && u.TokenVersion == session.Version && session.Epoch == a.authEpoch() && time.Now().Unix() < session.Expires
		if valid && frame.T == "ping" {
			p.send(pong())
		}
		return valid
	})
}

func (a *App) connectTeacher(p *Peer, token string) (claims, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, err := a.verify(token)
	u, exists := a.state.Users[c.Subject]
	if err != nil || !exists || !u.Active() || c.Version != u.TokenVersion {
		_ = p.conn.Close(websocket.StatusPolicyViolation, "invalid session")
		return c, false
	}
	p.UserID = u.ID
	p.teacherSession = c
	a.hub.addTeacher(p)
	p.send(Frame{T: "welcome", D: map[string]any{"server_time": time.Now().Unix()}})
	return c, true
}
