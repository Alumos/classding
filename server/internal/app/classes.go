package app

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

func (a *App) listClasses(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	out := []Class{}
	for _, c := range a.state.Classes {
		if a.canClass(u, c) {
			out = append(out, a.classView(c, u))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out, nil
}
func (a *App) createClass(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if !a.limit("activate:"+u.ID, 10, time.Hour) {
		return nil, fail(429, "RATE_LIMITED", "激活尝试过于频繁，请一小时后重试。")
	}
	var in struct {
		Name, Grade string
		Code        string `json:"activation_code"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	in.Name = strings.TrimSpace(in.Name)
	if utf8.RuneCountInString(in.Name) < 1 || utf8.RuneCountInString(in.Name) > 40 {
		return nil, bad("班级名称需为 1–40 字。")
	}
	code, exists := a.state.Codes[strings.ToUpper(strings.TrimSpace(in.Code))]
	if !exists || code.Status != "unused" || code.ExpiresAt != nil && elapsed(*code.ExpiresAt) {
		return nil, fail(400, "INVALID_ACTIVATION_CODE", "激活码不存在、已使用或已过期。")
	}
	if code.OrgID != "" && code.OrgID != u.OrgID && u.Role != "super_admin" {
		return nil, fail(403, "LICENSE_ORG_MISMATCH", "该激活码属于其他学校，请联系管理员确认账号和激活码的所属学校。")
	}
	c := Class{ID: id("c"), Name: in.Name, Grade: in.Grade, OwnerID: u.ID, OrgID: u.OrgID, Volume: 75, MaxTeachers: code.MaxTeachers, MaxDevices: code.MaxDevices, CreatedAt: now(), ActivationCode: code.Code}
	code.Status = "active"
	code.OrgID = c.OrgID
	code.ClassID = &c.ID
	stamp := now()
	code.RedeemedAt = &stamp
	a.state.Codes[code.Code] = code
	a.state.Classes[c.ID] = c
	a.state.Members[c.ID+"|"+u.ID] = Member{u.ID, c.ID, u.DisplayName, u.Username, "owner", now()}
	a.audit(u, "class.create", c.ID)
	return a.classView(c, u), nil
}
func (a *App) updateClass(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	c, err := a.getClass(u, chi.URLParam(r, "id"), true)
	if err != nil {
		return nil, err
	}
	var in struct {
		Name   *string
		Volume *int
	}
	if err = decode(r, &in); err != nil {
		return nil, err
	}
	if in.Name != nil {
		value := strings.TrimSpace(*in.Name)
		if utf8.RuneCountInString(value) < 1 || utf8.RuneCountInString(value) > 40 {
			return nil, bad("班级名称需为 1–40 字。")
		}
		c.Name = value
	}
	if in.Volume != nil {
		if *in.Volume < 0 || *in.Volume > 100 {
			return nil, bad("音量需为 0–100。")
		}
		c.Volume = *in.Volume
	}
	a.state.Classes[c.ID] = c
	a.audit(u, "class.update", c.ID)
	a.deferJob(func() { a.hub.Class(c.ID, Frame{T: "config", D: map[string]any{"volume": c.Volume}}) })
	return a.classView(c, u), nil
}
func (a *App) deleteClass(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	c, err := a.getClass(u, chi.URLParam(r, "id"), true)
	if err != nil {
		return nil, err
	}
	a.removeClass(c, u)
	return ok(), nil
}

func (a *App) removeClass(c Class, u *User) {
	for key, item := range a.state.Homeworks {
		if item.ClassID == c.ID {
			delete(a.state.Homeworks, key)
		}
	}
	for key, m := range a.state.Members {
		if m.ClassID == c.ID {
			delete(a.state.Members, key)
		}
	}
	for key, d := range a.state.Devices {
		if d.ClassID == c.ID {
			a.unbind(d)
			delete(a.state.Devices, key)
		}
	}
	for key, inv := range a.state.Invites {
		if inv.ClassID == c.ID {
			delete(a.state.Invites, key)
		}
	}
	for key, b := range a.bindings {
		if b.ClassID == c.ID {
			delete(a.bindings, key)
		}
	}
	code := a.state.Codes[c.ActivationCode]
	if code.Status != "revoked" {
		code.Status = "released"
	}
	code.ClassID = nil
	a.state.Codes[code.Code] = code
	delete(a.state.Classes, c.ID)
	a.audit(u, "class.delete", c.ID)
}
func (a *App) members(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	c, err := a.getClass(u, chi.URLParam(r, "id"), false)
	if err != nil {
		return nil, err
	}
	out := []Member{}
	for _, m := range a.state.Members {
		if m.ClassID == c.ID {
			if user, exists := a.state.Users[m.UserID]; exists {
				m.DisplayName = user.DisplayName
				m.Username = user.Username
			}
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].JoinedAt < out[j].JoinedAt })
	return out, nil
}
func (a *App) invite(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	c, err := a.getClass(u, chi.URLParam(r, "id"), true)
	if err != nil {
		return nil, err
	}
	if !a.licensed(c) {
		return nil, fail(403, "LICENSE_INACTIVE", "班级授权已失效。")
	}
	if a.classView(c, u).TeachersCount >= c.MaxTeachers {
		return nil, fail(409, "TEACHER_LIMIT_REACHED", "教师席位已满。")
	}
	code := strings.ToUpper(random(9))
	inv := Invite{code, c.ID, after(7 * 24 * time.Hour), false}
	a.state.Invites[code] = inv
	a.audit(u, "class.invite", c.ID)
	return map[string]string{"code": code, "expires_at": inv.ExpiresAt}, nil
}
func (a *App) joinClass(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if !a.limit("join:"+u.ID, 10, time.Hour) {
		return nil, fail(429, "RATE_LIMITED", "尝试过于频繁。")
	}
	var in struct{ Code string }
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	key := strings.ToUpper(strings.TrimSpace(in.Code))
	inv, exists := a.state.Invites[key]
	if !exists || inv.Used || elapsed(inv.ExpiresAt) {
		return nil, bad("协同码不存在、已使用或已过期。")
	}
	c, exists := a.state.Classes[inv.ClassID]
	if !exists {
		return nil, missing()
	}
	if !a.licensed(c) {
		return nil, fail(403, "LICENSE_INACTIVE", "班级授权已失效。")
	}
	memberKey := c.ID + "|" + u.ID
	if _, exists = a.state.Members[memberKey]; exists {
		return nil, fail(409, "ALREADY_JOINED", "你已经在这个班级里了。")
	}
	if a.classView(c, u).TeachersCount >= c.MaxTeachers {
		return nil, fail(409, "TEACHER_LIMIT_REACHED", "这个班级的教师席位已满。")
	}
	a.state.Members[memberKey] = Member{u.ID, c.ID, u.DisplayName, u.Username, "collaborator", now()}
	inv.Used = true
	a.state.Invites[key] = inv
	a.audit(u, "class.join", c.ID)
	return a.classView(c, u), nil
}
func (a *App) removeMember(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	c, err := a.getClass(u, chi.URLParam(r, "id"), true)
	if err != nil {
		return nil, err
	}
	target := chi.URLParam(r, "uid")
	if target == c.OwnerID {
		return nil, bad("请先转让创建者身份。")
	}
	if _, exists := a.state.Members[c.ID+"|"+target]; !exists {
		return nil, missing()
	}
	delete(a.state.Members, c.ID+"|"+target)
	a.deferJob(func() { a.hub.RemoveTeacher(target) })
	a.audit(u, "class.member.remove", target)
	return ok(), nil
}
func (a *App) transfer(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	c, err := a.getClass(u, chi.URLParam(r, "id"), true)
	if err != nil {
		return nil, err
	}
	var in struct {
		UserID string `json:"user_id"`
	}
	if err = decode(r, &in); err != nil {
		return nil, err
	}
	next, exists := a.state.Members[c.ID+"|"+in.UserID]
	if !exists || next.Role == "owner" {
		return nil, bad("请选择班级内的协同老师。")
	}
	owner := a.state.Members[c.ID+"|"+c.OwnerID]
	owner.Role = "collaborator"
	a.state.Members[c.ID+"|"+c.OwnerID] = owner
	next.Role = "owner"
	a.state.Members[c.ID+"|"+in.UserID] = next
	c.OwnerID = in.UserID
	a.state.Classes[c.ID] = c
	a.audit(u, "class.transfer", c.ID)
	return a.classView(c, u), nil
}
func (a *App) leave(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	c, err := a.getClass(u, chi.URLParam(r, "id"), false)
	if err != nil {
		return nil, err
	}
	if c.OwnerID == u.ID {
		return nil, bad("请先转让创建者身份，再退出班级。")
	}
	delete(a.state.Members, c.ID+"|"+u.ID)
	a.deferJob(func() { a.hub.RemoveTeacher(u.ID) })
	a.audit(u, "class.leave", c.ID)
	return ok(), nil
}
func (a *App) devices(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	c, err := a.getClass(u, chi.URLParam(r, "id"), false)
	if err != nil {
		return nil, err
	}
	out := []Device{}
	for _, d := range a.state.Devices {
		if d.ClassID == c.ID {
			d.Online = a.hub.DeviceOnline(d.ID)
			out = append(out, publicDevice(d))
		}
	}
	return out, nil
}
func (a *App) bindCode(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	c, err := a.getClass(u, chi.URLParam(r, "id"), true)
	if err != nil {
		return nil, err
	}
	if !a.licensed(c) {
		return nil, fail(403, "LICENSE_INACTIVE", "班级授权已失效。")
	}
	if !a.limit("bind-code:"+u.ID, 10, time.Minute) {
		return nil, fail(429, "RATE_LIMITED", "绑定码获取过于频繁。")
	}
	if a.classView(c, u).DevicesCount >= c.MaxDevices {
		return nil, fail(409, "DEVICE_LIMIT_REACHED", "大屏名额已满，请先解绑旧设备。")
	}
	for key, b := range a.bindings {
		if b.ClassID == c.ID {
			delete(a.bindings, key)
		}
	}
	var code string
	for {
		code = sixDigits()
		if _, exists := a.bindings[code]; !exists {
			break
		}
	}
	expires := after(10 * time.Minute)
	a.bindings[code] = binding{c.ID, expires}
	return map[string]string{"code": code, "expires_at": expires}, nil
}
func (a *App) bindDevice(w http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	if !a.limit("bind:"+a.clientIP(r), 5, time.Minute) {
		return nil, fail(429, "RATE_LIMITED", "绑定尝试过于频繁，请一分钟后再试。")
	}
	var in struct {
		Code, Name, OS string
		ClientVersion  string `json:"client_version"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	b, exists := a.bindings[in.Code]
	if !exists || elapsed(b.Expires) {
		return nil, bad("绑定码不存在、已使用或已过期。")
	}
	c, exists := a.state.Classes[b.ClassID]
	if !exists || !a.licensed(c) {
		return nil, fail(403, "LICENSE_INACTIVE", "班级授权已失效。")
	}
	if a.classView(c, nil).DevicesCount >= c.MaxDevices {
		return nil, fail(409, "DEVICE_LIMIT_REACHED", "大屏名额已满。")
	}
	if utf8.RuneCountInString(in.Name) > 60 || len(in.OS) > 80 || len(in.ClientVersion) > 30 {
		return nil, bad("设备信息过长。")
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Name) == "教室大屏" {
		in.Name = c.Name + "智慧黑板"
	}
	token := random(32)
	d := Device{ID: id("d"), ClassID: c.ID, Name: in.Name, OS: in.OS, ClientVersion: in.ClientVersion, LastSeenAt: now(), TokenHash: hash(token)}
	a.state.Devices[d.ID] = d
	delete(a.bindings, in.Code)
	a.bindPending(d)
	a.audit(nil, "device.bind", d.ID)
	return map[string]any{"device_token": token, "device": publicDevice(d), "class": map[string]string{"id": c.ID, "name": c.Name}, "volume": c.Volume, "homeworks": a.homeworkItems(c.ID), "homework_revision": c.HomeworkRevision}, nil
}
func (a *App) deviceFromToken(token string) (Device, error) {
	if token == "" {
		return Device{}, fail(401, "UNAUTHORIZED", "设备尚未绑定。")
	}
	tokenHash := hash(token)
	for _, d := range a.state.Devices {
		if d.TokenHash == tokenHash {
			return d, nil
		}
	}
	return Device{}, fail(401, "DEVICE_REVOKED", "设备绑定已失效。")
}
func (a *App) deviceMe(w http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	d, err := a.deviceFromToken(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if err != nil {
		return nil, err
	}
	c, exists := a.state.Classes[d.ClassID]
	if !exists {
		return nil, fail(401, "DEVICE_REVOKED", "班级已被删除。")
	}
	return map[string]any{"device": publicDevice(d), "class": map[string]string{"id": c.ID, "name": c.Name}, "volume": c.Volume, "sequence": d.Sequence, "homeworks": a.homeworkItems(c.ID), "homework_revision": c.HomeworkRevision}, nil
}
func (a *App) renameDevice(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	d, exists := a.state.Devices[chi.URLParam(r, "id")]
	if !exists {
		return nil, missing()
	}
	if _, err := a.getClass(u, d.ClassID, true); err != nil {
		return nil, err
	}
	var in struct{ Name string }
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(strings.TrimSpace(in.Name)) < 1 || utf8.RuneCountInString(in.Name) > 60 {
		return nil, bad("设备名称需为 1–60 字。")
	}
	d.Name = strings.TrimSpace(in.Name)
	a.state.Devices[d.ID] = d
	return publicDevice(d), nil
}
func (a *App) unbind(d Device) {
	a.deferJob(func() {
		a.hub.Device(d.ID, Frame{T: "unbind", D: map[string]any{}})
		a.hub.RemoveDevice(d.ID)
		a.mu.Lock()
		defer a.mu.Unlock()
		a.presence(d, false)
	})
}
func (a *App) removeDevice(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	d, exists := a.state.Devices[chi.URLParam(r, "id")]
	if !exists {
		return nil, missing()
	}
	if _, err := a.getClass(u, d.ClassID, true); err != nil {
		return nil, err
	}
	a.unbind(d)
	delete(a.state.Devices, d.ID)
	for key, m := range a.state.Messages {
		for i := range m.Targets {
			if m.Targets[i].DeviceID == d.ID && m.Targets[i].Status == "queued" {
				m.Targets[i].Status = "failed"
			}
		}
		a.state.Messages[key] = m
	}
	a.audit(u, "device.unbind", d.ID)
	return ok(), nil
}
