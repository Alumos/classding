package app

import (
	"encoding/base64"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	sessionTTL   = 30 * 24 * time.Hour
	refreshGrace = 30 * time.Second
	minUIScale   = 85
	maxUIScale   = 150
)

// Each workspace keeps its own refresh cookie, so a teacher and an
// administrator can be signed in side by side in one browser.
const (
	scopeTeacher = "teacher"
	scopeAdmin   = "admin"
)

func authScope(u User) string {
	if u.Role == RoleTeacher {
		return scopeTeacher
	}
	return scopeAdmin
}

func refreshCookieName(scope string) string { return "classding_refresh_" + scope }

func requestScope(r *http.Request) string {
	switch scope := r.Header.Get("X-ClassDing-Auth-Scope"); scope {
	case scopeAdmin, scopeTeacher:
		return scope
	default:
		return ""
	}
}

// refreshCookie reads the workspace's own cookie. A request without a scope
// accepts either workspace's cookie.
func refreshCookie(r *http.Request) (*http.Cookie, error) {
	candidates := []string{scopeTeacher, scopeAdmin}
	if scope := requestScope(r); scope != "" {
		candidates = []string{scope}
	}
	for _, candidate := range candidates {
		if cookie, err := r.Cookie(refreshCookieName(candidate)); err == nil {
			return cookie, nil
		}
	}
	return nil, http.ErrNoCookie
}

// secureCookie trusts a forwarded protocol only from a configured proxy.
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

func (a *App) setRefreshCookie(w http.ResponseWriter, r *http.Request, name, value string) {
	age := int(sessionTTL.Seconds())
	if value == "" {
		age = -1
	}
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/api/v1/auth", MaxAge: age, HttpOnly: true, Secure: a.secureCookie(r), SameSite: http.SameSiteStrictMode})
}

func (a *App) newSession(raw string, u User, scope string) {
	key := hash(raw)
	a.state.Sessions[key] = Session{ID: key, UserID: u.ID, ExpiresAt: after(sessionTTL), Version: u.TokenVersion, Scope: scope}
}

func (a *App) signedIn(u User) any {
	return map[string]any{"access_token": a.access(u), "user": u.Public()}
}

// issue starts a refresh session for the account and returns its access token.
func (a *App) issue(w http.ResponseWriter, r *http.Request, u User) any {
	scope := requestScope(r)
	if scope == "" {
		scope = authScope(u)
	}
	raw := random(32)
	a.newSession(raw, u, scope)
	a.setRefreshCookie(w, r, refreshCookieName(scope), raw)
	return a.signedIn(u)
}

// refreshSuccessor derives the next refresh token from the server secret, so
// that simultaneous refreshes converge on one token without storing it.
func (a *App) refreshSuccessor(raw string) string {
	return base64.RawURLEncoding.EncodeToString(sign(a.config.JWTSecret, "classding-refresh-rotation-v1\x00", raw))
}

func (a *App) register(w http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	var in struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if !a.limit("register:"+a.clientIP(r), 10, time.Hour) {
		return nil, tooMany("注册过于频繁，请稍后再试。")
	}
	username, name := normalizeUsername(in.Username), strings.TrimSpace(in.DisplayName)
	if len(username) < 3 || len(username) > 32 || !validPassword(in.Password) || !validDisplayName(name) {
		return nil, bad("用户名需 3–32 位，密码至少 10 位且包含字母和数字，落款 1–24 字。")
	}
	if !validUsername(username) {
		return nil, bad("用户名仅支持英文字母、数字、点、下划线和短横线。")
	}
	if _, taken := a.userByUsername(username); taken {
		return nil, usernameTaken()
	}
	u := User{ID: id("u"), Username: username, DisplayName: name, Role: RoleTeacher, OrgID: id("org"), Status: AccountActive, PasswordHash: freshHash(r, in.Password)}
	a.state.Users[u.ID] = u
	a.audit(&u, "auth.register", u.ID)
	return a.issue(w, r, u), nil
}

func usernameTaken() error { return fail(409, "USERNAME_TAKEN", "此用户名已被使用。") }

// Failed passwords are limited per account and address, and per address alone.
const (
	accountLoginAttempts = 8
	addressLoginAttempts = 30
)

func accountLoginKey(ip, username string) string { return "login:" + ip + ":" + username }
func addressLoginKey(ip string) string           { return "login-ip:" + ip }

