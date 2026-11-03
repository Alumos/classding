package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type securityFixture struct {
	app   *App
	users map[string]User
}

func newSecurityFixture(t *testing.T) *securityFixture {
	t.Helper()
	a, err := New(Config{DataDir: t.TempDir(), JWTSecret: strings.Repeat("s", 32), SendCooldown: 0})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Error(err)
		}
	})
	users := map[string]User{
		"school":   {ID: "school_a", Username: "school_a", DisplayName: "学校管理员", Role: "school_admin", OrgID: "org_a", Status: "active"},
		"super":    {ID: "super", Username: "super", DisplayName: "运营管理员", Role: "super_admin", OrgID: "ops", Status: "active"},
		"owner":    {ID: "owner_a", Username: "owner_a", DisplayName: "甲老师", Role: "user", OrgID: "org_a", Status: "active"},
		"collab":   {ID: "collab_a", Username: "collab_a", DisplayName: "乙老师", Role: "user", OrgID: "org_a", Status: "active"},
		"other":    {ID: "owner_b", Username: "owner_b", DisplayName: "外校老师", Role: "user", OrgID: "org_b", Status: "active"},
		"outsider": {ID: "outsider", Username: "outsider", DisplayName: "无关老师", Role: "user", OrgID: "org_a", Status: "active"},
	}
	for _, u := range users {
		a.state.Users[u.ID] = u
	}
	for _, c := range []Class{
		{ID: "class_a", Name: "甲班", OrgID: "org_a", OwnerID: "owner_a", MaxTeachers: 2, MaxDevices: 1, ActivationCode: "CODE-A", CreatedAt: now()},
		{ID: "class_b", Name: "外校班级", OrgID: "org_b", OwnerID: "owner_b", MaxTeachers: 3, MaxDevices: 1, ActivationCode: "CODE-B", CreatedAt: now()},
	} {
		a.state.Classes[c.ID] = c
		code := c.ActivationCode
		classID := c.ID
		a.state.Codes[code] = Code{Code: code, Status: "active", ClassID: &classID, OrgID: c.OrgID, MaxTeachers: c.MaxTeachers, MaxDevices: c.MaxDevices}
	}
	for _, m := range []Member{
		{UserID: "owner_a", ClassID: "class_a", Role: "owner"}, {UserID: "collab_a", ClassID: "class_a", Role: "collaborator"},
		{UserID: "owner_b", ClassID: "class_b", Role: "owner"}, {UserID: "school_a", ClassID: "class_b", Role: "collaborator"},
	} {
		a.state.Members[m.ClassID+"|"+m.UserID] = m
	}
	for _, d := range []Device{{ID: "device_a", ClassID: "class_a", Name: "甲设备", TokenHash: hash("token-a")}, {ID: "device_b", ClassID: "class_b", Name: "外校设备", TokenHash: hash("token-b")}} {
		a.state.Devices[d.ID] = d
	}
	for _, m := range []Message{
		{ID: "message_a", Kind: "text", Text: "甲班通知", SenderID: "owner_a", CreatedAt: now(), ExpiresAt: after(time.Hour), Targets: []Target{{ClassID: "class_a", ClassName: "甲班", DeviceID: "device_a", Status: "queued"}}},
		{ID: "message_b", Kind: "text", Text: "外校私密通知", SenderID: "owner_b", CreatedAt: now(), ExpiresAt: after(time.Hour), Targets: []Target{{ClassID: "class_b", ClassName: "外校班级", DeviceID: "device_b", Status: "queued"}}},
		{ID: "message_mixed", Kind: "text", Text: "跨班通知", SenderID: "owner_a", CreatedAt: now(), ExpiresAt: after(time.Hour), Targets: []Target{{ClassID: "class_a", ClassName: "甲班", DeviceID: "device_a", Status: "queued"}, {ClassID: "class_b", ClassName: "外校班级", DeviceID: "device_b", Status: "queued"}}},
	} {
		a.state.Messages[m.ID] = m
	}
	return &securityFixture{app: a, users: users}
}

