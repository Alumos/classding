package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestTeacherClassGradeIsBounded(t *testing.T) {
	f := newSecurityFixture(t)
	f.app.state.Codes["NEW-CODE"] = Code{Code: "NEW-CODE", Status: LicenseUnused, MaxTeachers: 2, MaxDevices: 1}
	body := map[string]any{"name": "新班", "grade": strings.Repeat("年", maxClassName+1), "activation_code": "NEW-CODE"}
	expectStatus(t, f.request(t, "POST", "/api/v1/classes", f.token("owner"), body, nil), 400)
	body["grade"] = "  三年级  "
	w := f.request(t, "POST", "/api/v1/classes", f.token("owner"), body, nil)
	expectStatus(t, w, 200)
	var view ClassView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil || view.Grade != "三年级" {
		t.Fatalf("grade = %q, %v", view.Grade, err)
	}
}

func TestRecallKeepsFinishedDeliveryOutcomes(t *testing.T) {
	f := newSecurityFixture(t)
	m := f.app.state.Messages["message_a"]
	m.Targets = []Target{
		{ClassID: "class_a", DeviceID: "d1", Status: DeliveryDelivered},
		{ClassID: "class_a", DeviceID: "d2", Status: DeliveryFailed},
		{ClassID: "class_a", DeviceID: "d3", Status: DeliveryExpired},
		{ClassID: "class_a", DeviceID: "d4", Status: DeliveryPlayed},
	}
	f.app.state.Messages[m.ID] = m
	expectStatus(t, f.request(t, "POST", "/api/v1/messages/message_a/recall", f.token("owner"), nil, nil), 200)
	want := []DeliveryStatus{DeliveryDismissed, DeliveryFailed, DeliveryExpired, DeliveryPlayed}
	for i, target := range f.app.state.Messages[m.ID].Targets {
		if target.Status != want[i] {
			t.Fatalf("target %d = %s, want %s", i, target.Status, want[i])
		}
	}
}

func TestMessagePagesDoNotSkipTiedTimestamps(t *testing.T) {
	f := newSecurityFixture(t)
	for id := range f.app.state.Messages {
		delete(f.app.state.Messages, id)
	}
	at := now()
	for i := range messagePageSize + 5 {
		id := "tied_" + string(rune('a'+i/26)) + string(rune('a'+i%26))
		f.app.state.Messages[id] = Message{ID: id, Kind: "text", SenderID: "owner_a", CreatedAt: at, ExpiresAt: after(time.Hour), Targets: []Target{{ClassID: "class_a"}}}
	}
	seen := map[string]bool{}
	cursor := ""
	for range 3 {
		path := "/api/v1/messages"
		if cursor != "" {
			path += "?cursor=" + cursor
		}
		w := f.request(t, "GET", path, f.token("owner"), nil, nil)
		expectStatus(t, w, 200)
		var page messagePage
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Fatalf("%s repeated", item.ID)
			}
			seen[item.ID] = true
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	if len(seen) != messagePageSize+5 {
		t.Fatalf("saw %d messages, want %d", len(seen), messagePageSize+5)
	}
	expectStatus(t, f.request(t, "GET", "/api/v1/messages?cursor=bad", f.token("owner"), nil, nil), 400)
}

func TestSweepAppliesRetentionToCallsAuditAndInvites(t *testing.T) {
	f := newSecurityFixture(t)
	when := time.Now()
	old, fresh := stamp(when.Add(-200*24*time.Hour)), stamp(when)
	f.app.state.StudentCalls["old_call"] = studentCallRecord{StudentCall: StudentCall{ID: "old_call", ClassID: "class_a", CreatedAt: old}, RequestID: "r-old"}
	f.app.state.StudentCalls["new_call"] = studentCallRecord{StudentCall: StudentCall{ID: "new_call", ClassID: "class_a", CreatedAt: fresh}, RequestID: "r-new"}
	f.app.state.Audit["old_audit"] = Audit{ID: "old_audit", At: old}
	f.app.state.Audit["new_audit"] = Audit{ID: "new_audit", At: fresh}
	f.app.state.Invites["USED"] = Invite{Code: "USED", ClassID: "class_a", ExpiresAt: after(time.Hour), Used: true}
	f.app.state.Invites["GONE"] = Invite{Code: "GONE", ClassID: "class_a", ExpiresAt: stamp(when.Add(-time.Minute))}
	f.app.state.Invites["OPEN"] = Invite{Code: "OPEN", ClassID: "class_a", ExpiresAt: after(time.Hour)}
	f.app.mu.Lock()
	err := f.app.maintenanceSweep(when)
	f.app.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	s := f.app.state
	_, oldCall := s.StudentCalls["old_call"]
	_, newCall := s.StudentCalls["new_call"]
	_, oldAudit := s.Audit["old_audit"]
	_, newAudit := s.Audit["new_audit"]
	if oldCall || !newCall || oldAudit || !newAudit || len(s.Invites) != 1 || s.Invites["OPEN"].Code == "" {
		t.Fatalf("retention left calls=%v audit=%v invites=%v", s.StudentCalls, s.Audit, s.Invites)
	}
}

