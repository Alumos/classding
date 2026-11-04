package app

import (
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	messageTTL      = 10 * time.Minute
	messagePageSize = 30
	maxMessageClass = 20
)

type messageInput struct {
	ClassIDs    []string `json:"class_ids"`
	Kind        string   `json:"kind"`
	Text        string   `json:"text"`
	StudentName string   `json:"student_name"`
	Signature   string   `json:"signature"`
	Style       string   `json:"style"`
	DisplaySec  int      `json:"display_sec"`
	Repeat      int      `json:"repeat"`
	Chime       bool     `json:"chime"`
	PlayMode    string   `json:"play_mode"`
	AudioID     string   `json:"audio_id"`
}

// validate checks the notification itself; class access is checked separately.
func (in *messageInput) validate(a *App, u *User) error {
	if len(in.ClassIDs) == 0 || len(in.ClassIDs) > maxMessageClass {
		return bad("请选择 1–20 个班级。")
	}
	if in.Kind != "text" && in.Kind != "voice" {
		return bad("通知类型不正确。")
	}
	if in.Style != "notice" && in.Style != "praise" && in.Style != "urgent" {
		return bad("通知样式不正确。")
	}
	if !inRange(in.DisplaySec, 15, 600) || !inRange(in.Repeat, 1, 3) {
		return bad("显示时长需为 15–600 秒，朗读次数为 1–3 遍。")
	}
	if in.Style != "urgent" {
		in.Chime = false
	}
	in.Text = strings.TrimSpace(in.Text)
	if !runesBetween(in.Text, 0, maxMessageText) || !runesBetween(in.StudentName, 0, maxDisplayName) || !runesBetween(in.Signature, 0, maxDisplayName) {
		return bad("正文最多 200 字，姓名与落款最多 24 字。")
	}
	switch in.Kind {
	case "text":
		if in.AudioID != "" {
			return bad("文字通知不能携带原声录音。")
		}
		if in.Text == "" || in.PlayMode != "tts" {
			return bad("请输入文字通知内容。")
		}
	case "voice":
		if audio, exists := a.state.Audio[in.AudioID]; !exists || audio.UserID != u.ID || in.PlayMode != "voice" {
			return bad("请先上传自己的录音。")
		}
	}
	if blockedWord(in.Text + in.StudentName) {
		return bad("通知含有不可发送的内容，请修改。")
	}
	return nil
}

// blockedWord checks text against the comma-separated SENSITIVE_WORDS setting.
func blockedWord(text string) bool {
	text = strings.ToLower(text)
	for word := range strings.SplitSeq(os.Getenv("SENSITIVE_WORDS"), ",") {
		if word = strings.ToLower(strings.TrimSpace(word)); word != "" && strings.Contains(text, word) {
			return true
		}
	}
	return false
}

func (a *App) sendMessage(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in messageInput
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	return a.createMessage(u, in)
}

// createMessage queues one delivery per screen in every target class. A class
// without screens gets a placeholder that the next screen to bind picks up.
func (a *App) createMessage(u *User, in messageInput) (Message, error) {
	if err := in.validate(a, u); err != nil {
		return Message{}, err
	}
	var classes []Class
	for _, classID := range unique(in.ClassIDs) {
		c, err := a.usableClass(u, classID, false)
		if err != nil {
			return Message{}, err
		}
		if last, sent := a.lastSent[classID]; sent && time.Since(last) < a.config.SendCooldown {
			return Message{}, fail(429, "SEND_TOO_FAST", "同一班级请间隔 5 秒再发送。")
		}
		classes = append(classes, c)
	}
	if in.Signature == "" {
		in.Signature = u.DisplayName
	}
	m := Message{
		ID: id("m"), Kind: in.Kind, Text: in.Text,
		StudentName: strings.TrimSpace(in.StudentName), Signature: strings.TrimSpace(in.Signature),
		Style: in.Style, DisplaySec: in.DisplaySec, Repeat: in.Repeat, Chime: in.Chime, PlayMode: in.PlayMode,
		SenderID: u.ID, CreatedAt: now(), ExpiresAt: after(messageTTL), Targets: []Target{}, AudioID: in.AudioID,
	}
	for _, c := range classes {
		screens := 0
		for _, d := range a.state.Devices {
			if d.ClassID == c.ID {
				screens++
				m.Targets = append(m.Targets, Target{ClassID: c.ID, ClassName: c.Name, DeviceID: d.ID, DeviceName: d.Name, Status: DeliveryQueued, Seq: a.nextSeq(d.ID)})
			}
		}
		if screens == 0 {
			m.Targets = append(m.Targets, Target{ClassID: c.ID, ClassName: c.Name, Status: DeliveryQueued})
		}
		a.lastSent[c.ID] = time.Now()
	}
	a.state.Messages[m.ID] = m
	a.audit(u, "message.send", m.ID)
	for _, t := range m.Targets {
		if t.DeviceID != "" {
			frame := a.notify(m, t)
			a.deferJob(func() { a.hub.Device(t.DeviceID, frame) })
		}
	}
	return m, nil
}

