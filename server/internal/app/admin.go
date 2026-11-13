package app

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// Every handler here is routed with adminOnly or superAdminOnly; a school
// administrator only ever sees records of their own organization.

func (a *App) adminClasses(_ http.ResponseWriter, _ *http.Request, u *User) (any, error) {
	return a.classViews(u, governedBy(u)), nil
}

func (a *App) adminDevices(_ http.ResponseWriter, _ *http.Request, u *User) (any, error) {
	return a.deviceViews(func(d Device) bool {
		c, exists := a.state.Classes[d.ClassID]
		return exists && u.Governs(c.OrgID)
	}), nil
}

func (a *App) adminUsers(_ http.ResponseWriter, _ *http.Request, u *User) (any, error) {
	out := []User{}
	for _, user := range a.state.Users {
		if user.Role == RoleTeacher && u.Governs(user.OrgID) {
			view := user.Public()
			view.LoginLockedUntil = a.loginLockedUntil(user.Username)
			out = append(out, view)
		}
	}
	slices.SortFunc(out, func(x, y User) int { return strings.Compare(x.Username, y.Username) })
	return out, nil
}

func (a *App) adminCreateUser(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		OrgID       string `json:"org_id"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	username, name, orgID := normalizeUsername(in.Username), strings.TrimSpace(in.DisplayName), strings.TrimSpace(in.OrgID)
	if !validUsername(username) || !validPassword(in.Password) || !validDisplayName(name) || len(orgID) > 128 {
		return nil, bad("用户名需 3–32 位，密码至少 10 位且包含字母和数字，落款 1–24 字。")
	}
	// A school administrator adds teachers to their own school; the operator
	// picks a school or leaves the teacher without one.
	switch {
	case !u.IsSuperAdmin() && (orgID == "" || orgID == u.OrgID) && u.OrgID != "":
		orgID = u.OrgID
	case !u.IsSuperAdmin():
		return nil, forbidden()
	case orgID == "":
		orgID = id("org")
	case !a.isSchool(orgID):
		return nil, bad("请选择已有的学校。")
	}
	if _, taken := a.userByUsername(username); taken {
		return nil, usernameTaken()
	}
	teacher := User{ID: id("u"), Username: username, DisplayName: name, Role: RoleTeacher, OrgID: orgID, Status: AccountActive, PasswordHash: freshHash(r, in.Password)}
	a.state.Users[teacher.ID] = teacher
	a.audit(u, "admin.user.create", teacher.ID)
	return teacher.Public(), nil
}

// managedTeacher loads a teacher account the administrator governs.
func (a *App) managedTeacher(u *User, userID string) (User, error) {
	teacher, exists := a.state.Users[userID]
	if !exists {
		return User{}, missing()
	}
	if teacher.Role != RoleTeacher || !u.Governs(teacher.OrgID) {
		return User{}, forbidden()
	}
	return teacher, nil
}

func (a *App) adminUpdateUser(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	teacher, err := a.managedTeacher(u, chi.URLParam(r, "id"))
	if err != nil {
		return nil, err
	}
	var in struct {
		Username    *string        `json:"username"`
		Password    *string        `json:"password"`
		DisplayName *string        `json:"display_name"`
		Status      *AccountStatus `json:"status"`
		OrgID       *string        `json:"org_id"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if in.OrgID != nil && *in.OrgID != teacher.OrgID {
		if !u.IsSuperAdmin() {
			return nil, forbidden()
		}
		orgID := *in.OrgID
		if orgID == "" {
			orgID = id("org")
		} else if !a.isSchool(orgID) {
			return nil, bad("请选择已有的学校。")
		}
		a.moveTeacher(&teacher, orgID)
	}
	if in.Username != nil {
		username := normalizeUsername(*in.Username)
		if !validUsername(username) {
			return nil, bad("用户名需为 3–32 位，仅支持英文字母、数字、点、下划线和短横线。")
		}
		if other, taken := a.userByUsername(username); taken && other.ID != teacher.ID {
			return nil, usernameTaken()
		}
		teacher.Username = username
	}
	if in.DisplayName != nil {
		name := strings.TrimSpace(*in.DisplayName)
		if !validDisplayName(name) {
			return nil, bad("落款需为 1–24 字。")
		}
		teacher.DisplayName = name
	}
	revoke := false
	if in.Status != nil {
		if *in.Status != AccountActive && *in.Status != AccountDisabled {
			return nil, bad("账号状态不正确。")
		}
		revoke = *in.Status != teacher.Status
		teacher.Status = *in.Status
	}
	if in.Password != nil {
		if !validPassword(*in.Password) {
			return nil, bad("密码至少 10 位，且包含字母和数字。")
		}
		teacher.PasswordHash = freshHash(r, *in.Password)
		revoke = true
	}
	if revoke {
		a.revokeSessions(&teacher)
	}
	a.state.Users[teacher.ID] = teacher
	a.audit(u, "admin.user.update", teacher.ID)
	return teacher.Public(), nil
}

