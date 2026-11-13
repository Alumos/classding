package app

import (
	"context"
	"encoding/json"
	"testing"
)

func mutateStudentBackup(t *testing.T, archive []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	return alterBackup(t, archive, func(files map[string][]byte) {
		var manifest map[string]any
		if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
			t.Fatal(err)
		}
		mutate(manifest["state"].(map[string]any))
		files["manifest.json"], _ = json.Marshal(manifest)
	})
}

func TestStudentCallsBackupCompatibilityRoundTripAndValidation(t *testing.T) {
	src := prepareBackupFixture(t)
	item := postStudentCall(t, src, "token-a", "backup-key")
	expectStatus(t, src.request(t, "POST", studentCallsPath+"/"+item.ID+"/read", src.token("collab"), nil, nil), 200)
	archive := getBackup(t, src)
	dst := newSecurityFixture(t)
	expectStatus(t, restoreBackup(t, dst, dst.token("super"), archive), 200)
	if got := dst.app.state.StudentCalls[item.ID]; got.RequestID != "backup-key" || got.ReadBy != "collab_a" {
		t.Fatalf("restore lost call: %+v", got)
	}
	var persisted State
	if err := dst.app.store.Load(context.Background(), &persisted); err != nil || persisted.StudentCalls[item.ID] != dst.app.state.StudentCalls[item.ID] {
		t.Fatal("restore not persisted", err)
	}
	if replay := postStudentCall(t, dst, "token-a", "backup-key"); replay.ID != item.ID || replay.ReadBy != "collab_a" {
		t.Fatal("restore lost replay or read state")
	}
	expectStatus(t, dst.request(t, "POST", deviceCallsPath, "token-a", map[string]string{"request_id": "too-soon"}, nil), 429)
	legacy := mutateStudentBackup(t, archive, func(s map[string]any) { delete(s, "student_calls") })
	old := newSecurityFixture(t)
	expectStatus(t, restoreBackup(t, old, old.token("super"), legacy), 200)
	if old.app.state.StudentCalls == nil || len(old.app.state.StudentCalls) != 0 {
		t.Fatal("legacy backup not upgraded")
	}
	postStudentCall(t, old, "token-a", "new-after-legacy")

	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"null collection", func(s map[string]any) { s["student_calls"] = nil }},
		{"missing class", func(i map[string]any) { i["class_id"] = "missing" }},
		{"key mismatch", func(i map[string]any) { i["id"] = "mismatch" }},
		{"empty device", func(i map[string]any) { i["device_id"] = "" }},
		{"empty device name", func(i map[string]any) { i["device_name"] = " " }},
		{"empty class name", func(i map[string]any) { i["class_name"] = " " }},
		{"invalid created time", func(i map[string]any) { i["created_at"] = "invalid" }},
		{"read before creation", func(i map[string]any) { i["read_at"] = "2000-01-01T00:00:00Z" }},
		{"invalid read time", func(i map[string]any) { i["read_at"] = "invalid" }},
		{"no processor", func(i map[string]any) { i["read_by"] = "" }},
		{"no processor name", func(i map[string]any) { i["read_by_name"] = " " }},
		{"partial unread", func(i map[string]any) { i["read_at"] = "" }},
		{"bad request key", func(i map[string]any) { i["request_id"] = " whitespace " }},
		{"missing read field", func(i map[string]any) { delete(i, "read_at") }},
		{"missing request key", func(i map[string]any) { delete(i, "request_id") }},
		{"null read field", func(i map[string]any) { i["read_by_name"] = nil }},
		{"duplicate request key", func(s map[string]any) {
			calls := s["student_calls"].(map[string]any)
			copy := map[string]any{}
			for k, v := range calls[item.ID].(map[string]any) {
				copy[k] = v
			}
			copy["id"] = "duplicate"
			calls["duplicate"] = copy
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			corrupt := mutateStudentBackup(t, archive, func(s map[string]any) {
				if tc.name == "null collection" || tc.name == "duplicate request key" {
					tc.mutate(s)
				} else {
					tc.mutate(s["student_calls"].(map[string]any)[item.ID].(map[string]any))
				}
			})
			f := newSecurityFixture(t)
			expectStatus(t, restoreBackup(t, f, f.token("super"), corrupt), 400)
			if len(f.app.state.StudentCalls) != 0 {
				t.Fatal("invalid backup mutated live calls")
			}
		})
	}
}

func TestStudentCallsHistoricalDevicesAndProcessorsSurviveBackup(t *testing.T) {
	src := prepareBackupFixture(t)
	item := postStudentCall(t, src, "token-a", "history")
	expectStatus(t, src.request(t, "POST", studentCallsPath+"/"+item.ID+"/read", src.token("collab"), nil, nil), 200)
	// Real administrative deletion must preserve the first processor snapshot.
	expectStatus(t, src.request(t, "DELETE", "/api/v1/admin/users/collab_a", src.token("school"), nil, nil), 200)
	expectStatus(t, src.request(t, "DELETE", "/api/v1/devices/device_a", src.token("owner"), nil, nil), 200)
	archive := getBackup(t, src)
	for _, rebound := range []bool{false, true} {
		data := archive
		if rebound {
			// A restored historical ID may now describe a replacement binding in a
			// different class. It must not invalidate the original call snapshot.
			data = mutateStudentBackup(t, archive, func(s map[string]any) {
				s["devices"].(map[string]any)["device_a"] = map[string]any{"id": "device_a", "class_id": "class_b", "name": "replacement", "token_hash": hash("replacement-token")}
			})
			// Legacy notification validation also checks existing device/class pairs;
			// remove those unrelated historical messages for this focused scenario.
			data = mutateStudentBackup(t, data, func(s map[string]any) { s["messages"] = map[string]any{} })
		}
		dst := newSecurityFixture(t)
		expectStatus(t, restoreBackup(t, dst, dst.token("super"), data), 200)
		got := dst.app.state.StudentCalls[item.ID]
		if got.ReadBy != "collab_a" || got.ReadByName != "乙老师" || got.DeviceName != "甲设备" || got.ClassID != "class_a" {
			t.Fatalf("history changed: %+v", got)
		}
	}
}

func TestStudentCallsClassDeletionRollsBack(t *testing.T) {
	f := newSecurityFixture(t)
	item := postStudentCall(t, f, "token-a", "delete-rollback")
	restore := closeStoreForRollback(t, f.app)
	expectStatus(t, f.request(t, "DELETE", "/api/v1/classes/class_a", f.token("owner"), nil, nil), 500)
	if f.app.state.StudentCalls[item.ID].ID != item.ID {
		t.Fatal("failed class deletion lost calls")
	}
	restore()
	expectStatus(t, f.request(t, "DELETE", "/api/v1/classes/class_a", f.token("owner"), nil, nil), 200)
	if len(f.app.state.StudentCalls) != 0 {
		t.Fatal("class deletion did not remove calls")
	}
}