func (f *securityFixture) token(who string) string {
	u := f.users[who]
	return f.app.access(u)
}
func (f *securityFixture) request(t *testing.T, method, path, token string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	f.app.Handler().ServeHTTP(w, r)
	return w
}
func expectStatus(t *testing.T, response *httptest.ResponseRecorder, status int) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, status, response.Body.String())
	}
}
func securityText(classes ...string) map[string]any {
	return map[string]any{"class_ids": classes, "kind": "text", "text": "请到办公室", "style": "notice", "display_sec": 60, "repeat": 1, "chime": true, "play_mode": "tts"}
}

func TestSecuritySchoolAdminOrgIsolation(t *testing.T) {
	f := newSecurityFixture(t)
	for _, tc := range []struct{ path, want, forbidden string }{
		{"/api/v1/admin/classes", "class_a", "class_b"},
		{"/api/v1/admin/devices", "device_a", "device_b"},
		{"/api/v1/admin/messages", "message_a", "message_b"},
		{"/api/v1/admin/overview", "message_a", "外校私密通知"},
		{"/api/v1/admin/users", "owner_a", "owner_b"},
		{"/api/v1/admin/activation-codes", "CODE-A", "CODE-B"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := f.request(t, "GET", tc.path, f.token("school"), nil, nil)
			expectStatus(t, w, http.StatusOK)
			if !strings.Contains(w.Body.String(), tc.want) {
				t.Errorf("expected own-school data %q: %s", tc.want, w.Body.String())
			}
			if strings.Contains(w.Body.String(), tc.forbidden) {
				t.Errorf("foreign-school data %q leaked: %s", tc.forbidden, w.Body.String())
			}
			if (tc.path == "/api/v1/admin/messages" || tc.path == "/api/v1/admin/overview") && strings.Contains(w.Body.String(), "device_b") {
				t.Errorf("foreign target leaked: %s", w.Body.String())
			}
		})
	}
}

func TestSecurityClassPermissionsAndQuotas(t *testing.T) {
	f := newSecurityFixture(t)
	for _, tc := range []struct {
		method, path, user string
		body               any
		status             int
	}{
		{"GET", "/api/v1/classes/class_a/members", "outsider", nil, 403},
		{"PATCH", "/api/v1/classes/class_a", "collab", map[string]any{"name": "越权修改"}, 403},
		{"POST", "/api/v1/classes/class_a/invites", "collab", nil, 403},
		{"POST", "/api/v1/classes/class_a/bind-code", "collab", nil, 403},
		{"DELETE", "/api/v1/classes/class_a/members/owner_a", "collab", nil, 403},
		{"POST", "/api/v1/classes/class_a/invites", "owner", nil, 409},
		{"POST", "/api/v1/classes/class_a/bind-code", "owner", nil, 409},
		{"PATCH", "/api/v1/admin/activation-codes/CODE-A", "super", map[string]any{"max_teachers": 1}, 400},
		{"POST", "/api/v1/messages", "collab", securityText("class_a", "class_b"), 403},
		{"POST", "/api/v1/messages/message_a/recall", "collab", nil, 403},
	} {
		t.Run(tc.method+tc.path+tc.user, func(t *testing.T) {
			expectStatus(t, f.request(t, tc.method, tc.path, f.token(tc.user), tc.body, nil), tc.status)
		})
	}
	if len(f.app.state.Messages) != 3 {
		t.Fatalf("failed send must not create a partial message")
	}
	w := f.request(t, "POST", "/api/v1/messages", f.token("collab"), securityText("class_a"), nil)
	expectStatus(t, w, 200)
	var message Message
	if err := json.Unmarshal(w.Body.Bytes(), &message); err != nil {
		t.Fatal(err)
	}
	expectStatus(t, f.request(t, "POST", "/api/v1/messages/"+message.ID+"/recall", f.token("collab"), nil, nil), 200)
	expectStatus(t, f.request(t, "POST", "/api/v1/messages/message_a/recall", f.token("owner"), nil, nil), 200)
}

