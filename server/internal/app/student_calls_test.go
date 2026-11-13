package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const studentCallsPath = "/api/v1/student-calls"
const deviceCallsPath = "/api/v1/device/student-calls"

func postStudentCall(t *testing.T, f *securityFixture, token, key string) StudentCall {
	t.Helper()
	w := f.request(t, "POST", deviceCallsPath, token, map[string]string{"request_id": key}, nil)
	expectStatus(t, w, 200)
	var item StudentCall
	if err := json.Unmarshal(w.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(w.Body.String(), "request_id") {
		t.Fatal("internal replay key leaked")
	}
	for _, key := range []string{"read_at", "read_by", "read_by_name"} {
		if !strings.Contains(w.Body.String(), `"`+key+`":""`) && item.ReadAt == "" {
			t.Fatalf("missing empty field %s: %s", key, w.Body)
		}
	}
	return item
}

func listStudentCall(t *testing.T, f *securityFixture, who, suffix string) studentCallList {
	t.Helper()
	w := f.request(t, "GET", studentCallsPath+suffix, f.token(who), nil, nil)
	expectStatus(t, w, 200)
	var list studentCallList
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	return list
}

func callPeer(t *testing.T, a *App, uid string) *Peer {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	u := a.state.Users[uid]
	session, _ := a.verify(a.access(u))
	p := &Peer{ctx: ctx, cancel: cancel, UserID: uid, queue: make(chan Frame, 256), teacherSession: session}
	a.hub.addTeacher(p)
	t.Cleanup(func() { cancel(); a.hub.removeTeacherPeer(p) })
	return p
}

func takeCallFrame(t *testing.T, p *Peer, item StudentCall, kind string) {
	t.Helper()
	select {
	case frame := <-p.queue:
		if frame.T != "student_call" || !reflect.DeepEqual(frame.D, map[string]any{"id": item.ID, "class_id": item.ClassID, "kind": kind}) {
			t.Fatalf("wrong frame: %+v", frame)
		}
	default:
		t.Fatal("missing call frame")
	}
}

func TestStudentCallsPermissionsReplayAndSharedRead(t *testing.T) {
	f := newSecurityFixture(t)
	owner := callPeer(t, f.app, "owner_a")
	collab := callPeer(t, f.app, "collab_a")
	outsiders := []*Peer{callPeer(t, f.app, "school_a"), callPeer(t, f.app, "super"), callPeer(t, f.app, "owner_b"), callPeer(t, f.app, "outsider")}
	item := postStudentCall(t, f, "token-a", "request-1")
	if item.ClassID != "class_a" || item.DeviceID != "device_a" || item.ClassName != "甲班" || item.DeviceName != "甲设备" || item.ReadAt != "" {
		t.Fatalf("wrong snapshot: %+v", item)
	}
	for _, p := range []*Peer{owner, collab} {
		takeCallFrame(t, p, item, "created")
	}
	if replay := postStudentCall(t, f, "token-a", "request-1"); replay != item {
		t.Fatal("replay changed record")
	}
	for _, who := range []string{"school", "super", "other", "outsider"} {
		if list := listStudentCall(t, f, who, ""); len(list.Items) != 0 || list.UnreadCount != 0 {
			t.Fatalf("%s leaked calls", who)
		}
		expectStatus(t, f.request(t, "GET", studentCallsPath+"?class_id=class_a", f.token(who), nil, nil), 403)
		expectStatus(t, f.request(t, "POST", studentCallsPath+"/"+item.ID+"/read", f.token(who), nil, nil), 403)
	}
	expectStatus(t, f.request(t, "GET", studentCallsPath, "", nil, nil), 401)
	expectStatus(t, f.request(t, "GET", studentCallsPath, "token-a", nil, nil), 401)
	expectStatus(t, f.request(t, "POST", deviceCallsPath, f.token("owner"), map[string]string{"request_id": "teacher"}, nil), 401)
	expectStatus(t, f.request(t, "POST", studentCallsPath+"/"+item.ID+"/read", "token-a", nil, nil), 401)
	expectStatus(t, f.request(t, "POST", deviceCallsPath, "token-b", map[string]string{"request_id": "request-1"}, nil), 409)
	w := f.request(t, "POST", deviceCallsPath, "token-a", map[string]string{"request_id": "request-2"}, nil)
	expectStatus(t, w, 429)
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("cooldown lacks Retry-After")
	}
	for _, who := range []string{"owner", "collab"} {
		if got := listStudentCall(t, f, who, ""); got.UnreadCount != 1 || len(got.Items) != 1 || got.Items[0] != item {
			t.Fatal("list should not auto-read")
		}
	}
	w = f.request(t, "POST", studentCallsPath+"/"+item.ID+"/read", f.token("collab"), nil, nil)
	expectStatus(t, w, 200)
	var read StudentCall
	if err := json.Unmarshal(w.Body.Bytes(), &read); err != nil {
		t.Fatal(err)
	}
	if read.ReadAt == "" || read.ReadBy != "collab_a" || read.ReadByName != "乙老师" {
		t.Fatalf("wrong processing snapshot: %+v", read)
	}
	for _, p := range []*Peer{owner, collab} {
		takeCallFrame(t, p, item, "read")
	}
	w = f.request(t, "POST", studentCallsPath+"/"+item.ID+"/read", f.token("owner"), nil, nil)
	expectStatus(t, w, 200)
	var again StudentCall
	_ = json.Unmarshal(w.Body.Bytes(), &again)
	if again != read || postStudentCall(t, f, "token-a", "request-1") != read {
		t.Fatal("idempotent read/replay changed first processor")
	}
	if got := listStudentCall(t, f, "owner", ""); got.UnreadCount != 0 || len(got.Items) != 1 {
		t.Fatal("shared read lost record or unread did not clear")
	}
	for _, p := range append(outsiders, owner, collab) {
		if len(p.queue) != 0 {
			t.Fatal("unauthorized or duplicate event")
		}
	}
}