// adminDeleteUser removes a teacher who owns no class, with their memberships,
// sessions, phrases and homework.
func (a *App) adminDeleteUser(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	teacher, err := a.managedTeacher(u, chi.URLParam(r, "id"))
	if err != nil {
		return nil, err
	}
	for _, class := range a.state.Classes {
		if class.OwnerID == teacher.ID {
			return nil, fail(409, "USER_OWNS_CLASSES", "该教师拥有班级，请先转移或删除其班级。")
		}
	}
	for key, member := range a.state.Members {
		if member.UserID == teacher.ID {
			delete(a.state.Members, key)
		}
	}
	for key, phrase := range a.state.Phrases {
		if phrase.UserID == teacher.ID {
			delete(a.state.Phrases, key)
		}
	}
	changed := map[string]bool{}
	for key, item := range a.state.Homeworks {
		if item.TeacherID == teacher.ID {
			delete(a.state.Homeworks, key)
			changed[item.ClassID] = true
		}
	}
	for classID := range changed {
		if err := a.homeworkChanged(classID); err != nil {
			return nil, err
		}
	}
	a.revokeSessions(&teacher)
	delete(a.state.Users, teacher.ID)
	a.audit(u, "admin.user.delete", teacher.ID)
	return ok(), nil
}

func (a *App) adminUnlockLogin(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	teacher, err := a.managedTeacher(u, chi.URLParam(r, "id"))
	if err != nil {
		return nil, err
	}
	a.audit(u, "admin.user.unlock_login", teacher.ID)
	// Limits are not part of the transaction, so clear them only once it commits.
	a.deferJob(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		for _, ip := range a.loginAddresses(teacher.Username) {
			delete(a.limits, accountLoginKey(ip, teacher.Username))
			delete(a.limits, addressLoginKey(ip))
		}
	})
	return ok(), nil
}

func (a *App) managedClass(u *User, classID string) (Class, error) {
	class, exists := a.state.Classes[classID]
	if !exists {
		return Class{}, missing()
	}
	if !u.Governs(class.OrgID) {
		return Class{}, forbidden()
	}
	return class, nil
}

// classOwner checks that a teacher can be put in charge of a class of orgID;
// an empty orgID accepts any organization the administrator governs.
func (a *App) classOwner(u *User, ownerID, orgID string) (User, error) {
	owner, exists := a.state.Users[ownerID]
	if !exists || owner.Role != RoleTeacher || !owner.Active() {
		return User{}, bad("请选择状态正常的教师作为班级负责人。")
	}
	if !u.Governs(owner.OrgID) || orgID != "" && owner.OrgID != orgID {
		return User{}, forbidden()
	}
	if owner.OrgID == "" {
		return User{}, bad("该教师没有所属组织，请联系管理员处理。")
	}
	return owner, nil
}

func validQuota(teachers, devices int) bool {
	return inRange(teachers, 1, maxTeachers) && inRange(devices, 1, maxDevices)
}

func (a *App) unusedCode() string {
	code := activation()
	for _, taken := a.state.Codes[code]; taken; _, taken = a.state.Codes[code] {
		code = activation()
	}
	return code
}

// adminCreateClass creates a class directly, with a license issued on the spot.
func (a *App) adminCreateClass(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
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
	name, grade := strings.TrimSpace(in.Name), strings.TrimSpace(in.Grade)
	if !validClassName(name) || !runesBetween(grade, 0, maxClassName) {
		return nil, bad("班级名称需为 1–40 字，年级最多 40 字。")
	}
	if !validQuota(in.MaxTeachers, in.MaxDevices) {
		return nil, bad("教师名额需为 1–100，大屏名额需为 1–20。")
	}
	owner, err := a.classOwner(u, in.OwnerID, "")
	if err != nil {
		return nil, err
	}
	code := Code{Code: a.unusedCode(), MaxTeachers: in.MaxTeachers, MaxDevices: in.MaxDevices, BatchID: "admin", Note: "管理员直接创建"}
	class := a.activateClass(code, Class{Name: name, Grade: grade, OwnerID: owner.ID, OrgID: owner.OrgID}, owner)
	a.audit(u, "admin.class.create", class.ID)
	return a.classView(class, u), nil
}