func (a *App) login(w http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	username, ip := normalizeUsername(in.Username), a.clientIP(r)
	if wait := a.loginWait(ip, username); wait > 0 {
		seconds := strconv.Itoa(int(wait.Seconds()) + 1)
		w.Header().Set("Retry-After", seconds)
		return nil, tooMany("密码错误次数过多，请在 " + seconds + " 秒后重试，或联系管理员解除登录限制。")
	}
	u, found := a.userByUsername(username)
	encoded := dummyPasswordHash
	if found {
		encoded = u.PasswordHash
	}
	if !checkPassword(r, in.Password, encoded) || !found || !u.Active() {
		a.limit(accountLoginKey(ip, username), accountLoginAttempts, time.Minute)
		a.limit(addressLoginKey(ip), addressLoginAttempts, time.Minute)
		return nil, fail(401, "INVALID_CREDENTIALS", "用户名或密码不正确，或账号已被停用。")
	}
	if requestScope(r) == scopeAdmin && !u.IsAdmin() {
		return nil, forbidden()
	}
	delete(a.limits, accountLoginKey(ip, username))
	a.audit(&u, "auth.login", u.ID)
	return a.issue(w, r, u), nil
}

// loginWait is how long the address must wait before trying the account again.
// Successful logins do not consume the allowance.
func (a *App) loginWait(ip, username string) time.Duration {
	var wait time.Duration
	for key, threshold := range map[string]int{accountLoginKey(ip, username): accountLoginAttempts, addressLoginKey(ip): addressLoginAttempts} {
		if b := a.limits[key]; b.Count >= threshold && time.Until(b.Expires) > wait {
			wait = time.Until(b.Expires)
		}
	}
	return wait
}

// loginAddresses lists the addresses that have failed to sign in to the account.
func (a *App) loginAddresses(username string) []string {
	var addresses []string
	for key := range a.limits {
		if rest, found := strings.CutPrefix(key, "login:"); found {
			if ip, found := strings.CutSuffix(rest, ":"+username); found {
				addresses = append(addresses, ip)
			}
		}
	}
	return addresses
}

func (a *App) loginLockedUntil(username string) string {
	var until time.Time
	for _, ip := range a.loginAddresses(username) {
		if wait := a.loginWait(ip, username); wait > 0 && time.Now().Add(wait).After(until) {
			until = time.Now().Add(wait)
		}
	}
	if until.IsZero() {
		return ""
	}
	return stamp(until)
}

func (a *App) refresh(w http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	cookie, err := refreshCookie(r)
	if err != nil {
		return nil, unauthorized("请先登录。")
	}
	raw := cookie.Value
	session, exists := a.state.Sessions[hash(raw)]
	if !exists || elapsed(session.ExpiresAt) {
		return nil, unauthorized("登录已过期。")
	}
	scope := requestScope(r)
	u, exists := a.state.Users[session.UserID]
	if !exists || !u.Active() || u.TokenVersion != session.Version ||
		scope == scopeAdmin && !u.IsAdmin() ||
		session.Scope != "" && scope != "" && session.Scope != scope {
		return nil, unauthorized("账号状态已改变，请重新登录。")
	}
	if session.Scope != "" {
		scope = session.Scope
	} else if scope == "" {
		scope = authScope(u)
	}

	if session.RotationUntil == "" {
		session.RotationUntil = after(refreshGrace)
		a.state.Sessions[session.ID] = session
		raw = a.refreshSuccessor(raw)
		a.newSession(raw, u, scope)
	} else {
		// Follow concurrent rotations to the latest successor. Every link must
		// still be within its grace period and belong to the same account.
		for session.RotationUntil != "" {
			if elapsed(session.RotationUntil) {
				return nil, unauthorized("登录已过期。")
			}
			raw = a.refreshSuccessor(raw)
			session, exists = a.state.Sessions[hash(raw)]
			if !exists || session.UserID != u.ID || session.Version != u.TokenVersion || elapsed(session.ExpiresAt) {
				return nil, unauthorized("登录已过期。")
			}
		}
	}
	a.setRefreshCookie(w, r, refreshCookieName(scope), raw)
	return a.signedIn(u), nil
}

// logout ends every session of the account named by the refresh cookie or the
// bearer token. An admin-scoped request never signs out a teacher account.
func (a *App) logout(w http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	scope := requestScope(r)
	permitted := func(userID string) bool {
		u, exists := a.state.Users[userID]
		return exists && (scope != scopeAdmin || u.IsAdmin())
	}
	var userID string
	cookie, cookieErr := refreshCookie(r)
	if cookieErr == nil {
		if session, exists := a.state.Sessions[hash(cookie.Value)]; exists && permitted(session.UserID) {
			userID = session.UserID
		}
	}
	if c, err := a.verify(bearerToken(r)); err == nil && permitted(c.Subject) {
		userID = c.Subject
	}
	if userID != "" {
		u := a.state.Users[userID]
		a.revokeSessions(&u)
		a.state.Users[userID] = u
	}
	switch {
	case cookieErr == nil:
		a.setRefreshCookie(w, r, cookie.Name, "")
	case scope != "":
		a.setRefreshCookie(w, r, refreshCookieName(scope), "")
	}
	return ok(), nil
}

