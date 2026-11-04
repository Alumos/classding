package app

import (
	"encoding/json"
	"testing"
)

func TestTeacherWithoutSchoolJoinsTheSchoolOfTheirLicense(t *testing.T) {
	f := newSecurityFixture(t)
	solo := User{ID: "solo", Username: "solo", DisplayName: "丙老师", Role: RoleTeacher, OrgID: "org_private", Status: AccountActive}
	f.app.state.Users[solo.ID] = solo
	f.app.state.Classes["class_solo"] = Class{ID: "class_solo", Name: "丙班", OrgID: "org_private", OwnerID: "solo", ActivationCode: "CODE-SOLO", CreatedAt: now()}
	classID := "class_solo"
	f.app.state.Codes["CODE-SOLO"] = Code{Code: "CODE-SOLO", Status: LicenseActive, ClassID: &classID, OrgID: "org_private"}
	f.app.state.Codes["SCHOOL-A"] = Code{Code: "SCHOOL-A", Status: LicenseUnused, OrgID: "org_a", MaxTeachers: 1, MaxDevices: 1}

	expectStatus(t, f.request(t, "POST", "/api/v1/classes", f.app.access(solo), map[string]any{"name": "新班", "activation_code": "SCHOOL-A"}, nil), 200)
	if got := f.app.state.Users["solo"].OrgID; got != "org_a" {
		t.Fatalf("teacher org = %q, want org_a", got)
	}
	if f.app.state.Classes["class_solo"].OrgID != "org_a" || f.app.state.Codes["CODE-SOLO"].OrgID != "org_a" {
		t.Fatal("the teacher's own classes did not follow them into the school")
	}
}

func TestSchoolsAreManagedByTheOperator(t *testing.T) {
	f := newSecurityFixture(t)
	expectStatus(t, f.request(t, "POST", "/api/v1/admin/schools", f.token("school"), map[string]string{"name": "新学校"}, nil), 403)
	expectStatus(t, f.request(t, "POST", "/api/v1/admin/schools", f.token("super"), map[string]string{"name": "阳光小学"}, nil), 409)
	created := f.request(t, "POST", "/api/v1/admin/schools", f.token("super"), map[string]string{"name": "新学校"}, nil)
	expectStatus(t, created, 200)
	var school SchoolView
	if err := json.Unmarshal(created.Body.Bytes(), &school); err != nil || school.Name != "新学校" {
		t.Fatalf("bad school: %s", created.Body.String())
	}

	var listed []SchoolView
	list := f.request(t, "GET", "/api/v1/admin/schools", f.token("school"), nil, nil)
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil || len(listed) != 1 || listed[0].ID != "org_a" || listed[0].TeachersCount != 3 {
		t.Fatalf("school administrator sees %s", list.Body.String())
	}

	expectStatus(t, f.request(t, "PATCH", "/api/v1/admin/schools/"+school.ID, f.token("super"), map[string]string{"name": "改名学校"}, nil), 200)
	expectStatus(t, f.request(t, "DELETE", "/api/v1/admin/schools/org_a", f.token("super"), nil, nil), 409)
	expectStatus(t, f.request(t, "DELETE", "/api/v1/admin/schools/"+school.ID, f.token("super"), nil, nil), 200)

	expectStatus(t, f.request(t, "POST", "/api/v1/admin/activation-codes", f.token("super"), map[string]any{"count": 1, "max_teachers": 1, "max_devices": 1, "org_id": "typo"}, nil), 400)
	expectStatus(t, f.request(t, "PATCH", "/api/v1/admin/users/owner_b", f.token("super"), map[string]string{"org_id": "org_a"}, nil), 200)
	if f.app.state.Users["owner_b"].OrgID != "org_a" || f.app.state.Classes["class_b"].OrgID != "org_a" {
		t.Fatal("moving a teacher did not move the classes they own")
	}
}

func TestDeleteUnusedActivationCodesInBulk(t *testing.T) {
	f := newSecurityFixture(t)
	f.app.state.Codes["FREE-1"] = Code{Code: "FREE-1", Status: LicenseUnused}
	f.app.state.Codes["FREE-2"] = Code{Code: "FREE-2", Status: LicenseRevoked}
	path := "/api/v1/admin/activation-codes/delete"
	expectStatus(t, f.request(t, "POST", path, f.token("school"), map[string]any{"codes": []string{"FREE-1"}}, nil), 403)
	expectStatus(t, f.request(t, "POST", path, f.token("super"), map[string]any{"codes": []string{"FREE-1", "CODE-A"}}, nil), 409)
	if _, exists := f.app.state.Codes["FREE-1"]; !exists {
		t.Fatal("a rejected batch deleted codes")
	}
	expectStatus(t, f.request(t, "POST", path, f.token("super"), map[string]any{"codes": []string{"FREE-1", "FREE-2"}}, nil), 200)
	if len(f.app.state.Codes) != 2 {
		t.Fatalf("codes left: %v", f.app.state.Codes)
	}
}

func TestUpgradeGivesLegacySchoolIDsARecord(t *testing.T) {
	s := newState()
	s.Users["admin"] = User{ID: "admin", Role: RoleSchoolAdmin, OrgID: "实验小学"}
	s.Codes["X"] = Code{Code: "X", Status: LicenseUnused, OrgID: "第二中学"}
	upgradeState(&s)
	if s.Schools["实验小学"].Name != "实验小学" || s.Schools["第二中学"].Name != "第二中学" || len(s.Schools) != 2 {
		t.Fatalf("schools = %v", s.Schools)
	}
}