func (a *App) adminUpdateClass(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
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
		if !validClassName(name) {
			return nil, bad("班级名称需为 1–40 字。")
		}
		class.Name = name
	}
	if in.Grade != nil {
		grade := strings.TrimSpace(*in.Grade)
		if !runesBetween(grade, 0, maxClassName) {
			return nil, bad("年级最多 40 字。")
		}
		class.Grade = grade
	}
	if in.MaxTeachers != nil {
		if !inRange(*in.MaxTeachers, 1, maxTeachers) {
			return nil, bad("教师名额需为 1–100 整数。")
		}
		class.MaxTeachers = *in.MaxTeachers
	}
	if in.MaxDevices != nil {
		if !inRange(*in.MaxDevices, 1, maxDevices) {
			return nil, bad("大屏名额需为 1–20 整数。")
		}
		class.MaxDevices = *in.MaxDevices
	}

	teachers := a.teacherCount(class.ID)
	var owner *User
	if in.OwnerID != nil && *in.OwnerID != class.OwnerID {
		next, err := a.classOwner(u, *in.OwnerID, class.OrgID)
		if err != nil {
			return nil, err
		}
		if _, member := a.state.Members[memberKey(class.ID, next.ID)]; !member {
			teachers++
		}
		owner = &next
	}
	if class.MaxTeachers < teachers || class.MaxDevices < a.deviceCount(class.ID) {
		return nil, bad("新上限不能小于当前使用数量。")
	}
	code, exists := a.state.Codes[class.ActivationCode]
	if !exists || code.ClassID == nil || *code.ClassID != class.ID {
		return nil, fail(409, "LICENSE_NOT_FOUND", "班级授权记录不存在，请联系运营方。")
	}
	if owner != nil {
		if _, member := a.state.Members[memberKey(class.ID, owner.ID)]; !member {
			a.addMember(class, *owner, MemberCollaborator)
		}
		class = a.changeOwner(class, owner.ID)
	}
	code.MaxTeachers, code.MaxDevices = class.MaxTeachers, class.MaxDevices
	a.state.Codes[code.Code] = code
	a.state.Classes[class.ID] = class
	a.audit(u, "admin.class.update", class.ID)
	return a.classView(class, u), nil
}

func (a *App) adminDeleteClass(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	class, err := a.managedClass(u, chi.URLParam(r, "id"))
	if err != nil {
		return nil, err
	}
	a.removeClass(class, u)
	return ok(), nil
}

// configureDevice sends a screen to another server, or cancels a pending move
// when the address is empty.
func (a *App) configureDevice(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	d, exists := a.state.Devices[chi.URLParam(r, "id")]
	if !exists {
		return nil, missing()
	}
	var in struct {
		ServerURL *string `json:"server_url"`
	}
	if err := decodeStrict(r, &in); err != nil {
		return nil, err
	}
	if in.ServerURL == nil || *in.ServerURL != "" && !validServerURL(*in.ServerURL) {
		return nil, bad("迁移地址必须是 HTTP 或 HTTPS 站点根地址。")
	}
	d.RemoteConfig = nil
	if *in.ServerURL != "" {
		d.RemoteConfig = &RemoteConfig{ServerURL: strings.TrimSuffix(*in.ServerURL, "/"), Revision: id("rev")}
	}
	a.state.Devices[d.ID] = d
	a.audit(u, "admin.device.config", d.ID)
	a.deferJob(func() { a.hub.Device(d.ID, Frame{T: "config", D: map[string]any{"remote_config": d.RemoteConfig}}) })
	return a.deviceView(d), nil
}

func (a *App) listCodes(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	query := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("search")))
	status := LicenseStatus(r.URL.Query().Get("status"))
	out := []Code{}
	for _, code := range a.state.Codes {
		if !u.IsSuperAdmin() && (u.OrgID == "" || code.OrgID != u.OrgID) ||
			status != "" && code.Status != status ||
			query != "" && !strings.Contains(strings.ToUpper(code.Code+" "+code.Note+" "+code.BatchID), query) {
			continue
		}
		out = append(out, code)
	}
	slices.SortFunc(out, func(x, y Code) int { return strings.Compare(x.Code, y.Code) })
	return out, nil
}