func (a *App) updateMe(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in struct {
		DisplayName *string `json:"display_name"`
		UIScale     *int    `json:"ui_scale"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if in.DisplayName == nil && in.UIScale == nil {
		return nil, bad("没有需要保存的内容。")
	}
	if in.DisplayName != nil {
		name := strings.TrimSpace(*in.DisplayName)
		if !validDisplayName(name) {
			return nil, bad("落款需为 1–24 字。")
		}
		u.DisplayName = name
	}
	if in.UIScale != nil {
		if !inRange(*in.UIScale, minUIScale, maxUIScale) {
			return nil, bad("界面字号需在 85% 至 150% 之间。")
		}
		u.UIScale = *in.UIScale
	}
	a.state.Users[u.ID] = *u
	return u.Public(), nil
}

func (a *App) changePassword(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if !a.limit("password:"+u.ID, 5, time.Minute) {
		return nil, tooMany("尝试过于频繁。")
	}
	if !checkPassword(r, in.Current, u.PasswordHash) {
		return nil, bad("当前密码不正确。")
	}
	if !validPassword(in.New) {
		return nil, bad("新密码至少 10 位，且包含字母和数字。")
	}
	u.PasswordHash = freshHash(r, in.New)
	a.revokeSessions(u)
	a.state.Users[u.ID] = *u
	a.audit(u, "auth.password", u.ID)
	return ok(), nil
}

// BootstrapAdmin creates the first operator account from the environment.
// It does nothing when an operator with that username already exists.
func (a *App) BootstrapAdmin(username, password, name string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	username, name = normalizeUsername(username), strings.TrimSpace(name)
	if !validPassword(password) || len(username) < 3 || len(username) > 32 {
		return bad("管理员用户名需 3–32 位，密码至少 10 位且含字母和数字。")
	}
	if !validUsername(username) {
		return bad("管理员用户名仅支持英文字母、数字、点、下划线和短横线。")
	}
	if existing, found := a.userByUsername(username); found {
		if !existing.IsSuperAdmin() {
			return bad("引导管理员用户名已被其他账号使用。")
		}
		return nil
	}
	if name == "" {
		name = "管理员"
	}
	if !validDisplayName(name) {
		return bad("管理员落款最多 24 字。")
	}
	u := User{ID: id("u"), Username: username, DisplayName: name, Role: RoleSuperAdmin, OrgID: id("org"), Status: AccountActive, PasswordHash: passwordHash(password)}
	a.state.Users[u.ID] = u
	if err := a.Save(); err != nil {
		delete(a.state.Users, u.ID)
		return err
	}
	return nil
}

// seed adds demo accounts and classes for local development.
func (a *App) seed() {
	accounts := []struct {
		username, name, password string
		role                     Role
	}{
		{"teacher", "林老师", "ClassDing2026!", RoleTeacher},
		{"admin", "管理员", "AdminDing2026!", RoleSuperAdmin},
	}
	if !a.isSchool("org_demo") {
		a.state.Schools["org_demo"] = School{ID: "org_demo", Name: "演示学校", CreatedAt: now()}
	}
	for _, entry := range accounts {
		if _, exists := a.userByUsername(entry.username); exists {
			continue
		}
		u := User{ID: id("u"), Username: entry.username, DisplayName: entry.name, Role: entry.role, OrgID: "org_demo", Status: AccountActive, PasswordHash: passwordHash(entry.password)}
		a.state.Users[u.ID] = u
		if u.Role != RoleTeacher {
			continue
		}
		for _, demo := range []struct{ code, name string }{{"DEMO-A234", "三年级（1）班"}, {"DEMO-B234", "三年级（2）班"}} {
			redeemed := now()
			c := Class{ID: id("c"), Name: demo.name, Grade: "三年级", OwnerID: u.ID, OrgID: u.OrgID, Volume: defaultVolume, MaxTeachers: 3, MaxDevices: 2, CreatedAt: redeemed, ActivationCode: demo.code}
			a.state.Classes[c.ID] = c
			a.addMember(c, u, MemberOwner)
			a.state.Codes[demo.code] = Code{Code: demo.code, MaxTeachers: 3, MaxDevices: 2, Status: LicenseActive, ClassID: &c.ID, OrgID: u.OrgID, BatchID: "demo", Note: "仅本地开发", RedeemedAt: &redeemed}
		}
	}
	if _, exists := a.state.Codes["TEST-2345"]; !exists {
		a.state.Codes["TEST-2345"] = Code{Code: "TEST-2345", MaxTeachers: 3, MaxDevices: 1, Status: LicenseUnused, BatchID: "demo", Note: "本地开发可用激活码"}
	}
}
