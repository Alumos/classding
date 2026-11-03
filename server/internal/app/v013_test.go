package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSettingsValidationPermissionsAndPersistence(t *testing.T) {
	f := newSecurityFixture(t)
	path := "/api/v1/admin/settings"
	body := map[string]any{"site_name": "阳光小学", "icp_number": "京ICP备12345678号-1", "theme": "blue"}
	expectStatus(t, f.request(t, "GET", path, f.token("school"), nil, nil), 403)
	expectStatus(t, f.request(t, "PUT", path, f.token("school"), body, nil), 403)
	for _, invalid := range []map[string]any{
		{"site_name": "", "icp_number": "", "theme": "green"},
		{"site_name": "学校", "icp_number": "https://evil.example", "theme": "green"},
		{"site_name": "学校", "icp_number": "", "theme": "purple"},
		{"site_name": "学校", "icp_number": ""},
		{"site_name": "学校", "icp_number": "", "theme": "green", "unexpected": true},
	} {
		expectStatus(t, f.request(t, "PUT", path, f.token("super"), invalid, nil), 400)
	}
	expectStatus(t, f.request(t, "PUT", path, f.token("super"), body, nil), 200)
	config := f.request(t, "GET", "/api/v1/config", "", nil, nil)
	for _, expected := range []string{"阳光小学", "京ICP备12345678号-1", "blue"} {
		if !strings.Contains(config.Body.String(), expected) {
			t.Fatalf("config missing %q: %s", expected, config.Body.String())
		}
	}
	var persisted State
	if err := f.app.store.Load(context.Background(), &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Settings["site_name"] != "阳光小学" || persisted.Settings["theme"] != "blue" {
		t.Fatalf("settings not persisted: %+v", persisted.Settings)
	}
}

func TestDeviceRemoteConfigurationAndBindingNames(t *testing.T) {
	f := newSecurityFixture(t)
	peerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	peer := &Peer{ctx: peerCtx, queue: make(chan Frame, 8)}
	f.app.hub.devices["device_a"] = peer
	defer delete(f.app.hub.devices, "device_a")
	path := "/api/v1/admin/devices/device_a/config"
	expectStatus(t, f.request(t, "POST", path, f.token("school"), map[string]string{"server_url": "https://new.example"}, nil), 403)
	for _, invalid := range []string{"https://user:pass@example.com", "ftp://example.com", "https://example.com/api/v1", "https://example.com?q=1", "https://example.com/#part", "https://example.com:", "https://example.com:65536", " https://example.com"} {
		expectStatus(t, f.request(t, "POST", path, f.token("super"), map[string]string{"server_url": invalid}, nil), 400)
	}
	w := f.request(t, "POST", path, f.token("super"), map[string]string{"server_url": "https://new.example/"}, nil)
	expectStatus(t, w, 200)
	var device Device
	if err := json.Unmarshal(w.Body.Bytes(), &device); err != nil || device.RemoteConfig == nil || device.RemoteConfig.ServerURL != "https://new.example" || device.RemoteConfig.Revision == "" {
		t.Fatalf("invalid device config: %s (%v)", w.Body.String(), err)
	}
	frame := <-peer.queue
	if frame.T != "config" || frame.D.(map[string]any)["remote_config"].(*RemoteConfig).Revision != device.RemoteConfig.Revision {
		t.Fatalf("bad config frame: %+v", frame)
	}
	me := f.request(t, "GET", "/api/v1/device/me", "token-a", nil, nil)
	if !strings.Contains(me.Body.String(), device.RemoteConfig.Revision) {
		t.Fatalf("device/me omitted config: %s", me.Body.String())
	}
	var persisted State
	if err := f.app.store.Load(context.Background(), &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Devices["device_a"].RemoteConfig.Revision != device.RemoteConfig.Revision {
		t.Fatal("device config was not persisted")
	}
	expectStatus(t, f.request(t, "POST", path, f.token("super"), map[string]string{"server_url": ""}, nil), 200)
	frame = <-peer.queue
	payload, _ := json.Marshal(frame.D)
	if string(payload) != `{"remote_config":null}` {
		t.Fatalf("cancellation frame = %s", payload)
	}
	for i, tc := range []struct{ provided, expected string }{{"", "甲班智慧黑板"}, {"教室大屏", "甲班智慧黑板"}, {"自定义设备", "自定义设备"}} {
		f.app.bindings[fmt.Sprint(i)] = binding{ClassID: "class_a", Expires: after(time.Minute)}
		class := f.app.state.Classes["class_a"]
		class.MaxDevices = 10
		f.app.state.Classes[class.ID] = class
		w := f.request(t, "POST", "/api/v1/device/bind", "", map[string]string{"code": fmt.Sprint(i), "name": tc.provided}, nil)
		expectStatus(t, w, 200)
		var result struct {
			Device Device `json:"device"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Device.Name != tc.expected {
			t.Fatalf("binding name: %s (%v)", w.Body.String(), err)
		}
	}
}

func TestMessageDurationChimeAndDismissal(t *testing.T) {
	f := newSecurityFixture(t)
	for _, tc := range []struct {
		style    string
		seconds  int
		chime    bool
		status   int
		expected bool
	}{
		{"notice", 15, true, 200, false}, {"praise", 600, true, 200, false}, {"urgent", 300, true, 200, true}, {"urgent", 60, false, 200, false}, {"urgent", 14, true, 400, false}, {"urgent", 601, true, 400, false},
	} {
		input := securityText("class_a")
		input["style"] = tc.style
		input["display_sec"] = tc.seconds
		input["chime"] = tc.chime
		w := f.request(t, "POST", "/api/v1/messages", f.token("owner"), input, nil)
		expectStatus(t, w, tc.status)
		if tc.status == 200 {
			var message Message
			if err := json.Unmarshal(w.Body.Bytes(), &message); err != nil || message.Chime != tc.expected {
				t.Fatalf("wrong chime: %s (%v)", w.Body.String(), err)
			}
		}
	}
	m := f.app.state.Messages["message_a"]
	m.Targets[0].Status = "played"
	f.app.state.Messages[m.ID] = m
	f.app.ack(f.app.state.Devices["device_a"], json.RawMessage(`{"message_id":"message_a","status":"dismissed"}`))
	if status := f.app.state.Messages[m.ID].Targets[0].Status; status != "dismissed" {
		t.Fatalf("played -> dismissed = %s", status)
	}
	f.app.ack(f.app.state.Devices["device_a"], json.RawMessage(`{"message_id":"message_a","status":"played"}`))
	if status := f.app.state.Messages[m.ID].Targets[0].Status; status != "dismissed" {
		t.Fatalf("dismissed regressed to %s", status)
	}
}

func prepareBackupFixture(t *testing.T) *securityFixture {
	t.Helper()
	f := newSecurityFixture(t)
	password := passwordHash("BackupPass123!")
	for key, user := range f.app.state.Users {
		user.PasswordHash = password
		f.app.state.Users[key] = user
	}
	audioPath := filepath.Join(f.app.config.DataDir, "audio", "audio_sample.webm")
	if err := os.WriteFile(audioPath, []byte("test recording"), 0600); err != nil {
		t.Fatal(err)
	}
	f.app.state.Audio["audio_sample"] = Audio{ID: "audio_sample", UserID: "owner_a", Path: audioPath, MIME: "audio/webm", CreatedAt: now()}
	f.app.state.Settings["site_name"] = "备份学校"
	f.app.state.Settings["theme"] = "slate"
	f.app.state.Sessions[hash("refresh-before-backup")] = Session{ID: hash("refresh-before-backup"), UserID: "owner_a", ExpiresAt: after(time.Hour)}
	device := f.app.state.Devices["device_a"]
	device.RemoteConfig = &RemoteConfig{ServerURL: "https://new.example", Revision: "revision-1"}
	f.app.state.Devices[device.ID] = device
	if err := f.app.Save(); err != nil {
		t.Fatal(err)
	}
	return f
}

func getBackup(t *testing.T, f *securityFixture) []byte {
	t.Helper()
	w := f.request(t, "GET", "/api/v1/admin/backup", f.token("super"), nil, nil)
	expectStatus(t, w, 200)
	if w.Header().Get("Content-Type") != "application/zip" || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("bad download headers: %v", w.Header())
	}
	return w.Body.Bytes()
}

func restoreBackup(t *testing.T, f *securityFixture, token string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", "/api/v1/admin/restore", bytes.NewReader(data))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/zip")
	r.Header.Set("X-ClassDing-Restore", "replace")
	w := httptest.NewRecorder()
	f.app.Handler().ServeHTTP(w, r)
	return w
}

func TestBackupRestoreToDifferentDirectory(t *testing.T) {
	src := prepareBackupFixture(t)
	backup := getBackup(t, src)
	dst := newSecurityFixture(t)
	oldAccess := dst.token("owner")
	oldAdmin := dst.token("super")
	owner := dst.app.state.Users["owner_a"]
	owner.Username = "oldowner"
	dst.app.state.Users[owner.ID] = owner
	dst.app.state.Users["legacy"] = User{ID: "legacy", Username: "owner_a", Role: "user", Status: "active"}
	if err := dst.app.Save(); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(dst.app.config.DataDir, "audio", "old.webm")
	if err := os.WriteFile(oldPath, []byte("old recording"), 0600); err != nil {
		t.Fatal(err)
	}
	dst.app.state.Sessions[hash("old-refresh")] = Session{ID: hash("old-refresh"), UserID: "owner_a", ExpiresAt: after(time.Hour)}
	w := restoreBackup(t, dst, oldAdmin, backup)
	expectStatus(t, w, 200)
	if w.Body.String() != "{\"ok\":true}\n" {
		t.Fatalf("unexpected result: %s", w.Body.String())
	}
	if _, exists := dst.app.state.Users["legacy"]; exists {
		t.Fatal("restore retained old user")
	}
	if dst.app.state.Users["owner_a"].PasswordHash != src.app.state.Users["owner_a"].PasswordHash {
		t.Fatal("password hash not preserved")
	}
	if dst.app.state.Devices["device_a"].TokenHash != hash("token-a") || dst.app.state.Devices["device_a"].ClassID != "class_a" {
		t.Fatal("device identity not preserved")
	}
	if dst.app.state.Settings["site_name"] != "备份学校" || dst.app.state.Settings["theme"] != "slate" {
		t.Fatal("settings not restored")
	}
	if len(dst.app.state.Sessions) != 0 {
		t.Fatal("refresh sessions survived restore")
	}
	expectStatus(t, dst.request(t, "GET", "/api/v1/me", oldAccess, nil, nil), 401)
	expectStatus(t, dst.request(t, "GET", "/api/v1/admin/settings", oldAdmin, nil, nil), 401)
	expectStatus(t, dst.request(t, "POST", "/api/v1/auth/refresh", "", nil, &http.Cookie{Name: "classding_refresh", Value: "old-refresh"}), 401)
	me := dst.request(t, "GET", "/api/v1/device/me", "token-a", nil, nil)
	expectStatus(t, me, 200)
	if !strings.Contains(me.Body.String(), "revision-1") {
		t.Fatalf("remote config missing: %s", me.Body.String())
	}
	newPath := dst.app.state.Audio["audio_sample"].Path
	if newPath == src.app.state.Audio["audio_sample"].Path || !strings.HasPrefix(newPath, dst.app.config.DataDir+string(filepath.Separator)) {
		t.Fatalf("audio path not rewritten: %s", newPath)
	}
	content, err := os.ReadFile(newPath)
	if err != nil || string(content) != "test recording" {
		t.Fatalf("audio not restored: %q %v", content, err)
	}
	if _, err = os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old audio still present: %v", err)
	}
	expectStatus(t, dst.request(t, "POST", "/api/v1/auth/login", "", map[string]string{"username": "owner_a", "password": "BackupPass123!"}, nil), 200)
	var persisted State
	if err = dst.app.store.Load(context.Background(), &persisted); err != nil || persisted.Devices["device_a"].TokenHash != hash("token-a") || persisted.Audio["audio_sample"].Path != newPath {
		t.Fatalf("restore not persisted: %v", err)
	}
	// A restored recording must remain exportable and portable on the next backup.
	newAdmin := dst.app.access(dst.app.state.Users["super"])
	secondBackup := dst.request(t, "GET", "/api/v1/admin/backup", newAdmin, nil, nil)
	expectStatus(t, secondBackup, 200)
	third := newSecurityFixture(t)
	expectStatus(t, restoreBackup(t, third, third.token("super"), secondBackup.Body.Bytes()), 200)
	secondAudio, err := os.ReadFile(third.app.state.Audio["audio_sample"].Path)
	if err != nil || string(secondAudio) != "test recording" {
		t.Fatalf("restored recording was not portable: %q %v", secondAudio, err)
	}
}

type unreadBody struct{ read bool }

func (b *unreadBody) Read([]byte) (int, error) { b.read = true; return 0, io.EOF }
func (*unreadBody) Close() error               { return nil }

func TestRestoreAuthenticatesBeforeBodyAndChecksOrigin(t *testing.T) {
	f := newSecurityFixture(t)
	for _, tc := range []struct {
		token, origin string
		status        int
	}{{"", "", 401}, {f.token("school"), "", 403}, {f.token("super"), "https://evil.example", 403}} {
		body := &unreadBody{}
		r := httptest.NewRequest("POST", "/api/v1/admin/restore", nil)
		r.Body = body
		r.Header.Set("Authorization", "Bearer "+tc.token)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Content-Type", "application/zip")
		r.Header.Set("X-ClassDing-Restore", "replace")
		w := httptest.NewRecorder()
		f.app.Handler().ServeHTTP(w, r)
		expectStatus(t, w, tc.status)
		if body.read {
			t.Fatal("unauthorized request body was read")
		}
	}
}

func alterBackup(t *testing.T, raw []byte, mutate func(map[string][]byte)) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, f := range zr.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		files[f.Name] = b
	}
	mutate(files)
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err = zw.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestRestoreRejectsCorruptionTraversalAndBrokenRelations(t *testing.T) {
	src := prepareBackupFixture(t)
	raw := getBackup(t, src)
	dst := newSecurityFixture(t)
	token := dst.token("super")
	cases := []struct {
		name   string
		mutate func(map[string][]byte)
	}{
		{"missing audio", func(f map[string][]byte) { delete(f, "audio/audio_sample.webm") }},
		{"modified audio", func(f map[string][]byte) { f["audio/audio_sample.webm"] = []byte("fake recording") }},
		{"path traversal", func(f map[string][]byte) { f["../escape"] = []byte("bad") }},
		{"wrong version", func(f map[string][]byte) {
			f["manifest.json"] = bytes.Replace(f["manifest.json"], []byte(`"version":1`), []byte(`"version":99`), 1)
		}},
		{"missing class", func(f map[string][]byte) {
			var m backupManifest
			if err := json.Unmarshal(f["manifest.json"], &m); err != nil {
				t.Fatal(err)
			}
			delete(m.State.Classes, "class_a")
			b, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			f["manifest.json"] = b
		}},
		{"missing collection", func(f map[string][]byte) {
			var m map[string]any
			if err := json.Unmarshal(f["manifest.json"], &m); err != nil {
				t.Fatal(err)
			}
			delete(m["state"].(map[string]any), "users")
			b, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			f["manifest.json"] = b
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := restoreBackup(t, dst, token, alterBackup(t, raw, tc.mutate))
			expectStatus(t, w, 400)
			if dst.app.state.Settings["site_name"] != "课堂叮咚" || len(dst.app.state.Devices) != 2 {
				t.Fatal("rejected restore changed state")
			}
		})
	}
	expectStatus(t, restoreBackup(t, dst, token, []byte("not a zip")), 400)
}

func TestRestoreRollsBackFilesAndStateOnDatabaseFailure(t *testing.T) {
	src := prepareBackupFixture(t)
	raw := getBackup(t, src)
	dst := newSecurityFixture(t)
	oldPath := filepath.Join(dst.app.config.DataDir, "audio", "old.webm")
	if err := os.WriteFile(oldPath, []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}
	reopen := closeStoreForRollback(t, dst.app)
	expectStatus(t, restoreBackup(t, dst, dst.token("super"), raw), 500)
	data, err := os.ReadFile(oldPath)
	if err != nil || string(data) != "keep me" {
		t.Fatalf("original audio not restored: %q %v", data, err)
	}
	if dst.app.state.Settings["site_name"] != "课堂叮咚" || len(dst.app.state.Devices) != 2 {
		t.Fatal("failed restore changed state")
	}
	if _, err := os.Stat(filepath.Join(dst.app.config.DataDir, "audio", "audio_sample.webm")); !os.IsNotExist(err) {
		t.Fatal("failed restore left new audio")
	}
	reopen()
}

func TestRestoreRejectsArchiveSizeAndFileCountLimits(t *testing.T) {
	f := prepareBackupFixture(t)
	raw := getBackup(t, f)
	var many bytes.Buffer
	writer := zip.NewWriter(&many)
	for i := 0; i <= maxBackupFiles; i++ {
		if _, err := writer.Create(fmt.Sprintf("file-%05d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	stage := t.TempDir()
	manyPath := filepath.Join(t.TempDir(), "many.zip")
	if err := os.WriteFile(manyPath, many.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareRestore(manyPath, stage); err == nil || !strings.Contains(err.Error(), "file count") {
		t.Fatalf("file count was not limited: %v", err)
	}

	// The central directory advertises an oversized uncompressed file; reject
	// before opening it, even when the compressed archive itself is tiny.
	oversized := append([]byte(nil), raw...)
	central := bytes.Index(oversized, []byte{'P', 'K', 1, 2})
	if central < 0 {
		t.Fatal("central directory missing")
	}
	advertised := uint32(maxBackupUnpacked + 1)
	for i := 0; i < 4; i++ {
		oversized[central+24+i] = byte(advertised >> (8 * i))
	}
	oversizedPath := filepath.Join(t.TempDir(), "oversized.zip")
	if err := os.WriteFile(oversizedPath, oversized, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareRestore(oversizedPath, stage); err == nil || !strings.Contains(err.Error(), "expanded backup too large") {
		t.Fatalf("expanded size was not limited: %v", err)
	}
}
