package app

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAdminTeachersAndClasses(t *testing.T) {
	f := newSecurityFixture(t)
	createUser := map[string]any{"username": " New.Teacher ", "password": "Password12345", "display_name": "新老师"}
	expectStatus(t, f.request(t, "POST", "/api/v1/admin/users", f.token("owner"), createUser, nil), 403)
	expectStatus(t, f.request(t, "POST", "/api/v1/admin/users", f.token("school"), map[string]any{"username": "foreign", "password": "Password12345", "display_name": "外校", "org_id": "org_b"}, nil), 403)
	created := f.request(t, "POST", "/api/v1/admin/users", f.token("school"), createUser, nil)
	expectStatus(t, created, 200)
	var teacher User
	if err := json.Unmarshal(created.Body.Bytes(), &teacher); err != nil {
		t.Fatal(err)
	}
	if teacher.Username != "new.teacher" || teacher.OrgID != "org_a" || teacher.Role != "user" || teacher.PasswordHash != "" {
		t.Fatalf("incorrect teacher: %+v", teacher)
	}
	expectStatus(t, f.request(t, "POST", "/api/v1/admin/users", f.token("super"), createUser, nil), 409)
	expectStatus(t, f.request(t, "PATCH", "/api/v1/admin/users/owner_b", f.token("school"), map[string]any{"status": "disabled"}, nil), 403)
	expectStatus(t, f.request(t, "DELETE", "/api/v1/admin/users/school_a", f.token("super"), nil, nil), 403)
	expectStatus(t, f.request(t, "DELETE", "/api/v1/admin/users/owner_a", f.token("school"), nil, nil), 409)

	createClass := map[string]any{"name": "新班", "grade": "三年级", "owner_id": teacher.ID, "max_teachers": 2, "max_devices": 1}
	expectStatus(t, f.request(t, "POST", "/api/v1/admin/classes", f.token("owner"), createClass, nil), 403)
	expectStatus(t, f.request(t, "POST", "/api/v1/admin/classes", f.token("school"), map[string]any{"name": "外校", "owner_id": "owner_b", "max_teachers": 2, "max_devices": 1}, nil), 403)
	created = f.request(t, "POST", "/api/v1/admin/classes", f.token("school"), createClass, nil)
	expectStatus(t, created, 200)
	var class Class
	if err := json.Unmarshal(created.Body.Bytes(), &class); err != nil {
		t.Fatal(err)
	}
	persisted := f.app.state.Classes[class.ID]
	if class.OrgID != "org_a" || class.OwnerID != teacher.ID || !f.app.licensed(persisted) {
		t.Fatalf("class not licensed: %+v", persisted)
	}
	if member := f.app.state.Members[class.ID+"|"+teacher.ID]; member.Role != "owner" {
		t.Fatalf("owner membership missing: %+v", member)
	}
	code := f.app.state.Codes[persisted.ActivationCode]
	if code.OrgID != "org_a" {
		t.Fatalf("wrong license org: %+v", code)
	}
	expectStatus(t, f.request(t, "PATCH", "/api/v1/admin/classes/class_b", f.token("school"), map[string]any{"name": "越权"}, nil), 403)
	expectStatus(t, f.request(t, "PATCH", "/api/v1/admin/classes/"+class.ID, f.token("school"), map[string]any{"owner_id": "owner_b"}, nil), 403)
	expectStatus(t, f.request(t, "PATCH", "/api/v1/admin/classes/"+class.ID, f.token("school"), map[string]any{"name": "调整后", "grade": "四年级", "owner_id": "collab_a", "max_teachers": 3, "max_devices": 2}, nil), 200)
	persisted = f.app.state.Classes[class.ID]
	code = f.app.state.Codes[persisted.ActivationCode]
	if persisted.Name != "调整后" || persisted.Grade != "四年级" || persisted.OwnerID != "collab_a" || code.MaxTeachers != 3 || code.MaxDevices != 2 {
		t.Fatalf("update not reflected in class and license: %+v %+v", persisted, code)
	}
	if f.app.state.Members[class.ID+"|"+teacher.ID].Role != "collaborator" || f.app.state.Members[class.ID+"|collab_a"].Role != "owner" {
		t.Fatal("transfer did not update membership roles")
	}
	expectStatus(t, f.request(t, "PATCH", "/api/v1/admin/classes/"+class.ID, f.token("school"), map[string]any{"max_teachers": 1}, nil), 400)
	expectStatus(t, f.request(t, "DELETE", "/api/v1/admin/users/collab_a", f.token("school"), nil, nil), 409)

	f.app.state.Sessions["teacher_session"] = Session{ID: "teacher_session", UserID: teacher.ID, ExpiresAt: after(time.Hour)}
	f.app.state.Phrases["teacher_phrase"] = Phrase{ID: "teacher_phrase", UserID: teacher.ID}
	expectStatus(t, f.request(t, "PATCH", "/api/v1/admin/users/"+teacher.ID, f.token("school"), map[string]any{"username": "renamed", "password": "NewPassword123", "display_name": "改名"}, nil), 200)
	if _, ok := f.app.state.Sessions["teacher_session"]; ok {
		t.Fatal("password reset left old session valid")
	}
	if !passwordMatches("NewPassword123", f.app.state.Users[teacher.ID].PasswordHash) {
		t.Fatal("password not changed")
	}
	f.app.state.Sessions["teacher_session"] = Session{ID: "teacher_session", UserID: teacher.ID, ExpiresAt: after(time.Hour)}
	expectStatus(t, f.request(t, "DELETE", "/api/v1/admin/users/"+teacher.ID, f.token("school"), nil, nil), 200)
	if _, ok := f.app.state.Users[teacher.ID]; ok {
		t.Fatal("teacher remains")
	}
	if _, ok := f.app.state.Members[class.ID+"|"+teacher.ID]; ok {
		t.Fatal("teacher membership remains")
	}
	if _, ok := f.app.state.Sessions["teacher_session"]; ok {
		t.Fatal("teacher session remains")
	}
	if _, ok := f.app.state.Phrases["teacher_phrase"]; ok {
		t.Fatal("teacher phrase remains")
	}

	f.app.state.Devices["managed_device"] = Device{ID: "managed_device", ClassID: class.ID}
	f.app.state.Invites["managed_invite"] = Invite{Code: "managed_invite", ClassID: class.ID}
	f.app.bindings["123456"] = binding{ClassID: class.ID, Expires: after(time.Minute)}
	expectStatus(t, f.request(t, "DELETE", "/api/v1/admin/classes/"+class.ID, f.token("school"), nil, nil), 200)
	if _, ok := f.app.state.Classes[class.ID]; ok {
		t.Fatal("class remains")
	}
	if _, ok := f.app.state.Members[class.ID+"|collab_a"]; ok {
		t.Fatal("class members remain")
	}
	if _, ok := f.app.state.Devices["managed_device"]; ok {
		t.Fatal("class device remains")
	}
	if _, ok := f.app.state.Invites["managed_invite"]; ok {
		t.Fatal("class invite remains")
	}
	if _, ok := f.app.bindings["123456"]; ok {
		t.Fatal("class binding remains")
	}
	if code = f.app.state.Codes[code.Code]; code.Status != "released" || code.ClassID != nil {
		t.Fatalf("license not released: %+v", code)
	}
	var state State
	if err := f.app.store.Load(context.Background(), &state); err != nil {
		t.Fatal(err)
	}
	if _, ok := state.Classes[class.ID]; ok {
		t.Fatal("deleted class persisted")
	}
	if _, ok := state.Users[teacher.ID]; ok {
		t.Fatal("deleted teacher persisted")
	}
}