func (a *App) createCodes(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in struct {
		Count       int     `json:"count"`
		MaxTeachers int     `json:"max_teachers"`
		MaxDevices  int     `json:"max_devices"`
		ExpiresAt   *string `json:"expires_at"`
		OrgID       string  `json:"org_id"`
		Note        string  `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if !inRange(in.Count, 1, 1000) || !validQuota(in.MaxTeachers, in.MaxDevices) || len(in.Note) > 1000 {
		return nil, bad("数量 1–1000，教师名额 1–100，大屏名额 1–20。")
	}
	if in.ExpiresAt != nil && elapsed(*in.ExpiresAt) {
		return nil, bad("有效期必须为未来时间。")
	}
	if in.OrgID != "" && !a.isSchool(in.OrgID) {
		return nil, bad("请选择已有的学校。")
	}
	batch := id("batch")
	out := make([]Code, in.Count)
	for i := range out {
		out[i] = Code{Code: a.unusedCode(), MaxTeachers: in.MaxTeachers, MaxDevices: in.MaxDevices, ExpiresAt: in.ExpiresAt, Status: LicenseUnused, OrgID: in.OrgID, BatchID: batch, Note: in.Note}
		a.state.Codes[out[i].Code] = out[i]
	}
	a.audit(u, "license.create", batch)
	return out, nil
}

// deleteCodes removes licenses that no class uses. A license in use must be
// freed by deleting its class first, so that no class loses its license.
func (a *App) deleteCodes(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in struct {
		Codes []string `json:"codes"`
	}
	if err := decodeStrict(r, &in); err != nil {
		return nil, err
	}
	codes := unique(in.Codes)
	if !inRange(len(codes), 1, 1000) {
		return nil, bad("请选择 1–1000 个激活码。")
	}
	for _, key := range codes {
		code, exists := a.state.Codes[key]
		if !exists {
			return nil, missing()
		}
		if code.ClassID != nil {
			return nil, fail(409, "LICENSE_IN_USE", "激活码 "+key+" 正在被班级使用，请先删除该班级后再删除激活码。")
		}
	}
	for _, key := range codes {
		delete(a.state.Codes, key)
		a.audit(u, "license.delete", key)
	}
	return map[string]int{"deleted": len(codes)}, nil
}

// updateCode adjusts a license. Its quota follows its class, and revoking or
// expiring it recalls the class's notifications from every screen.
func (a *App) updateCode(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	key := chi.URLParam(r, "code")
	code, exists := a.state.Codes[key]
	if !exists {
		return nil, missing()
	}
	// A JSON null clears the expiry, so presence matters as much as the value.
	var in map[string]any
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	quota := func(field string, max int, current *int, message string) error {
		value, present := in[field]
		if !present {
			return nil
		}
		n, isNumber := value.(float64)
		if !isNumber || n != float64(int(n)) || !inRange(int(n), 1, max) {
			return bad(message)
		}
		*current = int(n)
		return nil
	}
	if err := quota("max_teachers", maxTeachers, &code.MaxTeachers, "教师名额需为 1–100 整数。"); err != nil {
		return nil, err
	}
	if err := quota("max_devices", maxDevices, &code.MaxDevices, "大屏名额需为 1–20 整数。"); err != nil {
		return nil, err
	}
	if value, present := in["expires_at"]; present {
		code.ExpiresAt = nil
		if value != nil {
			expires, isString := value.(string)
			if _, valid := parseTime(expires); !isString || !valid {
				return nil, bad("有效期格式不正确。")
			}
			code.ExpiresAt = &expires
		}
	}
	if value, present := in["status"]; present {
		raw, _ := value.(string)
		status := LicenseStatus(raw)
		if status != LicenseRevoked && status != LicenseActive && status != LicenseUnused {
			return nil, bad("授权状态不正确。")
		}
		if code.ClassID != nil && status == LicenseUnused {
			return nil, bad("已核销激活码不能直接改为未使用。")
		}
		if code.ClassID == nil && status == LicenseActive {
			return nil, bad("未绑定班级的激活码无法激活。")
		}
		code.Status = status
	}
	if code.ClassID != nil {
		if c, exists := a.state.Classes[*code.ClassID]; exists {
			if code.MaxTeachers < a.teacherCount(c.ID) || code.MaxDevices < a.deviceCount(c.ID) {
				return nil, bad("新上限不能小于当前使用数量。")
			}
			c.MaxTeachers, c.MaxDevices = code.MaxTeachers, code.MaxDevices
			a.state.Classes[c.ID] = c
			if code.Status == LicenseRevoked || code.Expired() {
				a.recallClass(c.ID)
			}
		}
	}
	a.state.Codes[key] = code
	a.audit(u, "license.adjust", key)
	return code, nil
}

// recallClass withdraws every notification of the class from its screens.
func (a *App) recallClass(classID string) {
	for _, m := range a.state.Messages {
		targets := slices.DeleteFunc(slices.Clone(m.Targets), func(t Target) bool { return t.ClassID != classID || t.DeviceID == "" })
		if len(targets) > 0 {
			a.recallFrom(m.ID, targets)
		}
	}
}

func (a *App) adminMessages(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	a.audit(u, "admin.message.read", r.URL.Query().Get("class_id"))
	return a.messageList(governedBy(u), r.URL.Query().Get("class_id"), r.URL.Query().Get("cursor"))
}

func (a *App) auditLogs(_ http.ResponseWriter, _ *http.Request, _ *User) (any, error) {
	out := make([]Audit, 0, len(a.state.Audit))
	for _, entry := range a.state.Audit {
		out = append(out, entry)
	}
	slices.SortFunc(out, func(x, y Audit) int { return strings.Compare(y.At, x.At) })
	return out[:min(len(out), 500)], nil
}

type overview struct {
	OnlineDevices  int          `json:"online_devices"`
	TotalDevices   int          `json:"total_devices"`
	TodayMessages  int          `json:"today_messages"`
	DeliveryRate   float64      `json:"delivery_rate"`
	AvgDeliveryMS  float64      `json:"avg_delivery_ms"`
	TotalUsers     int          `json:"total_users"`
	TotalClasses   int          `json:"total_classes"`
	RecentMessages []Message    `json:"recent_messages"`
	Activity       []dailyCount `json:"activity"`
}

type dailyCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

func (a *App) overview(_ http.ResponseWriter, _ *http.Request, u *User) (any, error) {
	const day = "2006-01-02"
	var o overview
	for _, c := range a.state.Classes {
		if u.Governs(c.OrgID) {
			o.TotalClasses++
		}
	}
	for _, d := range a.state.Devices {
		if c, exists := a.state.Classes[d.ClassID]; exists && u.Governs(c.OrgID) {
			o.TotalDevices++
			if a.hub.DeviceOnline(d.ID) {
				o.OnlineDevices++
			}
		}
	}
	for _, user := range a.state.Users {
		if u.Governs(user.OrgID) {
			o.TotalUsers++
		}
	}

	perDay := map[string]int{}
	today := time.Now().Format(day)
	targets, delivered, latencyCount := 0, 0, 0
	o.RecentMessages = []Message{}
	for _, m := range a.state.Messages {
		view, visible := a.messageView(m, governedBy(u))
		if !visible {
			continue
		}
		o.RecentMessages = append(o.RecentMessages, view)
		created, _ := parseTime(m.CreatedAt)
		date := created.Local().Format(day)
		perDay[date]++
		if date == today {
			o.TodayMessages++
		}
		for _, t := range view.Targets {
			targets++
			if t.DeliveredAt == nil {
				continue
			}
			delivered++
			if at, _ := parseTime(*t.DeliveredAt); !at.Before(created) {
				o.AvgDeliveryMS += float64(at.Sub(created).Milliseconds())
				latencyCount++
			}
		}
	}
	slices.SortFunc(o.RecentMessages, func(x, y Message) int { return strings.Compare(y.CreatedAt, x.CreatedAt) })
	o.RecentMessages = o.RecentMessages[:min(len(o.RecentMessages), 6)]
	for days := 6; days >= 0; days-- {
		date := time.Now().AddDate(0, 0, -days).Format(day)
		o.Activity = append(o.Activity, dailyCount{date, perDay[date]})
	}
	if targets > 0 {
		o.DeliveryRate = float64(delivered) / float64(targets)
	}
	if latencyCount > 0 {
		o.AvgDeliveryMS /= float64(latencyCount)
	}
	return o, nil
}