func TestSecurityAudioOwnership(t *testing.T) {
	f := newSecurityFixture(t)
	path := filepath.Join(f.app.config.DataDir, "audio", "private.webm")
	if err := os.WriteFile(path, []byte("private recording"), 0600); err != nil {
		t.Fatal(err)
	}
	f.app.state.Audio["other_audio"] = Audio{ID: "other_audio", UserID: "owner_b", Path: path, MIME: "audio/webm", CreatedAt: now()}
	for _, tc := range []struct{ kind, mode string }{{"text", "tts"}, {"voice", "voice"}} {
		body := securityText("class_a")
		body["kind"] = tc.kind
		body["play_mode"] = tc.mode
		body["audio_id"] = "other_audio"
		w := f.request(t, "POST", "/api/v1/messages", f.token("owner"), body, nil)
		if w.Code < 400 || w.Code >= 500 {
			t.Errorf("%s message attached other user's audio: status %d body %s", tc.kind, w.Code, w.Body.String())
		}
	}
	expectStatus(t, f.request(t, "GET", "/api/v1/audio/other_audio", "", nil, nil), 403)
	expectStatus(t, f.request(t, "GET", "/api/v1/audio/other_audio?expires=9999999999&signature=invalid", "", nil, nil), 403)
	w := f.request(t, "GET", f.app.audioURL("other_audio"), "", nil, nil)
	expectStatus(t, w, 200)
	if w.Body.String() != "private recording" {
		t.Fatalf("signed download returned %q", w.Body.String())
	}
	expired := fmt.Sprint(time.Now().Add(-time.Minute).Unix())
	expiredURL := "/api/v1/audio/other_audio?expires=" + expired + "&signature=" + f.app.audioSignature("other_audio", expired)
	expectStatus(t, f.request(t, "GET", expiredURL, "", nil, nil), 403)

	f.app.state.Audio["own_audio"] = Audio{ID: "own_audio", UserID: "owner_a", Path: path, MIME: "audio/webm", CreatedAt: now()}
	body := securityText("class_a")
	body["kind"] = "voice"
	body["play_mode"] = "voice"
	body["audio_id"] = "own_audio"
	w = f.request(t, "POST", "/api/v1/messages", f.token("owner"), body, nil)
	expectStatus(t, w, 200)
	var message Message
	if err := json.Unmarshal(w.Body.Bytes(), &message); err != nil {
		t.Fatal(err)
	}
	frame := f.app.notify(message, message.Targets[0])
	url, _ := frame.D.(map[string]any)["audio_url"].(string)
	if url == "" {
		t.Fatal("owned voice message has no signed audio URL")
	}
	expectStatus(t, f.request(t, "GET", url, "", nil, nil), 200)
	message.AudioID = "other_audio"
	frame = f.app.notify(message, message.Targets[0])
	if url, _ := frame.D.(map[string]any)["audio_url"].(string); url != "" {
		t.Fatal("notification signed another user's audio")
	}
}

func TestSecurityReleasedLicenseCanBeReused(t *testing.T) {
	f := newSecurityFixture(t)
	expectStatus(t, f.request(t, "DELETE", "/api/v1/classes/class_a", f.token("owner"), nil, nil), 200)
	code := f.app.state.Codes["CODE-A"]
	if code.Status != "released" || code.ClassID != nil {
		t.Fatalf("released code retains deleted class or wrong status: %+v", code)
	}
	expectStatus(t, f.request(t, "PATCH", "/api/v1/admin/activation-codes/CODE-A", f.token("super"), map[string]any{"status": "unused"}, nil), 200)
	expectStatus(t, f.request(t, "POST", "/api/v1/classes", f.token("owner"), map[string]any{"name": "重新开通", "activation_code": "CODE-A"}, nil), 200)
}

func TestSecurityDeletingClassPreservesRevocation(t *testing.T) {
	f := newSecurityFixture(t)
	code := f.app.state.Codes["CODE-A"]
	code.Status = "revoked"
	f.app.state.Codes["CODE-A"] = code
	expectStatus(t, f.request(t, "DELETE", "/api/v1/classes/class_a", f.token("owner"), nil, nil), 200)
	code = f.app.state.Codes["CODE-A"]
	if code.Status != "revoked" || code.ClassID != nil {
		t.Fatalf("delete changed a revoked license: %+v", code)
	}
}