func TestStudentCallsStrictInputBindingAndLicenses(t *testing.T) {
	f := newSecurityFixture(t)
	for _, body := range []any{nil, map[string]any{}, map[string]any{"request_id": nil}, map[string]any{"request_id": 3}, map[string]any{"request_id": ""}, map[string]any{"request_id": " white "}, map[string]any{"request_id": strings.Repeat("a", 129)}, map[string]any{"request_id": "中文"}, map[string]any{"request_id": "ok", "class_id": "class_b"}, map[string]any{"request_id": "ok", "device_id": "device_b"}} {
		expectStatus(t, f.request(t, "POST", deviceCallsPath, "token-a", body, nil), 400)
	}
	for _, raw := range []string{`null`, `{"request_id":"ok"}{}`, `["ok"]`} {
		req := httptest.NewRequest("POST", deviceCallsPath, strings.NewReader(raw))
		req.Header.Set("Authorization", "Bearer token-a")
		w := httptest.NewRecorder()
		commitState(f.app)
		f.app.Handler().ServeHTTP(w, req)
		expectStatus(t, w, 400)
	}
	item := postStudentCall(t, f, "token-a", "aA0_.:-")
	d := f.app.state.Devices["device_a"]
	d.ClassID = "class_b"
	f.app.state.Devices[d.ID] = d
	expectStatus(t, f.request(t, "POST", deviceCallsPath, "token-a", map[string]string{"request_id": "aA0_.:-"}, nil), 409)
	moved := postStudentCall(t, f, "token-a", "moved")
	if moved.ClassID != "class_b" || item.ClassID != "class_a" {
		t.Fatal("binding not authoritative")
	}
	d.ClassID = "class_a"
	f.app.state.Devices[d.ID] = d
	for _, mode := range []string{"expired", "revoked", "released", "missing", "wrong_class"} {
		original := f.app.state.Codes["CODE-A"]
		code := original
		switch mode {
		case "expired":
			expired := stamp(time.Now().Add(-time.Hour))
			code.ExpiresAt = &expired
		case "revoked":
			code.Status = LicenseRevoked
		case "released":
			code.Status = LicenseReleased
		case "wrong_class":
			other := "class_b"
			code.ClassID = &other
		}
		f.app.state.Codes["CODE-A"] = code
		if mode == "missing" {
			delete(f.app.state.Codes, "CODE-A")
		}
		expectStatus(t, f.request(t, "POST", deviceCallsPath, "token-a", map[string]string{"request_id": "aA0_.:-"}, nil), 403)
		f.app.state.Codes["CODE-A"] = original
	}
	delete(f.app.state.Devices, "device_a")
	expectStatus(t, f.request(t, "POST", deviceCallsPath, "token-a", map[string]string{"request_id": "aA0_.:-"}, nil), 401)
}

