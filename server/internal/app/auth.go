package app

import (
	"encoding/base32"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

func (a *App) cookie(w http.ResponseWriter, value string, age int) {
	http.SetCookie(w, &http.Cookie{Name: "classding_refresh", Value: value, Path: "/api/v1/auth", MaxAge: age, HttpOnly: true, Secure: a.config.SecureCookies, SameSite: http.SameSiteStrictMode})
}
func (a *App) issue(w http.ResponseWriter, u User, verified bool) any {
	raw := random(32)
	key := hash(raw)
	a.state.Sessions[key] = Session{key, u.ID, after(30 * 24 * time.Hour), u.TokenVersion, verified}
	a.cookie(w, raw, 30*24*3600)
	return map[string]any{"access_token": a.access(u, verified), "user": publicUser(u)}
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
	return a.issue(w, u, false), nil
}
func (a *App) login(w http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	var in struct {
		Username, Password string
		TOTP               string `json:"totp_code"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	username := strings.ToLower(strings.TrimSpace(in.Username))
	if !a.limit("login:"+a.clientIP(r)+":"+username, 8, time.Minute) || !a.limit("login-ip:"+a.clientIP(r), 30, time.Minute) {
		return nil, fail(429, "RATE_LIMITED", "登录尝试过于频繁，请稍后再试。")
	}
	var found User
	for _, u := range a.state.Users {
		if u.Username == username {
			found = u
			break
		}
	}
	if found.ID == "" || !passwordMatches(in.Password, found.PasswordHash) || found.Status != "active" {
		return nil, fail(401, "INVALID_CREDENTIALS", "用户名或密码不正确，或账号已被停用。")
	}
	verified := false
	if found.Role != "user" {
		if a.config.AdminTOTPSecret == "" {
			if !a.config.DevSeed {
				return nil, fail(403, "TOTP_NOT_CONFIGURED", "管理员尚未配置二次验证，请联系运营方。")
			}
			verified = true
		} else {
			if !validTOTP(a.config.AdminTOTPSecret, in.TOTP) {
				return nil, fail(401, "TOTP_REQUIRED", "请输入有效的 6 位动态验证码。")
			}
			verified = true
		}
	}
	a.audit(&found, "auth.login", found.ID)
	return a.issue(w, found, verified), nil
}
func (a *App) refresh(w http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	cookie, err := r.Cookie("classding_refresh")
	if err != nil {
		return nil, fail(401, "UNAUTHORIZED", "请先登录。")
	}
	key := hash(cookie.Value)
	session, exists := a.state.Sessions[key]
	if !exists || elapsed(session.ExpiresAt) {
		a.cookie(w, "", -1)
		return nil, fail(401, "UNAUTHORIZED", "登录已过期。")
	}
	u, exists := a.state.Users[session.UserID]
	if !exists || u.Status != "active" || u.TokenVersion != session.Version || u.Role != "user" && !session.AdminVerified {
		return nil, fail(401, "UNAUTHORIZED", "账号状态已改变，请重新登录。")
	}
	delete(a.state.Sessions, key)
	return a.issue(w, u, session.AdminVerified), nil
}
func (a *App) logout(w http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	var uid string
	if cookie, err := r.Cookie("classding_refresh"); err == nil {
		if session, ok := a.state.Sessions[hash(cookie.Value)]; ok {
			uid = session.UserID
		}
	}
	if claims, err := a.verify(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")); err == nil {
		uid = claims.Subject
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
	a.cookie(w, "", -1)
	return ok(), nil
}
func (a *App) updateMe(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in struct {
		Name string `json:"display_name"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	in.Name = strings.TrimSpace(in.Name)
	if utf8.RuneCountInString(in.Name) < 1 || utf8.RuneCountInString(in.Name) > 24 {
		return nil, bad("落款需为 1–24 字。")
	}
	u.DisplayName = in.Name
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
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(a.config.AdminTOTPSecret)))
	if err != nil || len(key) < 16 {
		return bad("生产管理员必须配置至少 128 位的有效 Base32 TOTP 密钥。")
	}
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
