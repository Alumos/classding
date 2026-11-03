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
		if u.Role == "super_admin" || user.OrgID == u.OrgID {
			out = append(out, publicUser(user))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return out, nil
}
func (a *App) adminUpdateUser(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if !a.isAdmin(u) {
		return nil, forbidden()
	}
	target, exists := a.state.Users[chi.URLParam(r, "id")]
	if !exists {
		return nil, missing()
	}
	if u.Role != "super_admin" && (target.OrgID != u.OrgID || target.Role != "user") {
		return nil, forbidden()
	}
	var body map[string]any
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	if value, ok := body["status"]; ok {
		status, ok := value.(string)
		if !ok || status != "active" && status != "disabled" {
			return nil, bad("账号状态不正确。")
		}
		if target.ID == u.ID && status == "disabled" {
			return nil, bad("不能停用当前管理员账号。")
		}
		target.Status = status
		target.TokenVersion++
		uid := target.ID
		a.deferJob(func() { a.hub.RemoveTeacher(uid) })
	}
	if value, ok := body["display_name"]; ok {
		name, ok := value.(string)
		if !ok || utf8.RuneCountInString(strings.TrimSpace(name)) < 1 || utf8.RuneCountInString(name) > 24 {
			return nil, bad("落款需为 1–24 字。")
		}
		target.DisplayName = strings.TrimSpace(name)
	}
	a.state.Users[target.ID] = target
	a.audit(u, "admin.user.update", target.ID)
	return publicUser(target), nil
}
func (a *App) listCodes(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if !a.isAdmin(u) {
		return nil, forbidden()
	}
	query := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("search")))
	status := r.URL.Query().Get("status")
	out := []Code{}
	for _, code := range a.state.Codes {
		if u.Role != "super_admin" && code.OrgID != u.OrgID {
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
		if u.Role == "super_admin" || u.OrgID == user.OrgID {
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
	return u.Role == "super_admin" || u.Role == "school_admin" && u.OrgID == c.OrgID
}
