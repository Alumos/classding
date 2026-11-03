package app

import (
	"context"
	"encoding/json"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"
)

type Frame struct {
	T   string `json:"t"`
	ID  string `json:"id,omitempty"`
	Seq int64  `json:"seq,omitempty"`
	TS  int64  `json:"ts,omitempty"`
	D   any    `json:"d"`
}
type Peer struct {
	conn                *websocket.Conn
	queue               chan Frame
	ctx                 context.Context
	cancel              context.CancelFunc
	ID, ClassID, UserID string
}

func (p *Peer) close() { p.cancel(); _ = p.conn.CloseNow() }
func (p *Peer) send(f Frame) {
	select {
	case <-p.ctx.Done():
		return
	default:
	}
	select {
	case p.queue <- f:
	default:
		p.close()
	}
}
func (p *Peer) write() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case f := <-p.queue:
			ctx, cancel := context.WithTimeout(p.ctx, 8*time.Second)
			err := wsjson.Write(ctx, p.conn, f)
			cancel()
			if err != nil {
				p.close()
				return
			}
		}
	}
}

type Hub struct {
	mu       sync.Mutex
	devices  map[string]*Peer
	teachers map[string]map[*Peer]bool
}

func NewHub() *Hub { return &Hub{devices: map[string]*Peer{}, teachers: map[string]map[*Peer]bool{}} }
func (h *Hub) DeviceOnline(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	p := h.devices[id]
	return p != nil && p.ctx.Err() == nil
}
func (h *Hub) DeviceCount() int { h.mu.Lock(); defer h.mu.Unlock(); return len(h.devices) }
func (h *Hub) Device(id string, f Frame) {
	h.mu.Lock()
	p := h.devices[id]
	h.mu.Unlock()
	if p != nil {
		p.send(f)
	}
}
func (h *Hub) Class(id string, f Frame) {
	h.mu.Lock()
	peers := []*Peer{}
	for _, p := range h.devices {
		if p.ClassID == id {
			peers = append(peers, p)
		}
	}
	h.mu.Unlock()
	for _, p := range peers {
		p.send(f)
	}
}
func (h *Hub) Teacher(id string, f Frame) {
	h.mu.Lock()
	peers := []*Peer{}
	for p := range h.teachers[id] {
		peers = append(peers, p)
	}
	h.mu.Unlock()
	for _, p := range peers {
		p.send(f)
	}
}
func (h *Hub) RemoveDevice(id string) {
	h.mu.Lock()
	p := h.devices[id]
	delete(h.devices, id)
	h.mu.Unlock()
	if p != nil {
		p.close()
	}
}
func (h *Hub) RemoveTeacher(id string) {
	h.mu.Lock()
	peers := h.teachers[id]
	delete(h.teachers, id)
	h.mu.Unlock()
	for p := range peers {
		p.close()
	}
}
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, p := range h.devices {
		p.close()
	}
	for _, peers := range h.teachers {
		for p := range peers {
			p.close()
		}
	}
	h.devices = map[string]*Peer{}
	h.teachers = map[string]map[*Peer]bool{}
}
func (a *App) notify(m Message, t Target) Frame {
	volume := 75
	if c, exists := a.state.Classes[t.ClassID]; exists {
		volume = c.Volume
	}
	audio := ""
	if clip, exists := a.state.Audio[m.AudioID]; m.Kind == "voice" && exists && clip.UserID == m.SenderID {
		audio = a.audioURL(m.AudioID)
	}
	return Frame{T: "notify", ID: m.ID, Seq: t.Seq, TS: time.Now().Unix(), D: map[string]any{"message_id": m.ID, "style": m.Style, "text": m.Text, "student_name": m.StudentName, "signature": m.Signature, "class_name": t.ClassName, "audio_url": audio, "chime": m.Chime, "repeat": m.Repeat, "display_sec": m.DisplaySec, "volume": volume, "sent_at": m.CreatedAt, "expires_at": m.ExpiresAt}}
}
func (a *App) teacherRecipients(classID string) []string {
	c, exists := a.state.Classes[classID]
	if !exists {
		return nil
	}
	out := []string{}
	for _, u := range a.state.Users {
		if u.Status == "active" && a.canClass(&u, c) {
			out = append(out, u.ID)
		}
	}
	return out
}
func (a *App) receipt(m Message, t Target) {
	f := Frame{T: "receipt", D: map[string]any{"message_id": m.ID, "class_id": t.ClassID, "device_id": t.DeviceID, "status": t.Status, "at": now()}}
	for _, uid := range a.teacherRecipients(t.ClassID) {
		a.hub.Teacher(uid, f)
	}
}
func (a *App) presence(d Device, online bool) {
	f := Frame{T: "presence", D: map[string]any{"class_id": d.ClassID, "device_id": d.ID, "online": online}}
	for _, uid := range a.teacherRecipients(d.ClassID) {
		a.hub.Teacher(uid, f)
	}
}