func TestSecurityRefreshRotationAndPasswordChange(t *testing.T) {
	f := newSecurityFixture(t)
	u := f.app.state.Users["owner_a"]
	u.PasswordHash = passwordHash("OldPassword1234")
	f.app.state.Users[u.ID] = u
	login := f.request(t, "POST", "/api/v1/auth/login", "", map[string]any{"username": "owner_a", "password": "OldPassword1234"}, nil)
	expectStatus(t, login, 200)
	var auth struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &auth); err != nil || auth.AccessToken == "" {
		t.Fatalf("invalid auth response: %s", login.Body.String())
	}
	cookies := login.Result().Cookies()
	if len(cookies) == 0 || !cookies[0].HttpOnly {
		t.Fatal("missing HttpOnly refresh cookie")
	}
	oldCookie := cookies[0]
	rotated := f.request(t, "POST", "/api/v1/auth/refresh", "", nil, oldCookie)
	expectStatus(t, rotated, 200)
	newCookies := rotated.Result().Cookies()
	if len(newCookies) == 0 || newCookies[0].Value == oldCookie.Value {
		t.Fatal("refresh token did not rotate")
	}
	duplicate := f.request(t, "POST", "/api/v1/auth/refresh", "", nil, oldCookie)
	expectStatus(t, duplicate, 200)
	if got := duplicate.Result().Cookies()[0].Value; got != newCookies[0].Value {
		t.Fatal("concurrent refresh did not converge on the same successor")
	}
	oldSession := f.app.state.Sessions[hash(oldCookie.Value)]
	oldSession.RotationUntil = time.Now().Add(-time.Second).Format(time.RFC3339Nano)
	f.app.state.Sessions[hash(oldCookie.Value)] = oldSession
	expectStatus(t, f.request(t, "POST", "/api/v1/auth/refresh", "", nil, oldCookie), 401)
	expectStatus(t, f.request(t, "PUT", "/api/v1/me/password", auth.AccessToken, map[string]any{"current_password": "OldPassword1234", "new_password": "NewPassword1234"}, nil), 200)
	expectStatus(t, f.request(t, "GET", "/api/v1/me", auth.AccessToken, nil, nil), 401)
	expectStatus(t, f.request(t, "POST", "/api/v1/auth/refresh", "", nil, newCookies[0]), 401)
	expectStatus(t, f.request(t, "POST", "/api/v1/auth/login", "", map[string]any{"username": "owner_a", "password": "OldPassword1234"}, nil), 401)
	expectStatus(t, f.request(t, "POST", "/api/v1/auth/login", "", map[string]any{"username": "owner_a", "password": "NewPassword1234"}, nil), 200)
}

func TestSecurityAdminLoginWithoutTOTP(t *testing.T) {
	f := newSecurityFixture(t)
	u := f.app.state.Users["school_a"]
	u.PasswordHash = passwordHash("AdminPassword1234")
	f.app.state.Users[u.ID] = u
	login := f.request(t, "POST", "/api/v1/auth/login", "", map[string]any{"username": "school_a", "password": "AdminPassword1234"}, nil)
	expectStatus(t, login, 200)
	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	expectStatus(t, f.request(t, "GET", "/api/v1/admin/classes", result.AccessToken, nil, nil), 200)
}

type securityGatedBody struct {
	entered, release chan struct{}
	once             sync.Once
}

func (b *securityGatedBody) Read(_ []byte) (int, error) {
	b.once.Do(func() { close(b.entered) })
	<-b.release
	return 0, io.EOF
}
func (b *securityGatedBody) Close() error { return nil }

func TestSecuritySlowRequestBodyDoesNotBlockOtherRequests(t *testing.T) {
	f := newSecurityFixture(t)
	body := &securityGatedBody{entered: make(chan struct{}), release: make(chan struct{})}
	request := httptest.NewRequest("POST", "/api/v1/auth/register", nil)
	request.Body = body
	firstDone := make(chan struct{})
	go func() { f.app.Handler().ServeHTTP(httptest.NewRecorder(), request); close(firstDone) }()
	select {
	case <-body.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not start reading")
	}
	secondDone := make(chan struct{})
	go func() {
		f.app.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/v1/config", nil))
		close(secondDone)
	}()
	blocked := false
	select {
	case <-secondDone:
	case <-time.After(300 * time.Millisecond):
		blocked = true
	}
	close(body.release)
	<-firstDone
	<-secondDone
	if blocked {
		t.Fatal("slow unauthenticated request body blocked an unrelated request")
	}
}
