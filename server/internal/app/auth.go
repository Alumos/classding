package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const refreshGrace = 30 * time.Second

const (
	minUIScale = 85
	maxUIScale = 150
)

func authScope(u User) string {
	if u.Role == "user" {
		return "teacher"
	}
	return "admin"
}

func refreshCookieName(scope string) string { return "classding_refresh_" + scope }

// A forwarding header is only meaningful when the immediate peer is a trusted proxy.
func (a *App) secureCookie(r *http.Request) bool {
	switch a.config.CookieSecure {
	case "true":
		return true
	case "false":
		return false
	}
	if r.TLS != nil {
		return true
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer := net.ParseIP(host)
	forwarded := r.Header.Values("X-Forwarded-Proto")
	return peer != nil && a.isTrustedProxy(peer) && len(forwarded) == 1 && strings.EqualFold(strings.TrimSpace(forwarded[0]), "https")
}

func (a *App) cookie(w http.ResponseWriter, r *http.Request, name, value string, age int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/api/v1/auth", MaxAge: age, HttpOnly: true, Secure: a.secureCookie(r), SameSite: http.SameSiteStrictMode})
}

func (a *App) issue(w http.ResponseWriter, r *http.Request, u User) any {
	raw := random(32)
	key := hash(raw)
	scope := requestScope(r)
	if scope == "" {
		scope = authScope(u)
	}
	a.state.Sessions[key] = Session{ID: key, UserID: u.ID, ExpiresAt: after(30 * 24 * time.Hour), Version: u.TokenVersion, Scope: scope}
	a.cookie(w, r, refreshCookieName(scope), raw, 30*24*3600)
	return map[string]any{"access_token": a.access(u), "user": publicUser(u)}
}

// A server-secret-derived successor lets simultaneous refreshes converge without
// persisting bearer tokens. Rotated tokens are accepted only for a short grace period.
func (a *App) refreshSuccessor(raw string) string {
	mac := hmac.New(sha256.New, []byte(a.config.JWTSecret))
	mac.Write([]byte("classding-refresh-rotation-v1\x00"))
	mac.Write([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func requestScope(r *http.Request) string {
	switch r.Header.Get("X-ClassDing-Auth-Scope") {
	case "admin", "teacher":
		return r.Header.Get("X-ClassDing-Auth-Scope")
	default:
		return ""
	}
}

func refreshCookie(r *http.Request) (*http.Cookie, error) {
	if scope := requestScope(r); scope != "" {
		if cookie, err := r.Cookie(refreshCookieName(scope)); err == nil {
			return cookie, nil
		}
	}
	// Existing installations can migrate their original shared cookie.
	if cookie, err := r.Cookie("classding_refresh"); err == nil {
		return cookie, nil
	}
	if requestScope(r) == "" {
		for _, scope := range []string{"teacher", "admin"} {
			if cookie, err := r.Cookie(refreshCookieName(scope)); err == nil {
				return cookie, nil
			}
		}
	}
	return nil, http.ErrNoCookie
}
func (a *App) register(w http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	var in struct {
		Username, Password string
		Name               string `json:"display_name"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if !a.limit("register:"+a.clientIP(r), 10, time.Hour) {
		return nil, fail(429, "RATE_LIMITED", "注册过于频繁，请稍后再试。")
	}
	in.Username = strings.ToLower(strings.TrimSpace(in.Username))
	if len(in.Username) < 3 || len(in.Username) > 32 || !validPassword(in.Password) || utf8.RuneCountInString(strings.TrimSpace(in.Name)) < 1 || utf8.RuneCountInString(in.Name) > 24 {
		return nil, bad("用户名需 3–32 位，密码至少 10 位且包含字母和数字，落款 1–24 字。")
	}
	for _, ch := range in.Username {
		if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '-' || ch == '.') {
			return nil, bad("用户名仅支持英文字母、数字、点、下划线和短横线。")
		}
	}
	for _, u := range a.state.Users {
		if u.Username == in.Username {
			return nil, fail(409, "USERNAME_TAKEN", "此用户名已被使用。")
		}
	}
	u := User{ID: id("u"), Username: in.Username, DisplayName: strings.TrimSpace(in.Name), Role: "user", OrgID: id("org"), Status: "active", PasswordHash: passwordHash(in.Password)}
	a.state.Users[u.ID] = u
	a.audit(&u, "auth.register", u.ID)
	return a.issue(w, r, u), nil
}
func (a *App) login(w http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	var in struct {
		Username, Password string
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	username := strings.ToLower(strings.TrimSpace(in.Username))
	ip := a.clientIP(r)
	if wait := a.loginWait(ip, username); wait > 0 {
		seconds := int(wait.Seconds()) + 1
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		return nil, fail(429, "RATE_LIMITED", "密码错误次数过多，请在 "+strconv.Itoa(seconds)+" 秒后重试，或联系管理员解除登录限制。")
	}
	var found User
	for _, u := range a.state.Users {
		if u.Username == username {
			found = u
			break
		}
	}
	if found.ID == "" || !passwordMatches(in.Password, found.PasswordHash) || found.Status != "active" {
		a.limit("login:"+ip+":"+username, 8, time.Minute)
		a.limit("login-ip:"+ip, 30, time.Minute)
		return nil, fail(401, "INVALID_CREDENTIALS", "用户名或密码不正确，或账号已被停用。")
	}
	if requestScope(r) == "admin" && !a.isAdmin(&found) {
		return nil, forbidden()
	}
	delete(a.limits, "login:"+ip+":"+username)
	a.audit(&found, "auth.login", found.ID)
	return a.issue(w, r, found), nil
}
func (a *App) refresh(w http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	cookie, err := refreshCookie(r)
	if err != nil {
		return nil, fail(401, "UNAUTHORIZED", "请先登录。")
	}
	raw := cookie.Value
	session, exists := a.state.Sessions[hash(raw)]
	if !exists || elapsed(session.ExpiresAt) {
		return nil, fail(401, "UNAUTHORIZED", "登录已过期。")
	}
	u, exists := a.state.Users[session.UserID]
	if !exists || u.Status != "active" || u.TokenVersion != session.Version || requestScope(r) == "admin" && !a.isAdmin(&u) || session.Scope != "" && requestScope(r) != "" && session.Scope != requestScope(r) {
		return nil, fail(401, "UNAUTHORIZED", "账号状态已改变，请重新登录。")
	}
	scope := session.Scope
	if scope == "" {
		scope = requestScope(r)
		if scope == "" {
			scope = authScope(u)
		}
	}
	if session.RotationUntil == "" {
		session.RotationUntil = after(refreshGrace)
		a.state.Sessions[hash(raw)] = session
		raw = a.refreshSuccessor(raw)
		key := hash(raw)
		a.state.Sessions[key] = Session{ID: key, UserID: u.ID, ExpiresAt: after(30 * 24 * time.Hour), Version: u.TokenVersion, Scope: scope}
	} else {
		// Follow concurrent rotations to the latest successor. Every link must
		// still be within its grace period and belong to the same session user.
		for session.RotationUntil != "" {
			if elapsed(session.RotationUntil) {
				return nil, fail(401, "UNAUTHORIZED", "登录已过期。")
			}
			raw = a.refreshSuccessor(raw)
			session, exists = a.state.Sessions[hash(raw)]
			if !exists || session.UserID != u.ID || session.Version != u.TokenVersion || elapsed(session.ExpiresAt) {
				return nil, fail(401, "UNAUTHORIZED", "登录已过期。")
			}
		}
	}
	a.cookie(w, r, refreshCookieName(scope), raw, 30*24*3600)
	if cookie.Name == "classding_refresh" {
		a.cookie(w, r, cookie.Name, "", -1)
	}
	return map[string]any{"access_token": a.access(u), "user": publicUser(u)}, nil
}
func (a *App) logout(w http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	var uid string
	cookie, cookieErr := refreshCookie(r)
	if cookieErr == nil {
		if session, ok := a.state.Sessions[hash(cookie.Value)]; ok {
			if user, exists := a.state.Users[session.UserID]; exists && (requestScope(r) != "admin" || a.isAdmin(&user)) {
				uid = session.UserID
			}
		}
	}
	if claims, err := a.verify(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")); err == nil {
		if user, exists := a.state.Users[claims.Subject]; exists && (requestScope(r) != "admin" || a.isAdmin(&user)) {
			uid = claims.Subject
		}
	}
	if uid != "" {
		user := a.state.Users[uid]
		user.TokenVersion++
		a.state.Users[uid] = user
		for key, session := range a.state.Sessions {
			if session.UserID == uid {
				delete(a.state.Sessions, key)
			}
		}
		a.deferJob(func() { a.hub.RemoveTeacher(uid) })
	}
	if cookieErr == nil {
		// Do not erase a different account's legacy cookie when the caller
		// explicitly asked to log out another scope.
		if cookie.Name != "classding_refresh" || requestScope(r) == "" || uid != "" {
			a.cookie(w, r, cookie.Name, "", -1)
		}
	} else if scope := requestScope(r); scope != "" {
		a.cookie(w, r, refreshCookieName(scope), "", -1)
	}
	return ok(), nil
}
func (a *App) updateMe(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in struct {
		Name    *string `json:"display_name"`
		UIScale *int    `json:"ui_scale"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if in.Name == nil && in.UIScale == nil {
		return nil, bad("没有需要保存的内容。")
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 24 {
			return nil, bad("落款需为 1–24 字。")
		}
		u.DisplayName = name
	}
	if in.UIScale != nil {
		if *in.UIScale < minUIScale || *in.UIScale > maxUIScale {
			return nil, bad("界面字号需在 85% 至 150% 之间。")
		}
		u.UIScale = *in.UIScale
	}
	a.state.Users[u.ID] = *u
	return publicUser(*u), nil
}
func (a *App) changePassword(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if !a.limit("password:"+u.ID, 5, time.Minute) {
		return nil, fail(429, "RATE_LIMITED", "尝试过于频繁。")
	}
	if !passwordMatches(in.Current, u.PasswordHash) {
		return nil, bad("当前密码不正确。")
	}
	if !validPassword(in.New) {
		return nil, bad("新密码至少 10 位，且包含字母和数字。")
	}
	u.PasswordHash = passwordHash(in.New)
	u.TokenVersion++
	a.state.Users[u.ID] = *u
	for key, s := range a.state.Sessions {
		if s.UserID == u.ID {
			delete(a.state.Sessions, key)
		}
	}
	uid := u.ID
	a.deferJob(func() { a.hub.RemoveTeacher(uid) })
	a.audit(u, "auth.password", u.ID)
	return ok(), nil
}
func (a *App) seed() {
	for _, entry := range []struct{ Username, Name, Role, Password string }{{"teacher", "林老师", "user", "ClassDing2026!"}, {"admin", "管理员", "super_admin", "AdminDing2026!"}} {
		exists := false
		for _, u := range a.state.Users {
			if u.Username == entry.Username {
				exists = true
			}
		}
		if exists {
			continue
		}
		u := User{ID: id("u"), Username: entry.Username, DisplayName: entry.Name, Role: entry.Role, OrgID: "org_demo", Status: "active", PasswordHash: passwordHash(entry.Password)}
		a.state.Users[u.ID] = u
		if entry.Role == "user" {
			for i, name := range []string{"三年级（1）班", "三年级（2）班"} {
				code := []string{"DEMO-A234", "DEMO-B234"}[i]
				c := Class{ID: id("c"), Name: name, Grade: "三年级", OwnerID: u.ID, OrgID: u.OrgID, Volume: 75, MaxTeachers: 3, MaxDevices: 2, CreatedAt: now(), ActivationCode: code}
				a.state.Classes[c.ID] = c
				a.state.Members[c.ID+"|"+u.ID] = Member{u.ID, c.ID, u.DisplayName, u.Username, "owner", now()}
				stamp := now()
				a.state.Codes[code] = Code{Code: code, MaxTeachers: 3, MaxDevices: 2, Status: "active", ClassID: &c.ID, OrgID: u.OrgID, BatchID: "demo", Note: "仅本地开发", RedeemedAt: &stamp}
			}
		}
	}
	if _, exists := a.state.Codes["TEST-2345"]; !exists {
		a.state.Codes["TEST-2345"] = Code{Code: "TEST-2345", MaxTeachers: 3, MaxDevices: 1, Status: "unused", BatchID: "demo", Note: "本地开发可用激活码"}
	}
}

func (a *App) BootstrapAdmin(username, password, name string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	username = strings.ToLower(strings.TrimSpace(username))
	if !validPassword(password) || len(username) < 3 || len(username) > 32 {
		return bad("管理员用户名需 3–32 位，密码至少 10 位且含字母和数字。")
	}
	for _, char := range username {
		if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_' || char == '-' || char == '.') {
			return bad("管理员用户名仅支持英文字母、数字、点、下划线和短横线。")
		}
	}
	for _, user := range a.state.Users {
		if user.Username == username {
			if user.Role != "super_admin" {
				return bad("引导管理员用户名已被其他账号使用。")
			}
			return nil
		}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "管理员"
	}
	if utf8.RuneCountInString(name) > 24 {
		return bad("管理员落款最多 24 字。")
	}
	user := User{ID: id("u"), Username: username, DisplayName: name, Role: "super_admin", OrgID: id("org"), Status: "active", PasswordHash: passwordHash(password)}
	a.state.Users[user.ID] = user
	if err := a.Save(); err != nil {
		delete(a.state.Users, user.ID)
		return err
	}
	return nil
}

// Successful logins do not consume the failed-password allowance.
func (a *App) loginWait(ip, username string) time.Duration {
	var wait time.Duration
	for key, threshold := range map[string]int{"login:" + ip + ":" + username: 8, "login-ip:" + ip: 30} {
		b := a.limits[key]
		if b.Count >= threshold && time.Until(b.Expires) > wait {
			wait = time.Until(b.Expires)
		}
	}
	return wait
}

func (a *App) loginLockedUntil(username string) string {
	var until time.Time
	for key := range a.limits {
		if strings.HasPrefix(key, "login:") && strings.HasSuffix(key, ":"+username) {
			ip := strings.TrimSuffix(strings.TrimPrefix(key, "login:"), ":"+username)
			if wait := a.loginWait(ip, username); wait > 0 {
				candidate := time.Now().Add(wait)
				if candidate.After(until) {
					until = candidate
				}
			}
		}
	}
	if until.IsZero() {
		return ""
	}
	return until.UTC().Format(time.RFC3339Nano)
}
