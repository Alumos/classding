package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/go-chi/chi/v5"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
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

func (a *App) sendMessage(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in messageInput
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	return a.createMessage(u, in)
}
func (a *App) createMessage(u *User, in messageInput) (Message, error) {
	var zero Message
	if len(in.ClassIDs) == 0 || len(in.ClassIDs) > 20 {
		return zero, bad("请选择 1–20 个班级。")
	}
	if in.Kind != "text" && in.Kind != "voice" {
		return zero, bad("通知类型不正确。")
	}
	if in.Style != "notice" && in.Style != "praise" && in.Style != "urgent" {
		return zero, bad("通知样式不正确。")
	}
	if in.DisplaySec < 15 || in.DisplaySec > 600 || in.Repeat < 1 || in.Repeat > 3 {
		return zero, bad("显示时长需为 15–600 秒，朗读次数为 1–3 遍。")
	}
	if in.Style != "urgent" {
		in.Chime = false
	}
	in.Text = strings.TrimSpace(in.Text)
	if utf8.RuneCountInString(in.Text) > 200 || utf8.RuneCountInString(in.StudentName) > 24 || utf8.RuneCountInString(in.Signature) > 24 {
		return zero, bad("正文最多 200 字，姓名与落款最多 24 字。")
	}
	if in.Kind == "text" && in.AudioID != "" {
		return zero, bad("文字通知不能携带原声录音。")
	}
	if in.Kind == "text" && (in.Text == "" || in.PlayMode != "tts") {
		return zero, bad("请输入文字通知内容。")
	}
	if in.Kind == "voice" {
		audio, exists := a.state.Audio[in.AudioID]
		if !exists || audio.UserID != u.ID || in.PlayMode != "voice" {
			return zero, bad("请先上传自己的录音。")
		}
	}
	if blocked := os.Getenv("SENSITIVE_WORDS"); blocked != "" {
		for _, word := range strings.Split(blocked, ",") {
			word = strings.TrimSpace(word)
			if word != "" && strings.Contains(strings.ToLower(in.Text+in.StudentName), strings.ToLower(word)) {
				return zero, bad("通知含有不可发送的内容，请修改。")
			}
		}
	}
	classes := []Class{}
	seen := map[string]bool{}
	for _, key := range in.ClassIDs {
		if seen[key] {
			continue
		}
		seen[key] = true
		c, err := a.getClass(u, key, false)
		if err != nil {
			return zero, err
		}
		if !a.licensed(c) {
			return zero, fail(403, "LICENSE_INACTIVE", "班级授权已失效，请联系管理员。")
		}
		if last, exists := a.lastSent[key]; exists && time.Since(last) < a.config.SendCooldown {
			return zero, fail(429, "SEND_TOO_FAST", "同一班级请间隔 5 秒再发送。")
		}
		classes = append(classes, c)
	}
	if in.Signature == "" {
		in.Signature = u.DisplayName
	}
	m := Message{ID: id("m"), Kind: in.Kind, Text: in.Text, StudentName: strings.TrimSpace(in.StudentName), Signature: strings.TrimSpace(in.Signature), Style: in.Style, DisplaySec: in.DisplaySec, Repeat: in.Repeat, Chime: in.Chime, PlayMode: in.PlayMode, SenderID: u.ID, CreatedAt: now(), ExpiresAt: after(10 * time.Minute), Targets: []Target{}, AudioID: in.AudioID}
	for _, c := range classes {
		count := 0
		for _, d := range a.state.Devices {
			if d.ClassID == c.ID {
				count++
				m.Targets = append(m.Targets, Target{ClassID: c.ID, ClassName: c.Name, DeviceID: d.ID, DeviceName: d.Name, Status: "queued", Seq: a.nextSeq(d.ID)})
			}
		}
		if count == 0 {
			m.Targets = append(m.Targets, Target{ClassID: c.ID, ClassName: c.Name, Status: "queued"})
		}
		a.lastSent[c.ID] = time.Now()
	}
	a.state.Messages[m.ID] = m
	a.audit(u, "message.send", m.ID)
	for _, t := range m.Targets {
		if t.DeviceID != "" {
			deviceID, frame := t.DeviceID, a.notify(m, t)
			a.deferJob(func() { a.hub.Device(deviceID, frame) })
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
func (a *App) bindPending(d Device) {
	messages := []Message{}
	for _, m := range a.state.Messages {
		if m.RecalledAt == nil && !elapsed(m.ExpiresAt) {
			messages = append(messages, m)
		}
	}
	sort.Slice(messages, func(i, j int) bool { return messages[i].CreatedAt < messages[j].CreatedAt })
	for _, m := range messages {
		for i, t := range m.Targets {
			if t.ClassID == d.ClassID && t.DeviceID == "" && t.Status == "queued" {
				m.Targets[i].DeviceID = d.ID
				m.Targets[i].DeviceName = d.Name
				m.Targets[i].Seq = a.nextSeq(d.ID)
				a.state.Messages[m.ID] = m
			}
		}
	}
}
func (a *App) messageView(m Message, u *User) (Message, bool) {
	visible := []Target{}
	for _, t := range m.Targets {
		if c, exists := a.state.Classes[t.ClassID]; exists && a.canClass(u, c) {
			visible = append(visible, t)
		}
	}
	m.Targets = visible
	m.AudioID = ""
	return m, len(visible) > 0
}
func (a *App) listMessages(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	return a.messageList(u, r.URL.Query().Get("class_id"), r.URL.Query().Get("cursor")), nil
}
func (a *App) messageList(u *User, classID, cursor string) any {
	out := []Message{}
	for _, m := range a.state.Messages {
		visible, ok := a.messageView(m, u)
		if !ok {
			continue
		}
		if cursor != "" && m.CreatedAt >= cursor {
			continue
		}
		if classID != "" {
			filtered := []Target{}
			for _, t := range visible.Targets {
				if t.ClassID == classID {
					filtered = append(filtered, t)
				}
			}
			if len(filtered) == 0 {
				continue
			}
			visible.Targets = filtered
		}
		out = append(out, visible)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	var cursorOut *string
	if len(out) > 30 {
		out = out[:30]
		stamp := out[29].CreatedAt
		cursorOut = &stamp
	}
	return map[string]any{"items": out, "next_cursor": cursorOut}
}
func (a *App) getMessage(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	m, exists := a.state.Messages[chi.URLParam(r, "id")]
	if !exists {
		return nil, missing()
	}
	view, visible := a.messageView(m, u)
	if !visible {
		return nil, forbidden()
	}
	return view, nil
}
func (a *App) recallMessage(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	m, exists := a.state.Messages[chi.URLParam(r, "id")]
	if !exists {
		return nil, missing()
	}
	for _, t := range m.Targets {
		c, exists := a.state.Classes[t.ClassID]
		if !exists || !a.canClass(u, c) || m.SenderID != u.ID && !a.owns(u, c) {
			return nil, forbidden()
		}
	}
	if m.RecalledAt != nil {
		return m, nil
	}
	stamp := now()
	m.RecalledAt = &stamp
	for i := range m.Targets {
		if m.Targets[i].Status != "played" {
			m.Targets[i].Status = "dismissed"
		}
	}
	a.state.Messages[m.ID] = m
	a.audit(u, "message.recall", m.ID)
	copy := m
	a.deferJob(func() {
		for _, t := range copy.Targets {
			a.hub.Device(t.DeviceID, Frame{T: "recall", D: map[string]any{"message_id": copy.ID}})
		}
	})
	return m, nil
}
func (a *App) resendMessage(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	m, exists := a.state.Messages[chi.URLParam(r, "id")]
	if !exists {
		return nil, missing()
	}
	if m.RecalledAt != nil {
		return nil, bad("已撤回通知不可重发，请重新编辑。")
	}
	classIDs := []string{}
	seen := map[string]bool{}
	for _, t := range m.Targets {
		if !seen[t.ClassID] {
			classIDs = append(classIDs, t.ClassID)
			seen[t.ClassID] = true
		}
	}
	return a.createMessage(u, messageInput{classIDs, m.Kind, m.Text, m.StudentName, m.Signature, m.Style, m.DisplaySec, m.Repeat, m.Chime, m.PlayMode, m.AudioID})
}
func (a *App) listPhrases(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	out := []Phrase{}
	for _, p := range a.state.Phrases {
		if p.UserID == u.ID {
			p.UserID = ""
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sort < out[j].Sort })
	return out, nil
}
func (a *App) addPhrase(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in struct{ Text string }
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(strings.TrimSpace(in.Text)) < 1 || utf8.RuneCountInString(in.Text) > 200 {
		return nil, bad("短语需为 1–200 字。")
	}
	count := 0
	for _, p := range a.state.Phrases {
		if p.UserID == u.ID {
			count++
		}
	}
	if count >= 30 {
		return nil, bad("最多保存 30 条快捷短语。")
	}
	p := Phrase{id("p"), u.ID, strings.TrimSpace(in.Text), count}
	a.state.Phrases[p.ID] = p
	return p, nil
}
func (a *App) editPhrase(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	p, exists := a.state.Phrases[chi.URLParam(r, "id")]
	if !exists {
		return nil, missing()
	}
	if p.UserID != u.ID {
		return nil, forbidden()
	}
	var in struct{ Text string }
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(strings.TrimSpace(in.Text)) < 1 || utf8.RuneCountInString(in.Text) > 200 {
		return nil, bad("短语需为 1–200 字。")
	}
	p.Text = strings.TrimSpace(in.Text)
	a.state.Phrases[p.ID] = p
	return p, nil
}
func (a *App) deletePhrase(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	p, exists := a.state.Phrases[chi.URLParam(r, "id")]
	if !exists {
		return nil, missing()
	}
	if p.UserID != u.ID {
		return nil, forbidden()
	}
	delete(a.state.Phrases, p.ID)
	return ok(), nil
}
func (a *App) tool(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	c, err := a.getClass(u, chi.URLParam(r, "id"), false)
	if err != nil {
		return nil, err
	}
	if !a.licensed(c) {
		return nil, fail(403, "LICENSE_INACTIVE", "班级授权已失效。")
	}
	if !a.limit("tool:"+u.ID+":"+c.ID, 20, time.Minute) {
		return nil, fail(429, "RATE_LIMITED", "工具操作过于频繁。")
	}
	parts := strings.Split(r.URL.Path, "/")
	name := parts[len(parts)-1]
	params := map[string]any{}
	if name != "stop" {
		if err = decode(r, &params); err != nil {
			return nil, err
		}
	}
	if name == "countdown" {
		seconds, ok := params["seconds"].(float64)
		if !ok || seconds < 1 || seconds > 7200 || seconds != float64(int(seconds)) {
			return nil, bad("倒计时需为 1–7200 秒整数。")
		}
	}
	if name == "rollcall" {
		names, ok := params["names"].([]any)
		if !ok || len(names) == 0 || len(names) > 100 {
			return nil, bad("请输入 1–100 位同学姓名。")
		}
		for _, entry := range names {
			value, ok := entry.(string)
			if !ok || utf8.RuneCountInString(strings.TrimSpace(value)) < 1 || utf8.RuneCountInString(value) > 24 {
				return nil, bad("姓名需为 1–24 字。")
			}
		}
	}
	a.audit(u, "tool."+name, c.ID)
	a.deferJob(func() { a.hub.Class(c.ID, Frame{T: "tool", D: map[string]any{"name": name, "params": params}}) })
	return ok(), nil
}
func (a *App) audioSignature(audioID, expires string) string {
	mac := hmac.New(sha256.New, []byte(a.config.JWTSecret))
	mac.Write([]byte(audioID + ":" + expires))
	return hex.EncodeToString(mac.Sum(nil))
}
func (a *App) audioURL(audioID string) string {
	expires := strconv.FormatInt(time.Now().Add(20*time.Minute).Unix(), 10)
	return "/api/v1/audio/" + audioID + "?expires=" + expires + "&signature=" + a.audioSignature(audioID, expires)
}
func (a *App) uploadAudio(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if !a.limit("upload:"+u.ID, 20, time.Hour) {
		return nil, fail(429, "RATE_LIMITED", "录音上传过于频繁。")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		return nil, bad("录音不能超过 8 MB。")
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, bad("请选择录音文件。")
	}
	defer file.Close()
	mime := strings.Split(header.Header.Get("Content-Type"), ";")[0]
	allowed := map[string]string{"audio/webm": ".webm", "audio/mp4": ".m4a", "audio/mpeg": ".mp3", "audio/ogg": ".ogg", "audio/wav": ".wav", "video/webm": ".webm"}
	ext, exists := allowed[mime]
	if !exists {
		return nil, bad("仅支持 webm、mp4、mp3、ogg 或 wav 录音。")
	}
	audio := Audio{id("audio"), u.ID, "", mime, now()}
	audio.Path = filepath.Join(a.config.DataDir, "audio", audio.ID+ext)
	out, err := os.OpenFile(audio.Path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	size, err := io.Copy(out, io.LimitReader(file, 8<<20+1))
	closeErr := out.Close()
	if err != nil || closeErr != nil || size == 0 || size > 8<<20 {
		os.Remove(audio.Path)
		return nil, bad("录音为空或超过 8 MB。")
	}
	a.deferRollback(func() {
		if err := os.Remove(audio.Path); err != nil {
			slog.Error("remove rolled-back audio", "error", err)
		}
	})
	a.state.Audio[audio.ID] = audio
	return map[string]string{"audio_id": audio.ID, "url": a.audioURL(audio.ID)}, nil
}
func (a *App) downloadAudio(w http.ResponseWriter, r *http.Request) {
	audioID := chi.URLParam(r, "id")
	expiry := r.URL.Query().Get("expires")
	expires, err := strconv.ParseInt(expiry, 10, 64)
	if err != nil || time.Now().Unix() > expires || expires > time.Now().Add(21*time.Minute).Unix() || !hmac.Equal([]byte(r.URL.Query().Get("signature")), []byte(a.audioSignature(audioID, expiry))) {
		writeError(w, forbidden())
		return
	}
	a.mu.Lock()
	audio, exists := a.state.Audio[audioID]
	a.mu.Unlock()
	if !exists {
		writeError(w, missing())
		return
	}
	w.Header().Set("Content-Type", audio.MIME)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filepath.Base(audio.Path)))
	w.Header().Set("Cache-Control", "private, max-age=60")
	http.ServeFile(w, r, audio.Path)
}
