package app

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func migrationPayload(sequence int64, ids ...string) map[string]any {
	if ids == nil {
		ids = []string{}
	}
	return map[string]any{"min_sequence": sequence, "pending_ids": ids}
}

func TestMigrationCheckResignsAudioAndRejectsStaleBackup(t *testing.T) {
	src := prepareBackupFixture(t)
	device := src.app.state.Devices["device_a"]
	device.Sequence = 3
	src.app.state.Devices[device.ID] = device
	stamp := now()
	target := func(sequence int64) []Target {
		return []Target{{ClassID: "class_a", ClassName: "甲班", DeviceID: "device_a", DeviceName: "甲设备", Status: "queued", Seq: sequence}}
	}
	src.app.state.Messages["voice_queued"] = Message{ID: "voice_queued", Kind: "voice", Text: "录音通知", Style: "urgent", SenderID: "owner_a", AudioID: "audio_sample", DisplaySec: 60, Repeat: 1, PlayMode: "voice", CreatedAt: now(), ExpiresAt: after(time.Hour), Targets: target(3)}
	src.app.state.Messages["recall_queued"] = Message{ID: "recall_queued", Kind: "text", SenderID: "owner_a", CreatedAt: now(), ExpiresAt: after(time.Hour), RecalledAt: &stamp, Targets: target(1)}
	src.app.state.Messages["expired_queued"] = Message{ID: "expired_queued", Kind: "text", SenderID: "owner_a", CreatedAt: now(), ExpiresAt: after(-time.Minute), Targets: target(2)}
	if err := src.app.Save(); err != nil {
		t.Fatal(err)
	}
	oldURL := src.app.audioURL("audio_sample")
	expectStatus(t, src.request(t, "GET", oldURL, "", nil, nil), 200)
	archive := getBackup(t, src)

	// The original site accepts another notification after the backup.
	device.Sequence = 4
	src.app.state.Devices[device.ID] = device
	src.app.state.Messages["late_queued"] = Message{ID: "late_queued", Kind: "text", SenderID: "owner_a", CreatedAt: now(), ExpiresAt: after(time.Hour), Targets: target(4)}
	me := src.request(t, "GET", "/api/v1/device/me", "token-a", nil, nil)
	var sourceMe struct {
		Sequence int64 `json:"sequence"`
	}
	if err := json.Unmarshal(me.Body.Bytes(), &sourceMe); err != nil || sourceMe.Sequence != 4 {
		t.Fatalf("source sequence missing: %s (%v)", me.Body.String(), err)
	}

	dst := newSecurityFixture(t)
	dst.app.config.JWTSecret = strings.Repeat("different-destination-secret-", 2)
	expectStatus(t, restoreBackup(t, dst, dst.token("super"), archive), 200)
	path := "/api/v1/device/migration-check"
	for _, tc := range []struct {
		name    string
		payload map[string]any
	}{
		{"source checkpoint ahead", migrationPayload(4)},
		{"missing pending message", migrationPayload(3, "late_queued")},
		{"message for another device", migrationPayload(3, "message_b")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := dst.request(t, "POST", path, "token-a", tc.payload, nil)
			expectStatus(t, w, http.StatusConflict)
			if !strings.Contains(w.Body.String(), "MIGRATION_NOT_READY") {
				t.Fatalf("incorrect error: %s", w.Body.String())
			}
		})
	}
	w := dst.request(t, "POST", path, "token-a", migrationPayload(3, "voice_queued", "recall_queued", "expired_queued"), nil)
	expectStatus(t, w, 200)
	var result struct {
		Device Device `json:"device"`
		Class  struct {
			ID string `json:"id"`
		} `json:"class"`
		Sequence      int64    `json:"sequence"`
		Notifications []Frame  `json:"notifications"`
		RecalledIDs   []string `json:"recalled_ids"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Sequence != 3 || result.Device.ID != "device_a" || result.Device.ClassID != "class_a" || result.Device.TokenHash != "" || result.Class.ID != "class_a" || len(result.Notifications) != 1 || len(result.RecalledIDs) != 2 || result.RecalledIDs[0] != "recall_queued" || result.RecalledIDs[1] != "expired_queued" {
		t.Fatalf("unexpected migration result: %s", w.Body.String())
	}
	frame := result.Notifications[0]
	data, ok := frame.D.(map[string]any)
	if !ok || frame.T != "notify" || frame.ID != "voice_queued" || frame.Seq != 3 {
		t.Fatalf("invalid notify frame: %+v", frame)
	}
	newURL, ok := data["audio_url"].(string)
	if !ok || newURL == "" || newURL == oldURL {
		t.Fatalf("audio was not re-signed: %v", data["audio_url"])
	}
	expectStatus(t, dst.request(t, "GET", oldURL, "", nil, nil), 403)
	audio := dst.request(t, "GET", newURL, "", nil, nil)
	expectStatus(t, audio, 200)
	if audio.Body.String() != "test recording" {
		t.Fatalf("new signed URL returned %q", audio.Body.String())
	}
	if err := os.Remove(dst.app.state.Audio["audio_sample"].Path); err != nil {
		t.Fatal(err)
	}
	expectStatus(t, dst.request(t, "POST", path, "token-a", migrationPayload(3, "voice_queued"), nil), 409)
}

func TestMigrationCheckValidation(t *testing.T) {
	f := newSecurityFixture(t)
	path := "/api/v1/device/migration-check"
	expectStatus(t, f.request(t, "POST", path, "wrong-token", migrationPayload(0), nil), 401)
	for _, body := range []any{
		map[string]any{"pending_ids": []string{}},
		map[string]any{"min_sequence": -1, "pending_ids": []string{}},
		map[string]any{"min_sequence": 0},
		map[string]any{"min_sequence": 0, "pending_ids": nil},
		migrationPayload(0, ""),
		map[string]any{"min_sequence": 0, "pending_ids": make([]string, 4097)},
	} {
		expectStatus(t, f.request(t, "POST", path, "token-a", body, nil), 400)
	}
	w := f.request(t, "POST", path, "token-a", migrationPayload(0), nil)
	expectStatus(t, w, 200)
	if !strings.Contains(w.Body.String(), `"notifications":[]`) || !strings.Contains(w.Body.String(), `"recalled_ids":[]`) {
		t.Fatalf("empty arrays missing: %s", w.Body.String())
	}
}