func TestAdminClassDeleteRollback(t *testing.T) {
	f := newSecurityFixture(t)
	f.app.state.Invites["pending"] = Invite{Code: "pending", ClassID: "class_a"}
	f.app.bindings["123456"] = binding{ClassID: "class_a", Expires: after(time.Minute)}
	restore := closeStoreForRollback(t, f.app)
	response := f.request(t, "DELETE", "/api/v1/admin/classes/class_a", f.token("school"), nil, nil)
	expectStatus(t, response, http.StatusInternalServerError)
	if _, ok := f.app.state.Classes["class_a"]; !ok {
		t.Fatal("failed delete removed class")
	}
	if _, ok := f.app.state.Devices["device_a"]; !ok {
		t.Fatal("failed delete removed device")
	}
	if _, ok := f.app.state.Invites["pending"]; !ok {
		t.Fatal("failed delete removed invite")
	}
	if _, ok := f.app.bindings["123456"]; !ok {
		t.Fatal("failed delete removed binding")
	}
	if f.app.state.Codes["CODE-A"].Status != "active" {
		t.Fatal("failed delete released license")
	}
	restore()
	expectStatus(t, f.request(t, "DELETE", "/api/v1/admin/classes/class_a", f.token("school"), nil, nil), 200)
}

func TestActivationOrganizationAndStrictSchoolScope(t *testing.T) {
	f := newSecurityFixture(t)
	f.app.state.Codes["FREE"] = Code{Code: "FREE", Status: "unused", MaxTeachers: 1, MaxDevices: 1}
	response := f.request(t, "POST", "/api/v1/classes", f.token("owner"), map[string]any{"name": "新班", "activation_code": "FREE"}, nil)
	expectStatus(t, response, 200)
	if got := f.app.state.Codes["FREE"].OrgID; got != "org_a" {
		t.Fatalf("redeemed license org = %q", got)
	}
	f.app.state.Codes["OTHER"] = Code{Code: "OTHER", Status: "unused", OrgID: "org_b", MaxTeachers: 1, MaxDevices: 1}
	response = f.request(t, "POST", "/api/v1/classes", f.token("owner"), map[string]any{"name": "新班", "activation_code": "OTHER"}, nil)
	expectStatus(t, response, 403)
	if !strings.Contains(response.Body.String(), "「外校」") || !strings.Contains(response.Body.String(), "「阳光小学」") {
		t.Fatalf("unclear error: %s", response.Body.String())
	}
	// Joining another school's class lets a school administrator teach there,
	// but never manage that class from the admin backend.
	expectStatus(t, f.request(t, "GET", "/api/v1/classes/class_b/members", f.token("school"), nil, nil), 200)
	expectStatus(t, f.request(t, "PATCH", "/api/v1/admin/classes/class_b", f.token("school"), map[string]string{"name": "改名"}, nil), 403)
}