func TestLoginVerifiesOutsideLockAndRejectsUnknownAccounts(t *testing.T) {
	f := newSecurityFixture(t)
	u := f.app.state.Users["owner_a"]
	u.PasswordHash = passwordHash("Password12345")
	f.app.state.Users[u.ID] = u
	login := func(username, password string) int {
		return f.request(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{"username": username, "password": password}, nil).Code
	}
	if code := login("owner_a", "Password12345"); code != 200 {
		t.Fatalf("valid login = %d", code)
	}
	if code := login("owner_a", "Wrong123456"); code != 401 {
		t.Fatalf("wrong password = %d", code)
	}
	if code := login("nobody", "Password12345"); code != 401 {
		t.Fatalf("unknown account = %d", code)
	}
}

func TestSendCooldownMessageNamesConfiguredSeconds(t *testing.T) {
	f := newSecurityFixture(t)
	f.app.config.SendCooldown = 8 * time.Second
	body := map[string]any{"class_ids": []string{"class_a"}, "kind": "text", "text": "请安静", "style": "notice", "display_sec": 30, "repeat": 1, "play_mode": "tts"}
	expectStatus(t, f.request(t, "POST", "/api/v1/messages", f.token("owner"), body, nil), 200)
	w := f.request(t, "POST", "/api/v1/messages", f.token("owner"), body, nil)
	expectStatus(t, w, 429)
	if !strings.Contains(w.Body.String(), "8 秒") {
		t.Fatalf("cooldown message: %s", w.Body.String())
	}
}

// shortToken signs an access token that expires after ttl.
func shortToken(a *App, u User, ttl time.Duration) string {
	body, _ := json.Marshal(claims{Subject: u.ID, Expires: time.Now().Add(ttl).Unix(), Version: u.TokenVersion, Epoch: a.authEpoch()})
	payload := jwtHeader + "." + base64.RawURLEncoding.EncodeToString(body)
	return payload + "." + base64.RawURLEncoding.EncodeToString(sign(a.config.JWTSecret, payload))
}

func TestTeacherSocketRenewsAccessTokenOverPing(t *testing.T) {
	f := newSecurityFixture(t)
	commitState(f.app)
	server := httptest.NewServer(f.app.Handler())
	t.Cleanup(server.Close)
	owner := f.users["owner"]
	renewed := connectCallTeacher(t, server.URL, shortToken(f.app, owner, 2*time.Second))
	stale := connectCallTeacher(t, server.URL, shortToken(f.app, owner, 2*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := wsjson.Write(ctx, renewed, map[string]any{"t": "ping", "d": map[string]string{"token": f.token("owner")}}); err != nil {
		t.Fatal(err)
	}
	rtRead(t, renewed, "pong")
	time.Sleep(2500 * time.Millisecond)
	for _, conn := range []*websocket.Conn{renewed, stale} {
		if err := wsjson.Write(ctx, conn, map[string]any{"t": "ping", "d": map[string]any{}}); err != nil {
			t.Fatal(err)
		}
	}
	rtRead(t, renewed, "pong")
	var frame map[string]any
	if err := wsjson.Read(ctx, stale, &frame); err == nil {
		t.Fatalf("expired socket stayed open: %v", frame)
	}
	// Another account's token never takes over the socket.
	other := connectCallTeacher(t, server.URL, shortToken(f.app, owner, time.Minute))
	if err := wsjson.Write(ctx, other, map[string]any{"t": "ping", "d": map[string]string{"token": f.token("other")}}); err != nil {
		t.Fatal(err)
	}
	rtRead(t, other, "pong")
	f.app.hub.mu.Lock()
	defer f.app.hub.mu.Unlock()
	for p := range f.app.hub.teachers["owner_a"] {
		if p.teacherSession.Subject != "owner_a" {
			t.Fatal("socket adopted another account")
		}
	}
	if len(f.app.hub.teachers["owner_b"]) != 0 {
		t.Fatal("socket moved to another account")
	}
}
