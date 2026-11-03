package app

import (
	"bytes"
	"classding/server/internal/repo"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// closeStoreForRollback simulates an unavailable database, then reopens it
// so the same request can be retried after the failure.
func closeStoreForRollback(t *testing.T, a *App) func() {
	t.Helper()
	a.mu.Lock()
	err := a.Save()
	if err == nil {
		err = a.store.Close()
	}
	a.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	restored := false
	restore := func() {
		if restored {
			return
		}
		store, err := repo.Open(filepath.Join(a.config.DataDir, "classding.db"))
		if err != nil {
			t.Fatal(err)
		}
		var persisted State
		if err := store.Load(context.Background(), &persisted); err != nil {
			_ = store.Close()
			t.Fatal(err)
		}
		a.mu.Lock()
		a.store = store
		a.mu.Unlock()
		restored = true
	}
	t.Cleanup(restore)
	return restore
}

func TestRollbackRegistration(t *testing.T) {
	f := newSecurityFixture(t)
	restore := closeStoreForRollback(t, f.app)
	body := map[string]any{"username": "new_teacher", "password": "Password12345", "display_name": "新老师"}
	w := f.request(t, http.MethodPost, "/api/v1/auth/register", "", body, nil)
	expectStatus(t, w, http.StatusInternalServerError)
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("failed registration must not replace the browser session cookie")
	}
	for _, user := range f.app.state.Users {
		if user.Username == "new_teacher" {
			t.Fatal("failed registration left a user in memory")
		}
	}
	if len(f.app.state.Sessions) != 0 {
		t.Fatal("failed registration left a session in memory")
	}
	if f.app.limits["register:192.0.2.1"].Count != 1 {
		t.Fatal("failed request must still count towards rate limiting")
	}
	restore()
	expectStatus(t, f.request(t, http.MethodPost, "/api/v1/auth/register", "", body, nil), http.StatusOK)
}

func TestRollbackRefreshCookie(t *testing.T) {
	f := newSecurityFixture(t)
	registration := f.request(t, http.MethodPost, "/api/v1/auth/register", "", map[string]any{"username": "new_teacher", "password": "Password12345", "display_name": "新老师"}, nil)
	expectStatus(t, registration, http.StatusOK)
	cookies := registration.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("registration did not provide a refresh cookie")
	}
	restore := closeStoreForRollback(t, f.app)
	failed := f.request(t, http.MethodPost, "/api/v1/auth/refresh", "", nil, cookies[0])
	expectStatus(t, failed, http.StatusInternalServerError)
	if len(failed.Result().Cookies()) != 0 {
		t.Fatal("failed refresh replaced a valid cookie")
	}
	restore()
	expectStatus(t, f.request(t, http.MethodPost, "/api/v1/auth/refresh", "", nil, cookies[0]), http.StatusOK)
	invalid := f.request(t, http.MethodPost, "/api/v1/auth/refresh", "", nil, &http.Cookie{Name: "classding_refresh", Value: "invalid"})
	expectStatus(t, invalid, http.StatusUnauthorized)
	if len(invalid.Result().Cookies()) != 0 {
		t.Fatal("invalid stale refresh must not erase a newer browser cookie")
	}
}

func TestRollbackClassAndLicense(t *testing.T) {
	f := newSecurityFixture(t)
	f.app.state.Codes["ROLLBACK"] = Code{Code: "ROLLBACK", Status: "unused", MaxTeachers: 2, MaxDevices: 1}
	classes, members, audits := len(f.app.state.Classes), len(f.app.state.Members), len(f.app.state.Audit)
	restore := closeStoreForRollback(t, f.app)
	body := map[string]any{"name": "回滚班级", "activation_code": "ROLLBACK"}
	expectStatus(t, f.request(t, http.MethodPost, "/api/v1/classes", f.token("owner"), body, nil), http.StatusInternalServerError)
	if len(f.app.state.Classes) != classes || len(f.app.state.Members) != members || len(f.app.state.Audit) != audits {
		t.Fatalf("failed creation left classes, members, or audit entries: %d, %d, %d", len(f.app.state.Classes), len(f.app.state.Members), len(f.app.state.Audit))
	}
	if code := f.app.state.Codes["ROLLBACK"]; code.Status != "unused" || code.ClassID != nil {
		t.Fatalf("activation code was not restored: %+v", code)
	}
	restore()
	expectStatus(t, f.request(t, http.MethodPost, "/api/v1/classes", f.token("owner"), body, nil), http.StatusOK)
	code := f.app.state.Codes["ROLLBACK"]
	if code.ClassID == nil || !f.app.licensed(f.app.state.Classes[*code.ClassID]) {
		t.Fatal("successfully created class should be licensed")
	}
	if f.app.licensed(Class{ID: "another_class", ActivationCode: "ROLLBACK"}) {
		t.Fatal("a code bound to another class must not grant a license")
	}
}

