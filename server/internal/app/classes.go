package app

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const defaultVolume = 75

// The teacher workspace opens a class to its members only, administrators
// included; administrators reach every class they govern through /admin.

// canClass reports whether the account is a member of the class.
func (a *App) canClass(u *User, c Class) bool {
	if u == nil {
		return false
	}
	_, member := a.state.Members[memberKey(c.ID, u.ID)]
	return member
}

// owns reports whether the account created, or took over, the class.
func (a *App) owns(u *User, c Class) bool { return u != nil && c.OwnerID == u.ID }

// getClass loads a class the account can use, or manage when owner is set.
func (a *App) getClass(u *User, classID string, owner bool) (Class, error) {
	c, exists := a.state.Classes[classID]
	if !exists {
		return c, missing()
	}
	if !a.canClass(u, c) || owner && !a.owns(u, c) {
		return c, forbidden()
	}
	return c, nil
}

// licensed reports whether the class still holds an active, unexpired license.
func (a *App) licensed(c Class) bool {
	code, exists := a.state.Codes[c.ActivationCode]
	return exists && code.ClassID != nil && *code.ClassID == c.ID && code.Status == LicenseActive && !code.Expired()
}

// usableClass is getClass for actions that also need a valid license.
func (a *App) usableClass(u *User, classID string, owner bool) (Class, error) {
	c, err := a.getClass(u, classID, owner)
	if err == nil && !a.licensed(c) {
		err = licenseInactive()
	}
	return c, err
}

func (a *App) teacherCount(classID string) int {
	n := 0
	for _, m := range a.state.Members {
		if m.ClassID == classID {
			n++
		}
	}
	return n
}

func (a *App) deviceCount(classID string) int {
	n := 0
	for _, d := range a.state.Devices {
		if d.ClassID == classID {
			n++
		}
	}
	return n
}

func (a *App) classView(c Class, u *User) ClassView {
	view := ClassView{Class: c, Role: MemberCollaborator, TeachersCount: a.teacherCount(c.ID)}
	if a.owns(u, c) {
		view.Role = MemberOwner
	}
	for _, d := range a.state.Devices {
		if d.ClassID == c.ID {
			view.DevicesCount++
			if a.hub.DeviceOnline(d.ID) {
				view.OnlineDevices++
			}
		}
	}
	view.ActivationCode, view.HomeworkRevision = "", 0
	return view
}

func (a *App) classViews(u *User, include func(Class) bool) []ClassView {
	out := []ClassView{}
	for _, c := range a.state.Classes {
		if include(c) {
			out = append(out, a.classView(c, u))
		}
	}
	slices.SortFunc(out, func(x, y ClassView) int { return strings.Compare(x.CreatedAt, y.CreatedAt) })
	return out
}

func (a *App) addMember(c Class, u User, role MemberRole) {
	a.state.Members[memberKey(c.ID, u.ID)] = Member{UserID: u.ID, ClassID: c.ID, DisplayName: u.DisplayName, Username: u.Username, Role: role, JoinedAt: now()}
}

func validClassName(name string) bool { return runesBetween(name, 1, maxClassName) }

func (a *App) listClasses(_ http.ResponseWriter, _ *http.Request, u *User) (any, error) {
	return a.classViews(u, a.memberOf(u)), nil
}

