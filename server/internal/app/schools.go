package app

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
)

// A school is an organization the operator manages by name. Accounts, classes
// and licenses refer to it through their org_id. A teacher who registers on
// their own gets a private organization instead, which is not a school; such a
// teacher joins a school by redeeming one of its licenses or by being assigned.

const maxSchoolName = 40

// SchoolView is a school with how much of the service it uses.
type SchoolView struct {
	School
	TeachersCount int `json:"teachers_count"`
	ClassesCount  int `json:"classes_count"`
	CodesCount    int `json:"codes_count"`
}

func (a *App) isSchool(orgID string) bool {
	_, exists := a.state.Schools[orgID]
	return exists
}

// schoolName names the organization in messages; a private one has no name.
func (a *App) schoolName(orgID string) string {
	if school, exists := a.state.Schools[orgID]; exists {
		return "「" + school.Name + "」"
	}
	return "未加入学校的个人账号"
}

// ensureSchool records an organization that older releases only knew by ID.
func (s *State) ensureSchool(orgID string) {
	if _, exists := s.Schools[orgID]; orgID != "" && !exists {
		s.Schools[orgID] = School{ID: orgID, Name: orgID, CreatedAt: now()}
	}
}

// moveTeacher puts the teacher into orgID. Classes the teacher owns in the old
// organization move along with their licenses, so a class always sits in its
// owner's organization.
func (a *App) moveTeacher(teacher *User, orgID string) {
	previous := teacher.OrgID
	teacher.OrgID = orgID
	a.state.Users[teacher.ID] = *teacher
	for key, class := range a.state.Classes {
		if class.OwnerID != teacher.ID || class.OrgID != previous {
			continue
		}
		class.OrgID = orgID
		a.state.Classes[key] = class
		if code, exists := a.state.Codes[class.ActivationCode]; exists {
			code.OrgID = orgID
			a.state.Codes[code.Code] = code
		}
	}
}

// redeemFor checks that the teacher may redeem a license of orgID, and lets a
// teacher without a school join the license's school.
func (a *App) redeemFor(teacher *User, code Code) error {
	if code.OrgID == "" || code.OrgID == teacher.OrgID || teacher.IsSuperAdmin() {
		return nil
	}
	if a.isSchool(teacher.OrgID) || !a.isSchool(code.OrgID) {
		message := fmt.Sprintf("该激活码属于%s，你的账号属于%s，请联系管理员确认。", a.schoolName(code.OrgID), a.schoolName(teacher.OrgID))
		return fail(403, "LICENSE_ORG_MISMATCH", message)
	}
	a.moveTeacher(teacher, code.OrgID)
	return nil
}

func (a *App) schoolView(s School) SchoolView {
	view := SchoolView{School: s}
	for _, u := range a.state.Users {
		if u.OrgID == s.ID && u.Role == RoleTeacher {
			view.TeachersCount++
		}
	}
	for _, c := range a.state.Classes {
		if c.OrgID == s.ID {
			view.ClassesCount++
		}
	}
	for _, code := range a.state.Codes {
		if code.OrgID == s.ID {
			view.CodesCount++
		}
	}
	return view
}

func (a *App) listSchools(_ http.ResponseWriter, _ *http.Request, u *User) (any, error) {
	out := []SchoolView{}
	for _, school := range a.state.Schools {
		if u.Governs(school.ID) {
			out = append(out, a.schoolView(school))
		}
	}
	slices.SortFunc(out, func(x, y SchoolView) int { return strings.Compare(x.CreatedAt, y.CreatedAt) })
	return out, nil
}

// decodeSchoolName reads a name that no other school uses.
func (a *App) decodeSchoolName(r *http.Request, schoolID string) (string, error) {
	var in struct {
		Name string `json:"name"`
	}
	if err := decodeStrict(r, &in); err != nil {
		return "", err
	}
	name := strings.TrimSpace(in.Name)
	if !runesBetween(name, 1, maxSchoolName) {
		return "", bad("学校名称需为 1–40 字。")
	}
	for _, school := range a.state.Schools {
		if school.ID != schoolID && school.Name == name {
			return "", fail(409, "SCHOOL_NAME_TAKEN", "已有同名学校。")
		}
	}
	return name, nil
}

func (a *App) createSchool(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	name, err := a.decodeSchoolName(r, "")
	if err != nil {
		return nil, err
	}
	school := School{ID: id("org"), Name: name, CreatedAt: now()}
	a.state.Schools[school.ID] = school
	a.audit(u, "admin.school.create", school.ID)
	return a.schoolView(school), nil
}

func (a *App) updateSchool(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	school, exists := a.state.Schools[chi.URLParam(r, "id")]
	if !exists {
		return nil, missing()
	}
	name, err := a.decodeSchoolName(r, school.ID)
	if err != nil {
		return nil, err
	}
	school.Name = name
	a.state.Schools[school.ID] = school
	a.audit(u, "admin.school.update", school.ID)
	return a.schoolView(school), nil
}

// deleteSchool removes a school that nothing refers to any more.
func (a *App) deleteSchool(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	school, exists := a.state.Schools[chi.URLParam(r, "id")]
	if !exists {
		return nil, missing()
	}
	if view := a.schoolView(school); view.TeachersCount+view.ClassesCount+view.CodesCount > 0 {
		return nil, fail(409, "SCHOOL_IN_USE", "该学校仍有教师、班级或激活码，请先移出后再删除。")
	}
	for _, user := range a.state.Users {
		if user.OrgID == school.ID {
			return nil, fail(409, "SCHOOL_IN_USE", "该学校仍有管理员账号，请先移出后再删除。")
		}
	}
	delete(a.state.Schools, school.ID)
	a.audit(u, "admin.school.delete", school.ID)
	return ok(), nil
}