func TestRollbackDeviceBinding(t *testing.T) {
	f := newSecurityFixture(t)
	c := f.app.state.Classes["class_a"]
	c.MaxDevices = 2
	f.app.state.Classes[c.ID] = c
	f.app.bindings["123456"] = binding{ClassID: c.ID, Expires: after(time.Minute)}
	restore := closeStoreForRollback(t, f.app)
	body := map[string]any{"code": "123456", "name": "新设备"}
	expectStatus(t, f.request(t, http.MethodPost, "/api/v1/device/bind", "", body, nil), http.StatusInternalServerError)
	if _, exists := f.app.bindings["123456"]; !exists {
		t.Fatal("failed binding consumed the binding code")
	}
	if len(f.app.state.Devices) != 2 {
		t.Fatalf("failed binding left a device: %d devices", len(f.app.state.Devices))
	}
	if f.app.limits["bind:192.0.2.1"].Count != 1 {
		t.Fatal("failed binding must still count towards rate limiting")
	}
	restore()
	expectStatus(t, f.request(t, http.MethodPost, "/api/v1/device/bind", "", body, nil), http.StatusOK)
}

func TestRollbackMessageCooldown(t *testing.T) {
	f := newSecurityFixture(t)
	f.app.config.SendCooldown = time.Minute
	restore := closeStoreForRollback(t, f.app)
	expectStatus(t, f.request(t, http.MethodPost, "/api/v1/messages", f.token("owner"), securityText("class_a"), nil), http.StatusInternalServerError)
	if len(f.app.state.Messages) != 3 {
		t.Fatal("failed send left a message in memory")
	}
	if _, exists := f.app.lastSent["class_a"]; exists {
		t.Fatal("failed send started the cooldown")
	}
	restore()
	expectStatus(t, f.request(t, http.MethodPost, "/api/v1/messages", f.token("owner"), securityText("class_a"), nil), http.StatusOK)
}

func TestRollbackAudioFile(t *testing.T) {
	f := newSecurityFixture(t)
	restore := closeStoreForRollback(t, f.app)
	upload := func() *httptest.ResponseRecorder {
		var body bytes.Buffer
		multipartWriter := multipart.NewWriter(&body)
		headers := make(textproto.MIMEHeader)
		headers.Set("Content-Disposition", `form-data; name="file"; filename="clip.webm"`)
		headers.Set("Content-Type", "audio/webm")
		part, err := multipartWriter.CreatePart(headers)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte("recording")); err != nil {
			t.Fatal(err)
		}
		if err := multipartWriter.Close(); err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, "/api/v1/audio", &body)
		r.Header.Set("Content-Type", multipartWriter.FormDataContentType())
		r.Header.Set("Authorization", "Bearer "+f.token("owner"))
		w := httptest.NewRecorder()
		f.app.Handler().ServeHTTP(w, r)
		return w
	}
	expectStatus(t, upload(), http.StatusInternalServerError)
	files, err := os.ReadDir(filepath.Join(f.app.config.DataDir, "audio"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 || len(f.app.state.Audio) != 0 {
		t.Fatalf("failed upload left %d files and %d records", len(files), len(f.app.state.Audio))
	}
	restore()
	expectStatus(t, upload(), http.StatusOK)
	files, err = os.ReadDir(filepath.Join(f.app.config.DataDir, "audio"))
	if err != nil || len(files) != 1 || len(f.app.state.Audio) != 1 {
		t.Fatalf("successful upload: %d files, %d records, error %v", len(files), len(f.app.state.Audio), err)
	}
}

func TestRollbackMemberRemovalSideEffects(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, user string
	}{
		{"remove", http.MethodDelete, "/api/v1/classes/class_a/members/collab_a", "owner"},
		{"leave", http.MethodPost, "/api/v1/classes/class_a/leave", "collab"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSecurityFixture(t)
			f.app.hub.teachers["collab_a"] = map[*Peer]bool{}
			restore := closeStoreForRollback(t, f.app)
			expectStatus(t, f.request(t, tc.method, tc.path, f.token(tc.user), nil, nil), http.StatusInternalServerError)
			if _, exists := f.app.state.Members["class_a|collab_a"]; !exists {
				t.Fatal("failed operation removed the member")
			}
			if _, exists := f.app.hub.teachers["collab_a"]; !exists {
				t.Fatal("failed operation disconnected the teacher")
			}
			restore()
			expectStatus(t, f.request(t, tc.method, tc.path, f.token(tc.user), nil, nil), http.StatusOK)
			if _, exists := f.app.hub.teachers["collab_a"]; exists {
				t.Fatal("committed operation did not disconnect the teacher")
			}
		})
	}
}

func TestRollbackDeviceUnbindSideEffects(t *testing.T) {
	f := newSecurityFixture(t)
	peer := &Peer{ctx: context.Background(), queue: make(chan Frame, 4)}
	f.app.hub.teachers["owner_a"] = map[*Peer]bool{peer: true}
	defer delete(f.app.hub.teachers, "owner_a")
	restore := closeStoreForRollback(t, f.app)
	expectStatus(t, f.request(t, http.MethodDelete, "/api/v1/devices/device_a", f.token("owner"), nil, nil), http.StatusInternalServerError)
	if _, exists := f.app.state.Devices["device_a"]; !exists {
		t.Fatal("failed unbind removed the device")
	}
	if status := f.app.state.Messages["message_a"].Targets[0].Status; status != "queued" {
		t.Fatalf("failed unbind changed message status to %q", status)
	}
	if len(peer.queue) != 0 {
		t.Fatal("failed unbind sent a presence event")
	}
	restore()
	expectStatus(t, f.request(t, http.MethodDelete, "/api/v1/devices/device_a", f.token("owner"), nil, nil), http.StatusOK)
	if len(peer.queue) != 1 {
		t.Fatal("successful unbind did not send a presence event")
	}
}