func (a *App) nextSeq(deviceID string) int64 {
	device := a.state.Devices[deviceID]
	device.Sequence++
	a.state.Devices[deviceID] = device
	return device.Sequence
}

// bindPending hands placeholder deliveries of the class to a newly bound screen.
func (a *App) bindPending(d Device) {
	var live []Message
	for _, m := range a.state.Messages {
		if m.Live() {
			live = append(live, m)
		}
	}
	slices.SortFunc(live, func(x, y Message) int { return strings.Compare(x.CreatedAt, y.CreatedAt) })
	for _, m := range live {
		for i, t := range m.Targets {
			if t.ClassID == d.ClassID && t.DeviceID == "" && t.Status == DeliveryQueued {
				m.Targets[i].DeviceID, m.Targets[i].DeviceName, m.Targets[i].Seq = d.ID, d.Name, a.nextSeq(d.ID)
				a.state.Messages[m.ID] = m
			}
		}
	}
}

// messageView keeps only the deliveries in classes the viewer may see.
func (a *App) messageView(m Message, visible func(Class) bool) (Message, bool) {
	m.Targets = slices.DeleteFunc(slices.Clone(m.Targets), func(t Target) bool {
		c, exists := a.state.Classes[t.ClassID]
		return !exists || !visible(c)
	})
	m.AudioID = ""
	return m, len(m.Targets) > 0
}

// memberOf matches the classes the account belongs to.
func (a *App) memberOf(u *User) func(Class) bool {
	return func(c Class) bool { return a.canClass(u, c) }
}

// governedBy matches the classes an administrator manages.
func governedBy(u *User) func(Class) bool {
	return func(c Class) bool { return u.Governs(c.OrgID) }
}

type messagePage struct {
	Items      []Message `json:"items"`
	NextCursor *string   `json:"next_cursor"`
}

// messageList pages through visible messages, newest first. The cursor is the
// creation time of the last message on the previous page.
func (a *App) messageList(visible func(Class) bool, classID, cursor string) messagePage {
	out := []Message{}
	for _, m := range a.state.Messages {
		if cursor != "" && m.CreatedAt >= cursor {
			continue
		}
		view, shown := a.messageView(m, visible)
		if !shown {
			continue
		}
		if classID != "" {
			view.Targets = slices.DeleteFunc(view.Targets, func(t Target) bool { return t.ClassID != classID })
			if len(view.Targets) == 0 {
				continue
			}
		}
		out = append(out, view)
	}
	slices.SortFunc(out, func(x, y Message) int { return strings.Compare(y.CreatedAt, x.CreatedAt) })
	page := messagePage{Items: out}
	if len(out) > messagePageSize {
		page.Items = out[:messagePageSize]
		next := page.Items[messagePageSize-1].CreatedAt
		page.NextCursor = &next
	}
	return page
}

func (a *App) listMessages(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	return a.messageList(a.memberOf(u), r.URL.Query().Get("class_id"), r.URL.Query().Get("cursor")), nil
}

func (a *App) getMessage(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	m, exists := a.state.Messages[chi.URLParam(r, "id")]
	if !exists {
		return nil, missing()
	}
	view, visible := a.messageView(m, a.memberOf(u))
	if !visible {
		return nil, forbidden()
	}
	return view, nil
}

// managedMessage loads a message the account may recall or delete: its sender,
// or the owner of every class it went to.
func (a *App) managedMessage(u *User, messageID string) (Message, error) {
	m, exists := a.state.Messages[messageID]
	if !exists {
		return m, missing()
	}
	if len(m.Targets) == 0 && m.SenderID != u.ID {
		return m, forbidden()
	}
	for _, t := range m.Targets {
		c, exists := a.state.Classes[t.ClassID]
		if !exists || !a.canClass(u, c) || m.SenderID != u.ID && !a.owns(u, c) {
			return m, forbidden()
		}
	}
	return m, nil
}

func (a *App) recallMessage(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	m, err := a.managedMessage(u, chi.URLParam(r, "id"))
	if err != nil || m.RecalledAt != nil {
		return m, err
	}
	recalled := now()
	m.RecalledAt = &recalled
	for i := range m.Targets {
		if m.Targets[i].Status != DeliveryPlayed {
			m.Targets[i].Status = DeliveryDismissed
		}
	}
	a.state.Messages[m.ID] = m
	a.audit(u, "message.recall", m.ID)
	a.recallFrom(m.ID, m.Targets)
	return m, nil
}

