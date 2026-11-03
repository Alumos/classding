package app

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

func (a *App) adminClasses(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if !a.isAdmin(u) {
		return nil, forbidden()
	}
	out := []Class{}
	for _, c := range a.state.Classes {
		if a.adminScope(u, c) {
			out = append(out, a.classView(c, u))
		}
	}
	return out, nil
}
func (a *App) adminDevices(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if !a.isAdmin(u) {
		return nil, forbidden()
	}
	out := []Device{}
	for _, d := range a.state.Devices {
		if c, ok := a.state.Classes[d.ClassID]; ok && a.adminScope(u, c) {
			d.Online = a.hub.DeviceOnline(d.ID)
			out = append(out, publicDevice(d))
		}
	}
	return out, nil
}
func (a *App) adminUsers(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if !a.isAdmin(u) {
		return nil, forbidden()
	}
	out := []User{}
	for _, user := range a.state.Users {
		if user.Role == "user" && a.adminUserScope(u, user) {
			view := publicUser(user)
			view.LoginLockedUntil = a.loginLockedUntil(user.Username)
			out = append(out, view)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return out, nil
}
func (a *App) adminUserScope(admin *User, target User) bool {
	return admin.Role == "super_admin" || admin.Role == "school_admin" && admin.OrgID != "" && admin.OrgID == target.OrgID
}

func (a *App) adminCreateUser(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if !a.isAdmin(u) {
		return nil, forbidden()
	}
	var in struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		OrgID       string `json:"org_id"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	in.Username = strings.ToLower(strings.TrimSpace(in.Username))
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.OrgID = strings.TrimSpace(in.OrgID)
	if !validUsername(in.Username) || !validPassword(in.Password) || utf8.RuneCountInString(in.DisplayName) < 1 || utf8.RuneCountInString(in.DisplayName) > 24 || len(in.OrgID) > 128 {
		return nil, bad("用户名需 3–32 位，密码至少 10 位且包含字母和数字，落款 1–24 字。")
	}
	if in.OrgID == "" {
		in.OrgID = u.OrgID
	}
	if u.Role == "school_admin" && (u.OrgID == "" || in.OrgID != u.OrgID) {
		return nil, forbidden()
	}
	if in.OrgID == "" {
		in.OrgID = id("org")
	}
	for _, user := range a.state.Users {
		if user.Username == in.Username {
			return nil, fail(409, "USERNAME_TAKEN", "此用户名已被使用。")
		}
	}
	target := User{ID: id("u"), Username: in.Username, DisplayName: in.DisplayName, Role: "user", OrgID: in.OrgID, Status: "active", PasswordHash: passwordHash(in.Password)}
	a.state.Users[target.ID] = target
	a.audit(u, "admin.user.create", target.ID)
	return publicUser(target), nil
}

func (a *App) managedUser(u *User, userID string) (User, error) {
	if !a.isAdmin(u) {
		return User{}, forbidden()
	}
	target, exists := a.state.Users[userID]
	if !exists {
		return User{}, missing()
	}
	if target.Role != "user" || !a.adminUserScope(u, target) {
		return User{}, forbidden()
	}
	return target, nil
}

func (a *App) adminUpdateUser(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	target, err := a.managedUser(u, chi.URLParam(r, "id"))
	if err != nil {
		return nil, err
	}
	var in struct {
		Username    *string `json:"username"`
		Password    *string `json:"password"`
		DisplayName *string `json:"display_name"`
		Status      *string `json:"status"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if in.Username != nil {
		username := strings.ToLower(strings.TrimSpace(*in.Username))
		if !validUsername(username) {
			return nil, bad("用户名需为 3–32 位，仅支持英文字母、数字、点、下划线和短横线。")
		}
		for _, user := range a.state.Users {
			if user.ID != target.ID && user.Username == username {
				return nil, fail(409, "USERNAME_TAKEN", "此用户名已被使用。")
			}
		}
		target.Username = username
	}
	if in.DisplayName != nil {
		name := strings.TrimSpace(*in.DisplayName)
		if utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 24 {
			return nil, bad("落款需为 1–24 字。")
		}
		target.DisplayName = name
	}
	revoke := false
	if in.Status != nil {
		if *in.Status != "active" && *in.Status != "disabled" {
			return nil, bad("账号状态不正确。")
		}
		revoke = *in.Status != target.Status
		target.Status = *in.Status
	}
	if in.Password != nil {
		if !validPassword(*in.Password) {
			return nil, bad("密码至少 10 位，且包含字母和数字。")
		}
		target.PasswordHash = passwordHash(*in.Password)
		revoke = true
	}
	if revoke {
		target.TokenVersion++
		for key, session := range a.state.Sessions {
			if session.UserID == target.ID {
				delete(a.state.Sessions, key)
			}
		}
		uid := target.ID
		a.deferJob(func() { a.hub.RemoveTeacher(uid) })
	}
	a.state.Users[target.ID] = target
	a.audit(u, "admin.user.update", target.ID)
	return publicUser(target), nil
}

func (a *App) adminDeleteUser(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	target, err := a.managedUser(u, chi.URLParam(r, "id"))
	if err != nil {
		return nil, err
	}
	for _, class := range a.state.Classes {
		if class.OwnerID == target.ID {
			return nil, fail(409, "USER_OWNS_CLASSES", "该教师拥有班级，请先转移或删除其班级。")
		}
	}
	for key, member := range a.state.Members {
		if member.UserID == target.ID {
			delete(a.state.Members, key)
		}
	}
	for key, session := range a.state.Sessions {
		if session.UserID == target.ID {
			delete(a.state.Sessions, key)
		}
	}
	for key, phrase := range a.state.Phrases {
		if phrase.UserID == target.ID {
			delete(a.state.Phrases, key)
		}
	}
	changedClasses := map[string]bool{}
	for key, item := range a.state.Homeworks {
		if item.TeacherID == target.ID {
			delete(a.state.Homeworks, key)
			changedClasses[item.ClassID] = true
		}
	}
	for classID := range changedClasses {
		if err := a.homeworkChanged(classID); err != nil {
			return nil, err
		}
	}
	delete(a.state.Users, target.ID)
	a.deferJob(func() { a.hub.RemoveTeacher(target.ID) })
	a.audit(u, "admin.user.delete", target.ID)
	return ok(), nil
}

func (a *App) managedClass(u *User, classID string) (Class, error) {
	if !a.isAdmin(u) {
		return Class{}, forbidden()
	}
	class, exists := a.state.Classes[classID]
	if !exists {
		return Class{}, missing()
	}
	if !a.adminScope(u, class) {
		return Class{}, forbidden()
	}
	return class, nil
}

func (a *App) classOwner(u *User, ownerID, orgID string) (User, error) {
	owner, exists := a.state.Users[ownerID]
	if !exists || owner.Role != "user" || owner.Status != "active" {
		return User{}, bad("请选择状态正常的教师作为班级负责人。")
	}
	if !a.adminUserScope(u, owner) || orgID != "" && owner.OrgID != orgID {
		return User{}, forbidden()
	}
	if owner.OrgID == "" {
		return User{}, bad("该教师没有所属组织，请联系管理员处理。")
	}
	return owner, nil
}

func (a *App) adminCreateClass(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if !a.isAdmin(u) {
		return nil, forbidden()
	}
	var in struct {
		Name        string `json:"name"`
		Grade       string `json:"grade"`
		OwnerID     string `json:"owner_id"`
		MaxTeachers int    `json:"max_teachers"`
		MaxDevices  int    `json:"max_devices"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	in.Name, in.Grade = strings.TrimSpace(in.Name), strings.TrimSpace(in.Grade)
	if utf8.RuneCountInString(in.Name) < 1 || utf8.RuneCountInString(in.Name) > 40 || utf8.RuneCountInString(in.Grade) > 40 {
		return nil, bad("班级名称需为 1–40 字，年级最多 40 字。")
	}
	if in.MaxTeachers < 1 || in.MaxTeachers > 100 || in.MaxDevices < 1 || in.MaxDevices > 20 {
		return nil, bad("教师名额需为 1–100，大屏名额需为 1–20。")
	}
	owner, err := a.classOwner(u, in.OwnerID, "")
	if err != nil {
		return nil, err
	}
	var code string
	for {
		code = activation()
		if _, exists := a.state.Codes[code]; !exists {
			break
		}
	}
	class := Class{ID: id("c"), Name: in.Name, Grade: in.Grade, OwnerID: owner.ID, OrgID: owner.OrgID, Volume: 75, MaxTeachers: in.MaxTeachers, MaxDevices: in.MaxDevices, CreatedAt: now(), ActivationCode: code}
	stamp := now()
	a.state.Codes[code] = Code{Code: code, MaxTeachers: class.MaxTeachers, MaxDevices: class.MaxDevices, Status: "active", ClassID: &class.ID, OrgID: class.OrgID, BatchID: "admin", Note: "管理员直接创建", RedeemedAt: &stamp}
	a.state.Classes[class.ID] = class
	a.state.Members[class.ID+"|"+owner.ID] = Member{owner.ID, class.ID, owner.DisplayName, owner.Username, "owner", stamp}
	a.audit(u, "admin.class.create", class.ID)
	return a.classView(class, u), nil
}

func (a *App) adminUpdateClass(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	class, err := a.managedClass(u, chi.URLParam(r, "id"))
	if err != nil {
		return nil, err
	}
	var in struct {
		Name        *string `json:"name"`
		Grade       *string `json:"grade"`
		OwnerID     *string `json:"owner_id"`
		MaxTeachers *int    `json:"max_teachers"`
		MaxDevices  *int    `json:"max_devices"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 40 {
			return nil, bad("班级名称需为 1–40 字。")
		}
		class.Name = name
	}
	if in.Grade != nil {
		grade := strings.TrimSpace(*in.Grade)
		if utf8.RuneCountInString(grade) > 40 {
			return nil, bad("年级最多 40 字。")
		}
		class.Grade = grade
	}
	if in.MaxTeachers != nil {
		if *in.MaxTeachers < 1 || *in.MaxTeachers > 100 {
			return nil, bad("教师名额需为 1–100 整数。")
		}
		class.MaxTeachers = *in.MaxTeachers
	}
	if in.MaxDevices != nil {
		if *in.MaxDevices < 1 || *in.MaxDevices > 20 {
			return nil, bad("大屏名额需为 1–20 整数。")
		}
		class.MaxDevices = *in.MaxDevices
	}
	view := a.classView(class, u)
	var newOwner User
	if in.OwnerID != nil && *in.OwnerID != class.OwnerID {
		newOwner, err = a.classOwner(u, *in.OwnerID, class.OrgID)
		if err != nil {
			return nil, err
		}
		if _, exists := a.state.Members[class.ID+"|"+newOwner.ID]; !exists {
			view.TeachersCount++
		}
	}
	if class.MaxTeachers < view.TeachersCount || class.MaxDevices < view.DevicesCount {
		return nil, bad("新上限不能小于当前使用数量。")
	}
	code, exists := a.state.Codes[class.ActivationCode]
	if !exists || code.ClassID == nil || *code.ClassID != class.ID {
		return nil, fail(409, "LICENSE_NOT_FOUND", "班级授权记录不存在，请联系运营方。")
	}
	if newOwner.ID != "" {
		previousKey := class.ID + "|" + class.OwnerID
		if previous, exists := a.state.Members[previousKey]; exists {
			previous.Role = "collaborator"
			a.state.Members[previousKey] = previous
		}
		nextKey := class.ID + "|" + newOwner.ID
		next, exists := a.state.Members[nextKey]
		if !exists {
			next = Member{UserID: newOwner.ID, ClassID: class.ID, DisplayName: newOwner.DisplayName, Username: newOwner.Username, JoinedAt: now()}
		}
		next.Role = "owner"
		a.state.Members[nextKey] = next
		class.OwnerID = newOwner.ID
	}
	code.MaxTeachers, code.MaxDevices = class.MaxTeachers, class.MaxDevices
	a.state.Codes[code.Code] = code
	a.state.Classes[class.ID] = class
	a.audit(u, "admin.class.update", class.ID)
	return a.classView(class, u), nil
}

func (a *App) adminDeleteClass(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	class, err := a.managedClass(u, chi.URLParam(r, "id"))
	if err != nil {
		return nil, err
	}
	a.removeClass(class, u)
	return ok(), nil
}
func (a *App) listCodes(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if !a.isAdmin(u) {
		return nil, forbidden()
	}
	query := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("search")))
	status := r.URL.Query().Get("status")
	out := []Code{}
	for _, code := range a.state.Codes {
		if u.Role != "super_admin" && (u.OrgID == "" || code.OrgID != u.OrgID) {
			continue
		}
		if status != "" && code.Status != status {
			continue
		}
		if query != "" && !strings.Contains(strings.ToUpper(code.Code+" "+code.Note+" "+code.BatchID), query) {
			continue
		}
		out = append(out, code)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}
func (a *App) createCodes(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if u.Role != "super_admin" {
		return nil, forbidden()
	}
	var in struct {
		Count    int     `json:"count"`
		Teachers int     `json:"max_teachers"`
		Devices  int     `json:"max_devices"`
		Expires  *string `json:"expires_at"`
		OrgID    string  `json:"org_id"`
		Note     string  `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if in.Count < 1 || in.Count > 1000 || in.Teachers < 1 || in.Teachers > 100 || in.Devices < 1 || in.Devices > 20 || len(in.Note) > 1000 {
		return nil, bad("数量 1–1000，教师名额 1–100，大屏名额 1–20。")
	}
	if in.Expires != nil {
		if _, err := time.Parse(time.RFC3339Nano, *in.Expires); err != nil || elapsed(*in.Expires) {
			return nil, bad("有效期必须为未来时间。")
		}
	}
	batch := id("batch")
	out := []Code{}
	for i := 0; i < in.Count; i++ {
		var code string
		for {
			code = activation()
			if _, exists := a.state.Codes[code]; !exists {
				break
			}
		}
		c := Code{Code: code, MaxTeachers: in.Teachers, MaxDevices: in.Devices, ExpiresAt: in.Expires, Status: "unused", OrgID: in.OrgID, BatchID: batch, Note: in.Note}
		a.state.Codes[code] = c
		out = append(out, c)
	}
	a.audit(u, "license.create", batch)
	return out, nil
}
func (a *App) updateCode(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if u.Role != "super_admin" {
		return nil, forbidden()
	}
	key := chi.URLParam(r, "code")
	code, exists := a.state.Codes[key]
	if !exists {
		return nil, missing()
	}
	var in map[string]any
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if value, ok := in["max_teachers"]; ok {
		n, ok := value.(float64)
		if !ok || n < 1 || n > 100 || n != float64(int(n)) {
			return nil, bad("教师名额需为 1–100 整数。")
		}
		code.MaxTeachers = int(n)
	}
	if value, ok := in["max_devices"]; ok {
		n, ok := value.(float64)
		if !ok || n < 1 || n > 20 || n != float64(int(n)) {
			return nil, bad("大屏名额需为 1–20 整数。")
		}
		code.MaxDevices = int(n)
	}
	if value, ok := in["expires_at"]; ok {
		if value == nil {
			code.ExpiresAt = nil
		} else {
			stamp, valid := value.(string)
			if !valid {
				return nil, bad("有效期格式不正确。")
			}
			if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
				return nil, bad("有效期格式不正确。")
			}
			code.ExpiresAt = &stamp
		}
	}
	if value, ok := in["status"]; ok {
		status, ok := value.(string)
		if !ok || status != "revoked" && status != "active" && status != "unused" {
			return nil, bad("授权状态不正确。")
		}
		if code.ClassID != nil && status == "unused" {
			return nil, bad("已核销激活码不能直接改为未使用。")
		}
		if code.ClassID == nil && status == "active" {
			return nil, bad("未绑定班级的激活码无法激活。")
		}
		code.Status = status
	}
	if code.ClassID != nil {
		if c, exists := a.state.Classes[*code.ClassID]; exists {
			view := a.classView(c, u)
			if code.MaxTeachers < view.TeachersCount || code.MaxDevices < view.DevicesCount {
				return nil, bad("新上限不能小于当前使用数量。")
			}
			c.MaxTeachers = code.MaxTeachers
			c.MaxDevices = code.MaxDevices
			a.state.Classes[c.ID] = c
			if code.Status == "revoked" || code.ExpiresAt != nil && elapsed(*code.ExpiresAt) {
				for _, m := range a.state.Messages {
					for _, t := range m.Targets {
						if t.ClassID == c.ID && t.DeviceID != "" {
							deviceID, messageID := t.DeviceID, m.ID
							a.deferJob(func() { a.hub.Device(deviceID, Frame{T: "recall", D: map[string]any{"message_id": messageID}}) })
						}
					}
				}
			}
		}
	}
	a.state.Codes[key] = code
	a.audit(u, "license.adjust", key)
	return code, nil
}
func (a *App) adminMessages(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if !a.isAdmin(u) {
		return nil, forbidden()
	}
	a.audit(u, "admin.message.read", r.URL.Query().Get("class_id"))
	scoped := *u
	scoped.ID = ""
	return a.messageList(&scoped, r.URL.Query().Get("class_id"), r.URL.Query().Get("cursor")), nil
}
func (a *App) auditLogs(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if u.Role != "super_admin" {
		return nil, forbidden()
	}
	out := []Audit{}
	for _, entry := range a.state.Audit {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At > out[j].At })
	if len(out) > 500 {
		out = out[:500]
	}
	return out, nil
}
func (a *App) overview(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if !a.isAdmin(u) {
		return nil, forbidden()
	}
	online, deviceCount, classCount, userCount, todayCount, totalTargets, delivered := 0, 0, 0, 0, 0, 0, 0
	var latency float64
	latencyCount := 0
	for _, c := range a.state.Classes {
		if a.adminScope(u, c) {
			classCount++
		}
	}
	for _, d := range a.state.Devices {
		if c, exists := a.state.Classes[d.ClassID]; exists && a.adminScope(u, c) {
			deviceCount++
			if a.hub.DeviceOnline(d.ID) {
				online++
			}
		}
	}
	for _, user := range a.state.Users {
		if a.adminUserScope(u, user) {
			userCount++
		}
	}
	counts := map[string]int{}
	recent := []Message{}
	today := time.Now().Format("2006-01-02")
	for _, m := range a.state.Messages {
		scoped := *u
		scoped.ID = ""
		view, visible := a.messageView(m, &scoped)
		if !visible {
			continue
		}
		recent = append(recent, view)
		created, _ := time.Parse(time.RFC3339Nano, m.CreatedAt)
		date := created.Local().Format("2006-01-02")
		counts[date]++
		if date == today {
			todayCount++
		}
		for _, target := range view.Targets {
			totalTargets++
			if target.DeliveredAt != nil {
				delivered++
				at, _ := time.Parse(time.RFC3339Nano, *target.DeliveredAt)
				duration := float64(at.Sub(created).Milliseconds())
				if duration >= 0 {
					latency += duration
					latencyCount++
				}
			}
		}
	}
	sort.Slice(recent, func(i, j int) bool { return recent[i].CreatedAt > recent[j].CreatedAt })
	if len(recent) > 6 {
		recent = recent[:6]
	}
	activity := []map[string]any{}
	for days := 6; days >= 0; days-- {
		date := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
		activity = append(activity, map[string]any{"date": date, "count": counts[date]})
	}
	rate := float64(0)
	if totalTargets > 0 {
		rate = float64(delivered) / float64(totalTargets)
	}
	if latencyCount > 0 {
		latency /= float64(latencyCount)
	}
	return map[string]any{"online_devices": online, "total_devices": deviceCount, "today_messages": todayCount, "delivery_rate": rate, "avg_delivery_ms": latency, "total_users": userCount, "total_classes": classCount, "recent_messages": recent, "activity": activity}, nil
}

func (a *App) adminScope(u *User, c Class) bool {
	return u.Role == "super_admin" || u.Role == "school_admin" && u.OrgID != "" && u.OrgID == c.OrgID
}

func (a *App) adminUnlockLogin(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	target, err := a.managedUser(u, chi.URLParam(r, "id"))
	if err != nil {
		return nil, err
	}
	a.audit(u, "admin.user.unlock_login", target.ID)
	a.deferJob(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		for key := range a.limits {
			if strings.HasPrefix(key, "login:") && strings.HasSuffix(key, ":"+target.Username) {
				ip := strings.TrimSuffix(strings.TrimPrefix(key, "login:"), ":"+target.Username)
				delete(a.limits, key)
				delete(a.limits, "login-ip:"+ip)
			}
		}
	})
	return ok(), nil
}
