package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
)

const (
	maxClassHomeworks = 12
	// JSON clients represent revisions as numbers, so keep them exactly representable.
	maxHomeworkRevision int64 = 1<<53 - 1
)

// homeworkItems always returns a complete, stable snapshot, including hidden work.
// Callers hold a.mu.
func (a *App) homeworkItems(classID string) []Homework {
	items := []Homework{}
	for _, item := range a.state.Homeworks {
		if item.ClassID == classID {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].AlwaysOnTop != items[j].AlwaysOnTop {
			return items[i].AlwaysOnTop
		}
		if items[i].CreatedAt != items[j].CreatedAt {
			left, _ := time.Parse(time.RFC3339Nano, items[i].CreatedAt)
			right, _ := time.Parse(time.RFC3339Nano, items[j].CreatedAt)
			if !left.Equal(right) {
				return left.After(right)
			}
		}
		return items[i].ID < items[j].ID
	})
	return items
}

// JSON null is not a valid value for an optional patch field; it must not be
// silently treated as if that field had been omitted.
func decodeHomework(r *http.Request, dst any) error {
	var raw json.RawMessage
	if err := decode(r, &raw); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return bad("请求格式不正确。")
	}
	for _, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return bad("作业字段不能为 null。")
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(dst) != nil {
		return bad("请求格式不正确。")
	}
	return nil
}

func validHomeworkText(subject, text string) bool {
	return utf8.ValidString(subject) && utf8.ValidString(text) &&
		strings.TrimSpace(subject) != "" && strings.TrimSpace(text) != "" &&
		utf8.RuneCountInString(subject) <= 24 && utf8.RuneCountInString(text) <= 300
}

func (a *App) listHomeworks(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	classID := r.URL.Query().Get("class_id")
	if classID == "" {
		return nil, bad("请选择班级。")
	}
	if _, err := a.getClass(u, classID, false); err != nil {
		return nil, err
	}
	return map[string]any{"items": a.homeworkItems(classID)}, nil
}

func (a *App) deviceHomeworks(_ http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	d, err := a.deviceFromToken(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if err != nil {
		return nil, err
	}
	c, exists := a.state.Classes[d.ClassID]
	if !exists {
		return nil, fail(401, "DEVICE_REVOKED", "班级已被删除。")
	}
	return map[string]any{"items": a.homeworkItems(c.ID), "homework_revision": c.HomeworkRevision}, nil
}

type homeworkInput struct {
	ClassID     string  `json:"class_id"`
	Subject     *string `json:"subject"`
	Text        *string `json:"text"`
	Visible     *bool   `json:"visible"`
	AlwaysOnTop *bool   `json:"always_on_top"`
}

func (a *App) createHomework(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in homeworkInput
	if err := decodeHomework(r, &in); err != nil {
		return nil, err
	}
	if in.ClassID == "" || in.Subject == nil || in.Text == nil || in.Visible == nil || in.AlwaysOnTop == nil {
		return nil, bad("请完整填写班级、科目、作业内容和显示选项。")
	}
	c, err := a.getClass(u, in.ClassID, false)
	if err != nil {
		return nil, err
	}
	subject, text := strings.TrimSpace(*in.Subject), strings.TrimSpace(*in.Text)
	if !validHomeworkText(subject, text) {
		return nil, bad("科目需为 1–24 字，作业内容需为 1–300 字。")
	}
	if len(a.homeworkItems(c.ID)) >= maxClassHomeworks {
		return nil, fail(409, "HOMEWORK_LIMIT_REACHED", "每个班级最多保留 12 条作业。")
	}
	stamp := now()
	item := Homework{ID: id("hw"), ClassID: c.ID, Subject: subject, Text: text, TeacherID: u.ID, TeacherName: u.DisplayName, Visible: *in.Visible, AlwaysOnTop: *in.AlwaysOnTop, CreatedAt: stamp, UpdatedAt: stamp}
	a.state.Homeworks[item.ID] = item
	if err := a.homeworkChanged(c.ID); err != nil {
		return nil, err
	}
	a.audit(u, "homework.create", item.ID)
	return item, nil
}

func (a *App) managedHomework(u *User, itemID string) (Homework, error) {
	item, exists := a.state.Homeworks[itemID]
	if !exists {
		return Homework{}, missing()
	}
	c, err := a.getClass(u, item.ClassID, false)
	if err != nil {
		return Homework{}, err
	}
	if item.TeacherID != u.ID && !a.owns(u, c) {
		return Homework{}, forbidden()
	}
	return item, nil
}

func (a *App) updateHomework(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	item, err := a.managedHomework(u, chi.URLParam(r, "id"))
	if err != nil {
		return nil, err
	}
	var in struct {
		Subject     *string `json:"subject"`
		Text        *string `json:"text"`
		Visible     *bool   `json:"visible"`
		AlwaysOnTop *bool   `json:"always_on_top"`
	}
	if err = decodeHomework(r, &in); err != nil {
		return nil, err
	}
	if in.Subject == nil && in.Text == nil && in.Visible == nil && in.AlwaysOnTop == nil {
		return nil, bad("请选择要修改的作业内容。")
	}
	if in.Subject != nil {
		item.Subject = strings.TrimSpace(*in.Subject)
	}
	if in.Text != nil {
		item.Text = strings.TrimSpace(*in.Text)
	}
	if !validHomeworkText(item.Subject, item.Text) {
		return nil, bad("科目需为 1–24 字，作业内容需为 1–300 字。")
	}
	if in.Visible != nil {
		item.Visible = *in.Visible
	}
	if in.AlwaysOnTop != nil {
		item.AlwaysOnTop = *in.AlwaysOnTop
	}
	item.UpdatedAt = now()
	a.state.Homeworks[item.ID] = item
	if err := a.homeworkChanged(item.ClassID); err != nil {
		return nil, err
	}
	a.audit(u, "homework.update", item.ID)
	return item, nil
}

func (a *App) deleteHomework(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	item, err := a.managedHomework(u, chi.URLParam(r, "id"))
	if err != nil {
		return nil, err
	}
	delete(a.state.Homeworks, item.ID)
	if err := a.homeworkChanged(item.ClassID); err != nil {
		return nil, err
	}
	a.audit(u, "homework.delete", item.ID)
	return ok(), nil
}

// Called while the transaction is locked; broadcasting is deferred until save
// succeeds. Building and queuing under a.mu prevents older snapshots overtaking
// newer ones when several requests commit concurrently.
func (a *App) homeworkChanged(classID string) error {
	c := a.state.Classes[classID]
	if c.HomeworkRevision >= maxHomeworkRevision {
		return fail(409, "HOMEWORK_REVISION_EXHAUSTED", "作业修订号已达到上限，请联系管理员。")
	}
	c.HomeworkRevision++
	a.state.Classes[classID] = c
	a.deferJob(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		c, exists := a.state.Classes[classID]
		if !exists {
			return
		}
		frame := Frame{T: "homework", D: map[string]any{"items": a.homeworkItems(classID), "homework_revision": c.HomeworkRevision}}
		a.hub.Class(classID, frame)
		for _, userID := range a.teacherRecipients(classID) {
			a.hub.Teacher(userID, frame)
		}
	})
	return nil
}