// createClass redeems an activation code for a new class owned by the teacher.
func (a *App) createClass(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if !a.limit("activate:"+u.ID, 10, time.Hour) {
		return nil, tooMany("激活尝试过于频繁，请一小时后重试。")
	}
	var in struct {
		Name  string `json:"name"`
		Grade string `json:"grade"`
		Code  string `json:"activation_code"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if !validClassName(name) {
		return nil, bad("班级名称需为 1–40 字。")
	}
	code, exists := a.state.Codes[strings.ToUpper(strings.TrimSpace(in.Code))]
	if !exists || code.Status != LicenseUnused || code.Expired() {
		return nil, fail(400, "INVALID_ACTIVATION_CODE", "激活码不存在、已使用或已过期。")
	}
	if err := a.redeemFor(u, code); err != nil {
		return nil, err
	}
	c := a.activateClass(code, Class{Name: name, Grade: in.Grade, OwnerID: u.ID, OrgID: u.OrgID}, *u)
	a.audit(u, "class.create", c.ID)
	return a.classView(c, u), nil
}

// activateClass stores a new class under the license, with its owner as the first member.
func (a *App) activateClass(code Code, c Class, owner User) Class {
	c.ID, c.CreatedAt, c.Volume, c.ActivationCode = id("c"), now(), defaultVolume, code.Code
	c.MaxTeachers, c.MaxDevices = code.MaxTeachers, code.MaxDevices
	redeemed := c.CreatedAt
	code.Status, code.OrgID, code.ClassID, code.RedeemedAt = LicenseActive, c.OrgID, &c.ID, &redeemed
	a.state.Codes[code.Code] = code
	a.state.Classes[c.ID] = c
	a.addMember(c, owner, MemberOwner)
	return c
}

func (a *App) updateClass(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	c, err := a.getClass(u, chi.URLParam(r, "id"), true)
	if err != nil {
		return nil, err
	}
	var in struct {
		Name   *string `json:"name"`
		Volume *int    `json:"volume"`
	}
	if err = decode(r, &in); err != nil {
		return nil, err
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if !validClassName(name) {
			return nil, bad("班级名称需为 1–40 字。")
		}
		c.Name = name
	}
	if in.Volume != nil {
		if !inRange(*in.Volume, 0, 100) {
			return nil, bad("音量需为 0–100。")
		}
		c.Volume = *in.Volume
	}
	a.state.Classes[c.ID] = c
	a.audit(u, "class.update", c.ID)
	a.deferJob(func() { a.hub.Class(c.ID, Frame{T: "config", D: map[string]any{"volume": c.Volume}}) })
	return a.classView(c, u), nil
}

func (a *App) deleteClass(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	c, err := a.getClass(u, chi.URLParam(r, "id"), true)
	if err != nil {
		return nil, err
	}
	a.removeClass(c, u)
	return ok(), nil
}

// removeClass deletes the class with everything that belongs to it and
// releases its license for reuse.
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
	a.dropBindings(c.ID)
	code := a.state.Codes[c.ActivationCode]
	if code.Status != LicenseRevoked {
		code.Status = LicenseReleased
	}
	code.ClassID = nil
	a.state.Codes[code.Code] = code
	delete(a.state.Classes, c.ID)
	a.audit(u, "class.delete", c.ID)
}

func (a *App) members(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	c, err := a.getClass(u, chi.URLParam(r, "id"), false)
	if err != nil {
		return nil, err
	}
	out := []Member{}
	for _, m := range a.state.Members {
		if m.ClassID != c.ID {
			continue
		}
		if user, exists := a.state.Users[m.UserID]; exists {
			m.DisplayName, m.Username = user.DisplayName, user.Username
		}
		out = append(out, m)
	}
	slices.SortFunc(out, func(x, y Member) int { return strings.Compare(x.JoinedAt, y.JoinedAt) })
	return out, nil
}

// invite issues a one-time collaboration code valid for seven days.
func (a *App) invite(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	c, err := a.usableClass(u, chi.URLParam(r, "id"), true)
	if err != nil {
		return nil, err
	}
	if a.teacherCount(c.ID) >= c.MaxTeachers {
		return nil, fail(409, "TEACHER_LIMIT_REACHED", "教师席位已满。")
	}
	inv := Invite{Code: strings.ToUpper(random(9)), ClassID: c.ID, ExpiresAt: after(7 * 24 * time.Hour)}
	a.state.Invites[inv.Code] = inv
	a.audit(u, "class.invite", c.ID)
	return map[string]string{"code": inv.Code, "expires_at": inv.ExpiresAt}, nil
}

func (a *App) joinClass(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if !a.limit("join:"+u.ID, 10, time.Hour) {
		return nil, tooMany("尝试过于频繁。")
	}
	var in struct {
		Code string `json:"code"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	inv, exists := a.state.Invites[strings.ToUpper(strings.TrimSpace(in.Code))]
	if !exists || inv.Used || elapsed(inv.ExpiresAt) {
		return nil, bad("协同码不存在、已使用或已过期。")
	}
	c, exists := a.state.Classes[inv.ClassID]
	if !exists {
		return nil, missing()
	}
	if !a.licensed(c) {
		return nil, licenseInactive()
	}
	if _, joined := a.state.Members[memberKey(c.ID, u.ID)]; joined {
		return nil, fail(409, "ALREADY_JOINED", "你已经在这个班级里了。")
	}
	if a.teacherCount(c.ID) >= c.MaxTeachers {
		return nil, fail(409, "TEACHER_LIMIT_REACHED", "这个班级的教师席位已满。")
	}
	a.addMember(c, *u, MemberCollaborator)
	inv.Used = true
	a.state.Invites[inv.Code] = inv
	a.audit(u, "class.join", c.ID)
	return a.classView(c, u), nil
}

func (a *App) removeMember(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	c, err := a.getClass(u, chi.URLParam(r, "id"), true)
	if err != nil {
		return nil, err
	}
	target := chi.URLParam(r, "uid")
	if target == c.OwnerID {
		return nil, bad("请先转让创建者身份。")
	}
	key := memberKey(c.ID, target)
	if _, exists := a.state.Members[key]; !exists {
		return nil, missing()
	}
	delete(a.state.Members, key)
	a.deferJob(func() { a.hub.RemoveTeacher(target) })
	a.audit(u, "class.member.remove", target)
	return ok(), nil
}

func (a *App) transfer(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
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
	if next, exists := a.state.Members[memberKey(c.ID, in.UserID)]; !exists || next.Role == MemberOwner {
		return nil, bad("请选择班级内的协同老师。")
	}
	c = a.changeOwner(c, in.UserID)
	a.audit(u, "class.transfer", c.ID)
	return a.classView(c, u), nil
}

// changeOwner hands the class to an existing member; the previous owner stays
// on as a collaborator.
func (a *App) changeOwner(c Class, userID string) Class {
	previousKey, nextKey := memberKey(c.ID, c.OwnerID), memberKey(c.ID, userID)
	if previous, exists := a.state.Members[previousKey]; exists {
		previous.Role = MemberCollaborator
		a.state.Members[previousKey] = previous
	}
	next := a.state.Members[nextKey]
	next.Role = MemberOwner
	a.state.Members[nextKey] = next
	c.OwnerID = userID
	a.state.Classes[c.ID] = c
	return c
}

func (a *App) leave(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	c, err := a.getClass(u, chi.URLParam(r, "id"), false)
	if err != nil {
		return nil, err
	}
	if c.OwnerID == u.ID {
		return nil, bad("请先转让创建者身份，再退出班级。")
	}
	delete(a.state.Members, memberKey(c.ID, u.ID))
	a.deferJob(func() { a.hub.RemoveTeacher(u.ID) })
	a.audit(u, "class.leave", c.ID)
	return ok(), nil
}