type wire struct {
	T string          `json:"t"`
	D json.RawMessage `json:"d"`
}

func accept(w http.ResponseWriter, r *http.Request) (*Peer, wire, error) {
	var hello wire
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return nil, hello, err
	}
	conn.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(r.Context())
	p := &Peer{conn: conn, queue: make(chan Frame, 256), ctx: ctx, cancel: cancel}
	deadline, done := context.WithTimeout(ctx, 5*time.Second)
	defer done()
	if err = wsjson.Read(deadline, conn, &hello); err != nil || hello.T != "hello" {
		p.close()
		return nil, hello, context.Canceled
	}
	return p, hello, nil
}
func (a *App) deviceWS(w http.ResponseWriter, r *http.Request) {
	p, hello, err := accept(w, r)
	if err != nil {
		return
	}
	defer p.close()
	var in struct {
		Token      string   `json:"token"`
		LastSeq    int64    `json:"last_seq"`
		PendingIDs []string `json:"pending_ids"`
	}
	if json.Unmarshal(hello.D, &in) != nil || len(in.PendingIDs) > 4096 {
		return
	}
	a.mu.Lock()
	d, err := a.deviceFromToken(in.Token)
	if err != nil {
		a.mu.Unlock()
		_ = p.conn.Close(websocket.StatusPolicyViolation, "invalid device")
		return
	}
	c, exists := a.state.Classes[d.ClassID]
	if !exists {
		a.mu.Unlock()
		return
	}
	p.ID = d.ID
	p.ClassID = d.ClassID
	a.hub.mu.Lock()
	old := a.hub.devices[d.ID]
	a.hub.devices[d.ID] = p
	a.hub.mu.Unlock()
	if old != nil {
		old.close()
	}
	d.LastSeenAt = now()
	a.state.Devices[d.ID] = d
	if err = a.Save(); err != nil {
		a.hub.RemoveDevice(d.ID)
		a.mu.Unlock()
		return
	}
	recalled := []string{}
	for _, key := range in.PendingIDs {
		m, ok := a.state.Messages[key]
		allowed := false
		for _, target := range m.Targets {
			if target.DeviceID == d.ID {
				allowed = true
			}
		}
		if !ok || !allowed || m.RecalledAt != nil || elapsed(m.ExpiresAt) || !a.licensed(c) {
			recalled = append(recalled, key)
		}
	}
	p.send(Frame{T: "welcome", D: map[string]any{"class": map[string]string{"id": c.ID, "name": c.Name}, "volume": c.Volume, "server_time": time.Now().Unix(), "recalled_ids": recalled}})
	pending := []Frame{}
	if a.licensed(c) {
		for _, m := range a.state.Messages {
			if m.RecalledAt != nil || elapsed(m.ExpiresAt) {
				continue
			}
			for _, t := range m.Targets {
				if t.DeviceID == d.ID && (t.Status == "queued" || t.Status == "delivered" || t.Status == "playing") {
					pending = append(pending, a.notify(m, t))
				}
			}
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].Seq < pending[j].Seq })
	a.presence(d, true)
	a.mu.Unlock()
	go p.write()
	defer func() {
		a.mu.Lock()
		a.hub.mu.Lock()
		current := a.hub.devices[d.ID]
		if current == p {
			delete(a.hub.devices, d.ID)
		}
		a.hub.mu.Unlock()
		if current == p {
			if currentD, exists := a.state.Devices[d.ID]; exists {
				currentD.LastSeenAt = now()
				a.state.Devices[d.ID] = currentD
				_ = a.Save()
				a.presence(currentD, false)
			}
		}
		a.mu.Unlock()
	}()
	for _, frame := range pending {
		select {
		case p.queue <- frame:
		case <-p.ctx.Done():
			return
		case <-time.After(8 * time.Second):
			return
		}
	}
	for {
		var frame wire
		ctx, cancel := context.WithTimeout(p.ctx, 60*time.Second)
		err = wsjson.Read(ctx, p.conn, &frame)
		cancel()
		if err != nil {
			return
		}
		a.mu.Lock()
		current, stillBound := a.state.Devices[d.ID]
		if !stillBound || current.TokenHash != d.TokenHash {
			a.mu.Unlock()
			return
		}
		if frame.T == "ping" {
			current.LastSeenAt = now()
			a.state.Devices[d.ID] = current
			p.send(Frame{T: "pong", D: map[string]any{"server_time": time.Now().Unix()}})
		} else if frame.T == "ack" {
			a.ack(d, frame.D)
		}
		a.mu.Unlock()
	}
}
func (a *App) ack(d Device, raw json.RawMessage) {
	var in struct {
		MessageID string `json:"message_id"`
		Status    string `json:"status"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return
	}
	rank := map[string]int{"queued": 0, "delivered": 1, "playing": 2, "played": 3, "dismissed": 3, "failed": 3, "expired": 3}
	next, allowed := rank[in.Status]
	if !allowed || in.Status == "queued" || in.Status == "expired" {
		return
	}
	m, exists := a.state.Messages[in.MessageID]
	if !exists || m.RecalledAt != nil {
		return
	}
	before := m
	before.Targets = append([]Target(nil), m.Targets...)
	changed := -1
	for i, t := range m.Targets {
		if t.DeviceID != d.ID || rank[t.Status] >= next {
			continue
		}
		stamp := now()
		m.Targets[i].Status = in.Status
		if t.DeliveredAt == nil {
			m.Targets[i].DeliveredAt = &stamp
		}
		if in.Status == "played" {
			m.Targets[i].PlayedAt = &stamp
		}
		changed = i
	}
	if changed >= 0 {
		a.state.Messages[m.ID] = m
		if err := a.Save(); err != nil {
			a.state.Messages[m.ID] = before
			slog.Error("save receipt", "error", err)
			return
		}
		a.receipt(m, m.Targets[changed])
	}
}
func (a *App) teacherWS(w http.ResponseWriter, r *http.Request) {
	p, hello, err := accept(w, r)
	if err != nil {
		return
	}
	defer p.close()
	var in struct {
		Token string `json:"token"`
	}
	if json.Unmarshal(hello.D, &in) != nil {
		return
	}
	a.mu.Lock()
	claims, err := a.verify(in.Token)
	u, exists := a.state.Users[claims.Subject]
	if err != nil || !exists || u.Status != "active" || claims.Version != u.TokenVersion || u.Role != "user" && !claims.Admin {
		a.mu.Unlock()
		_ = p.conn.Close(websocket.StatusPolicyViolation, "invalid session")
		return
	}
	p.UserID = u.ID
	a.hub.mu.Lock()
	if a.hub.teachers[u.ID] == nil {
		a.hub.teachers[u.ID] = map[*Peer]bool{}
	}
	a.hub.teachers[u.ID][p] = true
	a.hub.mu.Unlock()
	p.send(Frame{T: "welcome", D: map[string]any{"server_time": time.Now().Unix()}})
	a.mu.Unlock()
	go p.write()
	defer func() {
		a.hub.mu.Lock()
		delete(a.hub.teachers[u.ID], p)
		if len(a.hub.teachers[u.ID]) == 0 {
			delete(a.hub.teachers, u.ID)
		}
		a.hub.mu.Unlock()
	}()
	for {
		var frame wire
		ctx, cancel := context.WithTimeout(p.ctx, 60*time.Second)
		err = wsjson.Read(ctx, p.conn, &frame)
		cancel()
		if err != nil {
			return
		}
		a.mu.Lock()
		current, exists := a.state.Users[u.ID]
		valid := exists && current.Status == "active" && current.TokenVersion == claims.Version && time.Now().Unix() < claims.Expires
		a.mu.Unlock()
		if !valid {
			return
		}
		if frame.T == "ping" {
			p.send(Frame{T: "pong", D: map[string]any{"server_time": time.Now().Unix()}})
		}
	}
}