func TestStudentCallsClassWideCooldownAndRollback(t *testing.T) {
	f := newSecurityFixture(t)
	p := callPeer(t, f.app, "owner_a")
	restore := closeStoreForRollback(t, f.app)
	expectStatus(t, f.request(t, "POST", deviceCallsPath, "token-a", map[string]string{"request_id": "retry"}, nil), 500)
	if len(f.app.state.StudentCalls) != 0 || len(p.queue) != 0 {
		t.Fatal("failed create leaked state/event")
	}
	restore()
	item := postStudentCall(t, f, "token-a", "retry")
	takeCallFrame(t, p, item, "created")
	// A second device shares the class cooldown, not its own separate timer.
	f.app.state.Devices["device_a2"] = Device{ID: "device_a2", ClassID: "class_a", Name: "second", TokenHash: hash("token-a2")}
	expectStatus(t, f.request(t, "POST", deviceCallsPath, "token-a2", map[string]string{"request_id": "retry"}, nil), 409)
	expectStatus(t, f.request(t, "POST", deviceCallsPath, "token-a2", map[string]string{"request_id": "second"}, nil), 429)
	postStudentCall(t, f, "token-b", "other-class")
	restore = closeStoreForRollback(t, f.app)
	expectStatus(t, f.request(t, "POST", studentCallsPath+"/"+item.ID+"/read", f.token("owner"), nil, nil), 500)
	if f.app.state.StudentCalls[item.ID].ReadAt != "" || len(p.queue) != 0 {
		t.Fatal("failed read consumed state/event")
	}
	restore()
	expectStatus(t, f.request(t, "POST", studentCallsPath+"/"+item.ID+"/read", f.token("collab"), nil, nil), 200)
	takeCallFrame(t, p, item, "read")
	old := f.app.state.StudentCalls[item.ID]
	old.CreatedAt = stamp(time.Now().Add(-31 * time.Second))
	f.app.state.StudentCalls[item.ID] = old
	postStudentCall(t, f, "token-a2", "second")
}

func TestStudentCallsConcurrentReadAndCreate(t *testing.T) {
	f := newSecurityFixture(t)
	var wg sync.WaitGroup
	ids := make(chan string, 12)
	for i := 0; i < 12; i++ {
		wg.Go(func() {
			w := f.request(t, "POST", deviceCallsPath, "token-a", map[string]string{"request_id": "concurrent"}, nil)
			expectStatus(t, w, 200)
			var item StudentCall
			_ = json.Unmarshal(w.Body.Bytes(), &item)
			ids <- item.ID
		})
	}
	wg.Wait()
	close(ids)
	var id string
	for got := range ids {
		if id != "" && got != id {
			t.Fatal("duplicate request created different records")
		}
		id = got
	}
	if len(f.app.state.StudentCalls) != 1 {
		t.Fatal("duplicate records")
	}
	reads := make(chan StudentCall, 12)
	for i := 0; i < 12; i++ {
		who := "owner"
		if i%2 == 0 {
			who = "collab"
		}
		wg.Go(func() {
			w := f.request(t, "POST", studentCallsPath+"/"+id+"/read", f.token(who), nil, nil)
			expectStatus(t, w, 200)
			var item StudentCall
			_ = json.Unmarshal(w.Body.Bytes(), &item)
			reads <- item
		})
	}
	wg.Wait()
	close(reads)
	first := f.app.state.StudentCalls[id].StudentCall
	for got := range reads {
		if got != first {
			t.Fatal("concurrent read overwrote first processor")
		}
	}
	if first.ReadAt == "" || first.ReadBy == "" {
		t.Fatal("not processed")
	}
}

func TestStudentCallsBroadcastRechecksCurrentAccess(t *testing.T) {
	f := newSecurityFixture(t)
	item := postStudentCall(t, f, "token-a", "pending-event")
	owner := callPeer(t, f.app, "owner_a")
	collab := callPeer(t, f.app, "collab_a")
	outsider := callPeer(t, f.app, "outsider")
	f.app.studentCallChanged(f.app.state.StudentCalls[item.ID], "read")
	jobs := f.app.jobs
	f.app.jobs = nil
	delete(f.app.state.Members, memberKey("class_a", "collab_a"))
	u := f.app.state.Users["owner_a"]
	u.Status = AccountDisabled
	f.app.state.Users[u.ID] = u
	for _, job := range jobs {
		job()
	}
	for _, p := range []*Peer{owner, collab, outsider} {
		if len(p.queue) != 0 {
			t.Fatal("lost/absent permission leaked broadcast")
		}
	}
	expectStatus(t, f.request(t, "GET", studentCallsPath, f.token("owner"), nil, nil), 401)
	expectStatus(t, f.request(t, "POST", studentCallsPath+"/"+item.ID+"/read", f.token("collab"), nil, nil), 403)
	f.app.studentCallChanged(f.app.state.StudentCalls[item.ID], "created")
	jobs = f.app.jobs
	f.app.jobs = nil
	delete(f.app.state.StudentCalls, item.ID)
	for _, job := range jobs {
		job()
	}
	if len(collab.queue) != 0 {
		t.Fatal("deleted call event leaked")
	}
}

