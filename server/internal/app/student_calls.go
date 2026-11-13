package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const studentCallCooldown = 30 * time.Second

var studentCallRequestID = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

func (a *App) createStudentCall(w http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	d, c, err := a.boundDevice(r)
	if err != nil {
		return nil, err
	}
	if !a.licensed(c) {
		return nil, licenseInactive()
	}
	var in struct {
		RequestID string `json:"request_id"`
	}
	if err := decodeStrict(r, &in); err != nil {
		return nil, err
	}
	if !studentCallRequestID.MatchString(in.RequestID) {
		return nil, bad("request_id 需为 1–128 位字母、数字或 _ . : -。")
	}
	// Keys are globally reserved, not scoped to a device or a class. Authenticate
	// and validate the current binding/license before returning even a replay.
	for _, item := range a.state.StudentCalls {
		if item.RequestID == in.RequestID {
			if item.DeviceID != d.ID || item.ClassID != c.ID {
				return nil, fail(409, "REQUEST_ID_CONFLICT", "request_id 已被使用，请使用新的请求标识。")
			}
			return item.StudentCall, nil
		}
	}
	at := time.Now()
	for _, item := range a.state.StudentCalls {
		created, valid := parseTime(item.CreatedAt)
		if item.ClassID == c.ID && valid && at.Sub(created) < studentCallCooldown {
			remaining := studentCallCooldown - at.Sub(created)
			seconds := (remaining + time.Second - 1) / time.Second
			w.Header().Set("Retry-After", strconv.FormatInt(int64(seconds), 10))
			return nil, tooMany("该班刚刚发起过呼叫，请稍后再试。")
		}
	}
	item := studentCallRecord{StudentCall: StudentCall{ID: id("sc"), ClassID: c.ID, ClassName: c.Name, DeviceID: d.ID, DeviceName: d.Name, CreatedAt: stamp(at)}, RequestID: in.RequestID}
	a.state.StudentCalls[item.ID] = item
	a.studentCallChanged(item, "created")
	return item.StudentCall, nil
}

type studentCallCursor struct {
	CreatedAt string `json:"created_at"`
	ID        string `json:"id"`
}

type studentCallList struct {
	Items       []StudentCall `json:"items"`
	UnreadCount int           `json:"unread_count"`
	NextCursor  *string       `json:"next_cursor"`
}

// Compare parsed times rather than RFC3339Nano strings: a whole second and a
// fractional second do not sort lexically at the boundary.
func compareStudentCalls(x, y StudentCall) int {
	left, _ := parseTime(x.CreatedAt)
	right, _ := parseTime(y.CreatedAt)
	if order := right.Compare(left); order != 0 {
		return order
	}
	return strings.Compare(y.ID, x.ID)
}

func (a *App) listStudentCalls(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	query := r.URL.Query()
	classID := query.Get("class_id")
	if classID != "" {
		if _, err := a.getClass(u, classID, false); err != nil {
			return nil, err
		}
	}
	limit := 100
	if raw, exists := query["limit"]; exists {
		var err error
		if len(raw) != 1 {
			return nil, bad("limit 需为 1–100。")
		}
		limit, err = strconv.Atoi(raw[0])
		if err != nil || limit < 1 || limit > 100 {
			return nil, bad("limit 需为 1–100。")
		}
	}
	var cursor *StudentCall
	if raw := query.Get("cursor"); raw != "" {
		if len(raw) > 1024 {
			return nil, bad("分页标识无效。")
		}
		data, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil {
			return nil, bad("分页标识无效。")
		}
		var key studentCallCursor
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&key) != nil || key.ID == "" || len(key.ID) > 128 {
			return nil, bad("分页标识无效。")
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			return nil, bad("分页标识无效。")
		}
		if _, valid := parseTime(key.CreatedAt); !valid {
			return nil, bad("分页标识无效。")
		}
		cursor = &StudentCall{ID: key.ID, CreatedAt: key.CreatedAt}
	}
	result := studentCallList{Items: []StudentCall{}}
	for _, item := range a.state.StudentCalls {
		c, exists := a.state.Classes[item.ClassID]
		if !exists || classID != "" && classID != item.ClassID || !a.canClass(u, c) {
			continue
		}
		if item.ReadAt == "" {
			result.UnreadCount++
		}
		if cursor == nil || compareStudentCalls(item.StudentCall, *cursor) > 0 {
			result.Items = append(result.Items, item.StudentCall)
		}
	}
	slices.SortFunc(result.Items, compareStudentCalls)
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
		last := result.Items[len(result.Items)-1]
		key, _ := json.Marshal(studentCallCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		encoded := base64.RawURLEncoding.EncodeToString(key)
		result.NextCursor = &encoded
	}
	return result, nil
}

func (a *App) readStudentCall(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	item, exists := a.state.StudentCalls[chi.URLParam(r, "id")]
	if !exists {
		return nil, missing()
	}
	if _, err := a.getClass(u, item.ClassID, false); err != nil {
		return nil, err
	}
	// A read is a shared acknowledgement. The first processor is authoritative;
	// retries and other teachers must not overwrite them or broadcast again.
	if item.ReadAt == "" {
		item.ReadAt, item.ReadBy, item.ReadByName = now(), u.ID, u.DisplayName
		a.state.StudentCalls[item.ID] = item
		a.audit(u, "student_call.read", item.ID)
		a.studentCallChanged(item, "read")
	}
	return item.StudentCall, nil
}

// broadcastStudentCall runs with App.mu held. A connected socket is not proof
// of authorization: its token may expire or be retired before the next ping.
func (a *App) broadcastStudentCall(classID string, frame Frame) {
	var peers []*Peer
	a.hub.mu.Lock()
	for _, uid := range a.teachers(classID) {
		u := a.state.Users[uid]
		for p := range a.hub.teachers[uid] {
			session := p.teacherSession
			if session.Subject == uid && session.Expires > time.Now().Unix() && session.Version == u.TokenVersion && session.Epoch == a.authEpoch() {
				peers = append(peers, p)
			}
		}
	}
	a.hub.mu.Unlock()
	sendAll(peers, frame)
}

func (a *App) studentCallChanged(item studentCallRecord, kind string) {
	a.deferJob(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		// Recheck live membership after commit, not at enqueue time: a teacher may
		// have left or been disabled, or the class may have been deleted meanwhile.
		current, exists := a.state.StudentCalls[item.ID]
		if !exists || current.ClassID != item.ClassID {
			return
		}
		a.broadcastStudentCall(item.ClassID, Frame{T: "student_call", D: map[string]any{"id": item.ID, "class_id": item.ClassID, "kind": kind}})
	})
}