func (a *App) resendMessage(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	m, exists := a.state.Messages[chi.URLParam(r, "id")]
	if !exists {
		return nil, missing()
	}
	if m.RecalledAt != nil {
		return nil, bad("已撤回通知不可重发，请重新编辑。")
	}
	return a.createMessage(u, messageInput{
		ClassIDs: m.ClassIDs(), Kind: m.Kind, Text: m.Text, StudentName: m.StudentName, Signature: m.Signature,
		Style: m.Style, DisplaySec: m.DisplaySec, Repeat: m.Repeat, Chime: m.Chime, PlayMode: m.PlayMode, AudioID: m.AudioID,
	})
}

func (a *App) deleteMessage(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	m, err := a.managedMessage(u, chi.URLParam(r, "id"))
	if err != nil {
		return nil, err
	}
	a.removeMessage(u, m, "message.delete")
	return ok(), nil
}

// adminDeleteMessage removes a message sent to classes the administrator
// governs. Messages to deleted classes are left to the operator.
func (a *App) adminDeleteMessage(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	m, exists := a.state.Messages[chi.URLParam(r, "id")]
	if !exists {
		return nil, missing()
	}
	if len(m.Targets) == 0 && !u.IsSuperAdmin() {
		return nil, forbidden()
	}
	for _, t := range m.Targets {
		c, exists := a.state.Classes[t.ClassID]
		if exists && !u.Governs(c.OrgID) || !exists && !u.IsSuperAdmin() {
			return nil, forbidden()
		}
	}
	a.removeMessage(u, m, "admin.message.delete")
	return ok(), nil
}

// removeMessage deletes the record and recalls deliveries that have not
// finished, so a deleted notification never appears on a screen afterwards.
func (a *App) removeMessage(u *User, m Message, action string) {
	delete(a.state.Messages, m.ID)
	a.audit(u, action, m.ID)
	if m.RecalledAt == nil {
		a.recallFrom(m.ID, slices.DeleteFunc(slices.Clone(m.Targets), func(t Target) bool { return !t.Status.Pending() }))
	}
}

// recallFrom tells the screens of the deliveries to drop the message.
func (a *App) recallFrom(messageID string, targets []Target) {
	frame := Frame{T: "recall", D: map[string]any{"message_id": messageID}}
	a.deferJob(func() {
		for _, t := range targets {
			a.hub.Device(t.DeviceID, frame)
		}
	})
}

func (a *App) listPhrases(_ http.ResponseWriter, _ *http.Request, u *User) (any, error) {
	out := []Phrase{}
	for _, p := range a.state.Phrases {
		if p.UserID == u.ID {
			p.UserID = ""
			out = append(out, p)
		}
	}
	slices.SortFunc(out, func(x, y Phrase) int { return x.Sort - y.Sort })
	return out, nil
}

func decodePhrase(r *http.Request) (string, error) {
	var in struct {
		Text string `json:"text"`
	}
	if err := decode(r, &in); err != nil {
		return "", err
	}
	text := strings.TrimSpace(in.Text)
	if text == "" || !runesBetween(in.Text, 1, maxPhraseText) {
		return "", bad("短语需为 1–200 字。")
	}
	return text, nil
}

func (a *App) addPhrase(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	text, err := decodePhrase(r)
	if err != nil {
		return nil, err
	}
	count := 0
	for _, p := range a.state.Phrases {
		if p.UserID == u.ID {
			count++
		}
	}
	if count >= maxPhrases {
		return nil, bad("最多保存 30 条快捷短语。")
	}
	p := Phrase{ID: id("p"), UserID: u.ID, Text: text, Sort: count}
	a.state.Phrases[p.ID] = p
	return p, nil
}

func (a *App) ownPhrase(u *User, phraseID string) (Phrase, error) {
	p, exists := a.state.Phrases[phraseID]
	if !exists {
		return p, missing()
	}
	if p.UserID != u.ID {
		return p, forbidden()
	}
	return p, nil
}

func (a *App) editPhrase(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	p, err := a.ownPhrase(u, chi.URLParam(r, "id"))
	if err != nil {
		return nil, err
	}
	if p.Text, err = decodePhrase(r); err != nil {
		return nil, err
	}
	a.state.Phrases[p.ID] = p
	return p, nil
}

func (a *App) deletePhrase(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	p, err := a.ownPhrase(u, chi.URLParam(r, "id"))
	if err != nil {
		return nil, err
	}
	delete(a.state.Phrases, p.ID)
	return ok(), nil
}