func TestStudentCallsPaginationCountsAndStableTimes(t *testing.T) {
	f := newSecurityFixture(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 105; i++ {
		id := fmt.Sprintf("call_%03d", i)
		f.app.state.StudentCalls[id] = studentCallRecord{StudentCall: StudentCall{ID: id, ClassID: "class_a", ClassName: "甲班", DeviceID: "device_a", DeviceName: "甲设备", CreatedAt: stamp(at.Add(time.Duration(i/2) * time.Second))}, RequestID: id}
	}
	first := listStudentCall(t, f, "owner", "")
	if len(first.Items) != 100 || first.UnreadCount != 105 || first.NextCursor == nil {
		t.Fatal("default window must not truncate unread count")
	}
	second := listStudentCall(t, f, "owner", "?cursor="+*first.NextCursor)
	if len(second.Items) != 5 || second.UnreadCount != 105 || second.NextCursor != nil {
		t.Fatal("wrong next page")
	}
	all := append(first.Items, second.Items...)
	seen := map[string]bool{}
	for _, item := range all {
		if seen[item.ID] {
			t.Fatal("cursor duplicated tied timestamp")
		}
		seen[item.ID] = true
	}
	for _, suffix := range []string{"?limit=0", "?limit=101", "?limit=oops", "?limit=", "?limit=1&limit=2", "?cursor=bad", "?cursor=" + base64.RawURLEncoding.EncodeToString([]byte(`{"created_at":"invalid","id":"x"}`))} {
		expectStatus(t, f.request(t, "GET", studentCallsPath+suffix, f.token("owner"), nil, nil), 400)
	}
	fractional := StudentCall{ID: "frac", CreatedAt: "2026-01-01T00:00:00.1Z"}
	whole := StudentCall{ID: "whole", CreatedAt: "2026-01-01T00:00:00Z"}
	if newestFirst(fractional.key(), whole.key()) >= 0 {
		t.Fatal("fractional timestamp sorted before whole second")
	}
	if got := listStudentCall(t, f, "school", ""); got.UnreadCount != 0 {
		t.Fatal("admin got school count")
	}
}

func TestStudentCallsPersistRestartAndClassDeletion(t *testing.T) {
	seed := newSecurityFixture(t)
	config := Config{DataDir: t.TempDir(), JWTSecret: strings.Repeat("r", 32)}
	a, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(seed.app.state)
	if err = json.Unmarshal(data, &a.state); err != nil {
		t.Fatal(err)
	}
	f := &securityFixture{app: a, users: seed.users}
	item := postStudentCall(t, f, "token-a", "restart")
	if err = a.Close(); err != nil {
		t.Fatal(err)
	}
	a, err = New(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	f.app = a
	if replay := postStudentCall(t, f, "token-a", "restart"); replay != item {
		t.Fatal("restart lost request dedupe")
	}
	expectStatus(t, f.request(t, "POST", deviceCallsPath, "token-a", map[string]string{"request_id": "restart-new"}, nil), 429)
	if got := listStudentCall(t, f, "owner", ""); got.UnreadCount != 1 {
		t.Fatal("offline records lost")
	}
	expectStatus(t, f.request(t, "POST", studentCallsPath+"/"+item.ID+"/read", f.token("collab"), nil, nil), 200)
	var persisted State
	if err = a.store.Load(context.Background(), &persisted); err != nil || persisted.StudentCalls[item.ID].ReadBy != "collab_a" {
		t.Fatal("shared read not durable", err)
	}
	expectStatus(t, f.request(t, "DELETE", "/api/v1/classes/class_a", f.token("owner"), nil, nil), 200)
	if len(a.state.StudentCalls) != 0 {
		t.Fatal("class deletion left call records")
	}
	persisted = State{}
	if err = a.store.Load(context.Background(), &persisted); err != nil || len(persisted.StudentCalls) != 0 {
		t.Fatal("class deletion not durable", err)
	}
}
