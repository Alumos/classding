package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func scopedRequest(a *App, path, scope string, body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	r := httptest.NewRequest("POST", "http://example.test/api/v1"+path, bytes.NewReader(payload))
	r.Header.Set("X-ClassDing-Auth-Scope", scope)
	r.Header.Set("Content-Type", "application/json")
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	return w
}

func TestProductionHTTPSessionsAndSeparateScopes(t *testing.T) {
	a, err := New(Config{DataDir: t.TempDir(), JWTSecret: strings.Repeat("s", 32)})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err := a.BootstrapAdmin("operator", "Password12345", "管理员"); err != nil {
		t.Fatal(err)
	}
	admin := scopedRequest(a, "/auth/login", "admin", map[string]string{"username": "operator", "password": "Password12345"})
	expectStatus(t, admin, 200)
	adminCookie := admin.Result().Cookies()[0]
	if adminCookie.Name != "classding_refresh_admin" || adminCookie.Secure || !adminCookie.HttpOnly {
		t.Fatalf("wrong production HTTP cookie: %+v", adminCookie)
	}
	teacher := scopedRequest(a, "/auth/register", "teacher", map[string]string{"username": "teacher", "password": "Password12345", "display_name": "老师"})
	expectStatus(t, teacher, 200)
	teacherCookie := teacher.Result().Cookies()[0]
	if teacherCookie.Name != "classding_refresh_teacher" {
		t.Fatalf("wrong teacher cookie: %+v", teacherCookie)
	}
	// Browser sends both cookies; each application restores its own identity.
	adminRefresh := scopedRequest(a, "/auth/refresh", "admin", nil, teacherCookie, adminCookie)
	expectStatus(t, adminRefresh, 200)
	if !strings.Contains(adminRefresh.Body.String(), `"username":"operator"`) {
		t.Fatal("admin session switched accounts")
	}
	teacherRefresh := scopedRequest(a, "/auth/refresh", "teacher", nil, teacherCookie, adminCookie)
	expectStatus(t, teacherRefresh, 200)
	if !strings.Contains(teacherRefresh.Body.String(), `"username":"teacher"`) {
		t.Fatal("teacher session switched accounts")
	}
	expectStatus(t, scopedRequest(a, "/auth/logout", "admin", nil, teacherCookie, adminRefresh.Result().Cookies()[0]), 200)
	expectStatus(t, scopedRequest(a, "/auth/refresh", "teacher", nil, teacherRefresh.Result().Cookies()[0]), 200)
	// A teacher cannot enter the admin scope, even with the right password.
	expectStatus(t, scopedRequest(a, "/auth/login", "admin", map[string]string{"username": "teacher", "password": "Password12345"}), 403)
	// An administrator may also use the teacher application and restore it.
	adminTeacher := scopedRequest(a, "/auth/login", "teacher", map[string]string{"username": "operator", "password": "Password12345"})
	expectStatus(t, adminTeacher, 200)
	if adminTeacher.Result().Cookies()[0].Name != "classding_refresh_teacher" {
		t.Fatal("administrator teacher-app session used wrong cookie")
	}
	expectStatus(t, scopedRequest(a, "/auth/refresh", "teacher", nil, adminTeacher.Result().Cookies()[0]), 200)
}

func TestCookieProtocolAndLegacyMigration(t *testing.T) {
	f := newSecurityFixture(t)
	for _, tc := range []struct {
		name, url, remote, forwarded, mode string
		want                               bool
	}{
		{"http", "http://example.test", "192.0.2.1:1234", "", "auto", false},
		{"https", "https://example.test", "192.0.2.1:1234", "", "auto", true},
		{"untrusted forwarding", "http://example.test", "192.0.2.1:1234", "https", "auto", false},
		{"trusted forwarding", "http://example.test", "10.2.3.4:1234", "https", "auto", true},
		{"forced secure", "http://example.test", "192.0.2.1:1234", "", "true", true},
		{"explicit http", "http://example.test", "192.0.2.1:1234", "", "false", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.app.config.TrustedProxies = []string{"10.0.0.0/8"}
			f.app.config.CookieSecure = tc.mode
			r := httptest.NewRequest("POST", tc.url, nil)
			r.RemoteAddr = tc.remote
			r.Header.Set("X-Forwarded-Proto", tc.forwarded)
			if got := f.app.secureCookie(r); got != tc.want {
				t.Fatalf("Secure=%v, want %v", got, tc.want)
			}
		})
	}
	f.app.config.CookieSecure = "auto"
	u := f.users["super"]
	raw := "legacy-admin-refresh-token"
	f.app.state.Sessions[hash(raw)] = Session{ID: hash(raw), UserID: u.ID, ExpiresAt: after(refreshGrace), Version: u.TokenVersion}
	response := scopedRequest(f.app, "/auth/refresh", "admin", nil, &http.Cookie{Name: "classding_refresh", Value: raw})
	expectStatus(t, response, 200)
	cookies := response.Result().Cookies()
	if len(cookies) != 2 || cookies[0].Name != "classding_refresh_admin" || cookies[1].Name != "classding_refresh" || cookies[1].MaxAge >= 0 {
		t.Fatalf("legacy migration cookies: %+v", cookies)
	}
}

func TestFailedLoginLimitAndAdministratorUnlock(t *testing.T) {
	f := newSecurityFixture(t)
	u := f.app.state.Users["owner_a"]
	u.PasswordHash = passwordHash("Password12345")
	f.app.state.Users[u.ID] = u
	login := func(password string) *httptest.ResponseRecorder {
		return scopedRequest(f.app, "/auth/login", "teacher", map[string]string{"username": u.Username, "password": password})
	}
	// Normal successful use must never exhaust the failed-password allowance.
	for i := 0; i < 10; i++ {
		expectStatus(t, login("Password12345"), 200)
	}
	for i := 0; i < 8; i++ {
		expectStatus(t, login("wrong-password"), 401)
	}
	limited := login("Password12345")
	expectStatus(t, limited, 429)
	if limited.Header().Get("Retry-After") == "" || f.app.loginLockedUntil(u.Username) == "" {
		t.Fatal("locked login did not expose its expiry")
	}
	expectStatus(t, f.request(t, "POST", "/api/v1/admin/users/owner_a/unlock-login", f.token("owner"), nil, nil), 403)
	expectStatus(t, f.request(t, "POST", "/api/v1/admin/users/owner_b/unlock-login", f.token("school"), nil, nil), 403)
	restore := closeStoreForRollback(t, f.app)
	expectStatus(t, f.request(t, "POST", "/api/v1/admin/users/owner_a/unlock-login", f.token("school"), nil, nil), 500)
	if f.app.loginLockedUntil(u.Username) == "" {
		t.Fatal("failed audit persistence must not unlock the login")
	}
	restore()
	expectStatus(t, f.request(t, "POST", "/api/v1/admin/users/owner_a/unlock-login", f.token("school"), nil, nil), 200)
	if f.app.loginLockedUntil(u.Username) != "" {
		t.Fatal("administrator did not clear the login limit")
	}
	expectStatus(t, login("Password12345"), 200)
}
