package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func homeworkBody(classID, subject, text string, visible, pinned bool) map[string]any {
	return map[string]any{"class_id": classID, "subject": subject, "text": text, "visible": visible, "always_on_top": pinned}
}

func postHomework(t *testing.T, f *securityFixture, user string, body map[string]any) Homework {
	t.Helper()
	w := f.request(t, http.MethodPost, "/api/v1/homeworks", f.token(user), body, nil)
	expectStatus(t, w, http.StatusOK)
	var item Homework
	if err := json.Unmarshal(w.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	return item
}

func TestHomeworkPermissionsValidationAndLimit(t *testing.T) {
	f := newSecurityFixture(t)
	path := "/api/v1/homeworks"
	valid := homeworkBody("class_a", "语文", "读课文", false, false)
	expectStatus(t, f.request(t, "GET", path+"?class_id=class_a", "", nil, nil), 401)
	expectStatus(t, f.request(t, "GET", path, f.token("owner"), nil, nil), 400)
	expectStatus(t, f.request(t, "GET", path+"?class_id=class_a", f.token("outsider"), nil, nil), 403)
	// Administrators open only the classes they joined in the teacher workspace.
	expectStatus(t, f.request(t, "GET", path+"?class_id=class_a", f.token("school"), nil, nil), 403)
	expectStatus(t, f.request(t, "GET", path+"?class_id=class_a", f.token("super"), nil, nil), 403)
	expectStatus(t, f.request(t, "POST", path, f.token("outsider"), valid, nil), 403)
	expectStatus(t, f.request(t, "POST", path, f.token("other"), valid, nil), 403)
	for _, body := range []map[string]any{
		homeworkBody("class_a", " ", "内容", true, false),
		homeworkBody("class_a", strings.Repeat("课", 25), "内容", true, false),
		homeworkBody("class_a", "数学", strings.Repeat("题", 301), true, false),
		homeworkBody("class_a", "数学", "\n  ", true, false),
		{"class_id": "class_a", "subject": "数学", "text": "内容", "visible": true},
		{"class_id": "class_a", "subject": "数学", "text": "内容", "visible": true, "always_on_top": false, "teacher_id": "owner_b"},
	} {
		expectStatus(t, f.request(t, "POST", path, f.token("owner"), body, nil), 400)
	}
	ownerItem := postHomework(t, f, "owner", valid)
	if ownerItem.TeacherID != "owner_a" || ownerItem.TeacherName != "甲老师" || ownerItem.Visible || ownerItem.CreatedAt == "" || ownerItem.UpdatedAt == "" {
		t.Fatalf("unexpected created homework: %+v", ownerItem)
	}
	collabItem := postHomework(t, f, "collab", homeworkBody("class_a", " 数学 ", " 做练习 ", true, true))
	if collabItem.TeacherID != "collab_a" || collabItem.TeacherName != "乙老师" || collabItem.Subject != "数学" || collabItem.Text != "做练习" {
		t.Fatalf("unexpected collaborator homework: %+v", collabItem)
	}
	ownerPath, collabPath := path+"/"+ownerItem.ID, path+"/"+collabItem.ID
	expectStatus(t, f.request(t, "PATCH", ownerPath, f.token("collab"), map[string]any{"text": "越权"}, nil), 403)
	expectStatus(t, f.request(t, "DELETE", ownerPath, f.token("collab"), nil, nil), 403)
	expectStatus(t, f.request(t, "PATCH", collabPath, f.token("collab"), map[string]any{"text": "自己修改"}, nil), 200)
	expectStatus(t, f.request(t, "PATCH", collabPath, f.token("owner"), map[string]any{"visible": false}, nil), 200)
	expectStatus(t, f.request(t, "PATCH", collabPath, f.token("school"), map[string]any{"always_on_top": false}, nil), 403)
	expectStatus(t, f.request(t, "PATCH", collabPath, f.token("other"), map[string]any{"text": "越权"}, nil), 403)
	expectStatus(t, f.request(t, "PATCH", collabPath, f.token("owner"), map[string]any{}, nil), 400)
	expectStatus(t, f.request(t, "PATCH", collabPath, f.token("owner"), map[string]any{"subject": ""}, nil), 400)
	expectStatus(t, f.request(t, "PATCH", collabPath, f.token("owner"), map[string]any{"subject": nil, "visible": false}, nil), 400)
	list := f.request(t, "GET", path+"?class_id=class_a", f.token("collab"), nil, nil)
	expectStatus(t, list, 200)
	var result struct {
		Items []Homework `json:"items"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &result); err != nil || len(result.Items) != 2 {
		t.Fatalf("teacher list omitted hidden items: %s (%v)", list.Body.String(), err)
	}
	deviceList := f.request(t, "GET", "/api/v1/device/homeworks", "token-a", nil, nil)
	expectStatus(t, deviceList, 200)
	if err := json.Unmarshal(deviceList.Body.Bytes(), &result); err != nil || len(result.Items) != 2 {
		t.Fatalf("device list omitted hidden items: %s (%v)", deviceList.Body.String(), err)
	}
	otherDevice := f.request(t, "GET", "/api/v1/device/homeworks", "token-b", nil, nil)
	if strings.Contains(otherDevice.Body.String(), ownerItem.ID) || !strings.Contains(otherDevice.Body.String(), `"items":[]`) {
		t.Fatalf("device leaked another class: %s", otherDevice.Body.String())
	}
	expectStatus(t, f.request(t, "GET", "/api/v1/device/homeworks", "bad", nil, nil), 401)
	for i := 0; i < 10; i++ {
		postHomework(t, f, "collab", homeworkBody("class_a", "英语", fmt.Sprint(i), true, false))
	}
	expectStatus(t, f.request(t, "POST", path, f.token("owner"), valid, nil), 409)
	expectStatus(t, f.request(t, "DELETE", collabPath, f.token("school"), nil, nil), 403)
	expectStatus(t, f.request(t, "DELETE", collabPath, f.token("owner"), nil, nil), 200)
	expectStatus(t, f.request(t, "DELETE", ownerPath, f.token("owner"), nil, nil), 200)
}

func TestHomeworkCommitRollbackAndPersistence(t *testing.T) {
	f := newSecurityFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	peer := &Peer{ctx: ctx, cancel: cancel, queue: make(chan Frame, 4), ClassID: "class_a"}
	f.app.hub.devices["device_a"] = peer
	// Remove the synthetic peer before App.Close tries to close its connection.
	t.Cleanup(func() { f.app.hub.mu.Lock(); delete(f.app.hub.devices, "device_a"); f.app.hub.mu.Unlock() })
	reopen := closeStoreForRollback(t, f.app)
	body := homeworkBody("class_a", "数学", "完成练习", true, false)
	expectStatus(t, f.request(t, "POST", "/api/v1/homeworks", f.token("owner"), body, nil), 500)
	if len(f.app.state.Homeworks) != 0 || f.app.state.Classes["class_a"].HomeworkRevision != 0 || len(peer.queue) != 0 {
		t.Fatal("failed write changed state, revision, or broadcast a snapshot")
	}
	reopen()
	item := postHomework(t, f, "owner", body)
	frame := <-peer.queue
	if frame.T != "homework" || frame.D.(map[string]any)["homework_revision"] != int64(1) {
		t.Fatalf("bad committed snapshot: %+v", frame)
	}
	var persisted State
	if err := f.app.store.Load(context.Background(), &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Homeworks[item.ID].Text != "完成练习" || persisted.Classes["class_a"].HomeworkRevision != 1 {
		t.Fatalf("homework or revision did not persist: %+v", persisted.Homeworks[item.ID])
	}
}

func TestHomeworkRevisionDoesNotOverflow(t *testing.T) {
	f := newSecurityFixture(t)
	c := f.app.state.Classes["class_a"]
	c.HomeworkRevision = maxHomeworkRevision
	f.app.state.Classes[c.ID] = c
	expectStatus(t, f.request(t, "POST", "/api/v1/homeworks", f.token("owner"), homeworkBody("class_a", "数学", "作业", true, false), nil), 409)
	if len(f.app.state.Homeworks) != 0 || f.app.state.Classes[c.ID].HomeworkRevision != maxHomeworkRevision {
		t.Fatal("overflowing revision changed homework state")
	}
}

func TestHomeworkUserAndClassDeletion(t *testing.T) {
	f := newSecurityFixture(t)
	own := postHomework(t, f, "owner", homeworkBody("class_a", "数学", "甲", true, false))
	collab := postHomework(t, f, "collab", homeworkBody("class_a", "语文", "乙", true, false))
	before := f.app.state.Classes["class_a"].HomeworkRevision
	expectStatus(t, f.request(t, "DELETE", "/api/v1/admin/users/collab_a", f.token("school"), nil, nil), 200)
	if _, exists := f.app.state.Homeworks[collab.ID]; exists || f.app.state.Classes["class_a"].HomeworkRevision != before+1 {
		t.Fatal("deleting teacher did not delete authored homework and increment revision")
	}
	if _, exists := f.app.state.Homeworks[own.ID]; !exists {
		t.Fatal("deleting teacher removed another author's homework")
	}
	expectStatus(t, f.request(t, "DELETE", "/api/v1/classes/class_a", f.token("owner"), nil, nil), 200)
	if len(f.app.state.Homeworks) != 0 {
		t.Fatal("deleting class left homework records")
	}
	var persisted State
	if err := f.app.store.Load(context.Background(), &persisted); err != nil || len(persisted.Homeworks) != 0 {
		t.Fatalf("deleted homework was not removed from storage: %v", err)
	}
}

func TestHomeworkConcurrentSnapshotsDoNotRegress(t *testing.T) {
	f := newSecurityFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	peer := &Peer{ctx: ctx, cancel: cancel, queue: make(chan Frame, 16), ClassID: "class_a"}
	f.app.hub.devices["device_a"] = peer
	t.Cleanup(func() { f.app.hub.mu.Lock(); delete(f.app.hub.devices, "device_a"); f.app.hub.mu.Unlock() })
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := f.request(t, "POST", "/api/v1/homeworks", f.token("owner"), homeworkBody("class_a", "数学", fmt.Sprint(i), true, false), nil)
			if w.Code != 200 {
				t.Errorf("create failed: %s", w.Body.String())
			}
		}(i)
	}
	wg.Wait()
	var previous int64
	for len(peer.queue) > 0 {
		frame := <-peer.queue
		data := frame.D.(map[string]any)
		revision := data["homework_revision"].(int64)
		if revision < previous || int64(len(data["items"].([]Homework))) != revision {
			t.Fatalf("out-of-order or inconsistent snapshot: previous=%d current=%+v", previous, data)
		}
		previous = revision
	}
	if previous != 8 {
		t.Fatalf("latest snapshot missing: %d", previous)
	}
}

func TestHomeworkBroadcastOnlyReachesAuthorizedClass(t *testing.T) {
	f := newSecurityFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	peer := func(classID string) *Peer {
		return &Peer{ctx: ctx, cancel: cancel, queue: make(chan Frame, 2), ClassID: classID}
	}
	deviceA, deviceB := peer("class_a"), peer("class_b")
	f.app.hub.devices["device_a"] = deviceA
	f.app.hub.devices["device_b"] = deviceB
	teachers := map[string]*Peer{}
	for _, user := range []string{"owner_a", "collab_a", "school_a", "super", "owner_b", "outsider"} {
		teachers[user] = peer("")
		f.app.hub.teachers[user] = map[*Peer]bool{teachers[user]: true}
	}
	t.Cleanup(func() {
		f.app.hub.mu.Lock()
		clear(f.app.hub.devices)
		clear(f.app.hub.teachers)
		f.app.hub.mu.Unlock()
	})
	postHomework(t, f, "owner", homeworkBody("class_a", "英语", "仅甲班", false, false))
	if len(deviceA.queue) != 1 || len(deviceB.queue) != 0 {
		t.Fatal("device received a snapshot from the wrong class")
	}
	for _, user := range []string{"owner_a", "collab_a"} {
		if len(teachers[user].queue) != 1 {
			t.Errorf("authorized teacher %s did not receive snapshot", user)
		}
	}
	for _, user := range []string{"school_a", "super", "owner_b", "outsider"} {
		if len(teachers[user].queue) != 0 {
			t.Errorf("unauthorized teacher %s received snapshot", user)
		}
	}
}

func TestHomeworkWebsocketReconnectAndDeviceBinding(t *testing.T) {
	_, server, token := rtSetup(t)
	var classes []Class
	rtRequest(t, server.URL, "GET", "/classes", token, nil, 200, &classes)
	classID := classes[0].ID
	var initial Homework
	rtRequest(t, server.URL, "POST", "/homeworks", token, homeworkBody(classID, "语文", "离线作业", false, false), 200, &initial)
	var code struct {
		Code string `json:"code"`
	}
	rtRequest(t, server.URL, "POST", "/classes/"+classID+"/bind-code", token, nil, 200, &code)
	var bound struct {
		Token     string     `json:"device_token"`
		Homeworks []Homework `json:"homeworks"`
		Revision  int64      `json:"homework_revision"`
	}
	rtRequest(t, server.URL, "POST", "/device/bind", "", map[string]string{"code": code.Code}, 200, &bound)
	if len(bound.Homeworks) != 1 || bound.Homeworks[0].ID != initial.ID || bound.Homeworks[0].Visible || bound.Revision != 1 {
		t.Fatalf("binding snapshot missing: %+v", bound)
	}
	device, welcome := rtConnect(t, server.URL, bound.Token, nil)
	if items, ok := welcome["homeworks"].([]any); !ok || len(items) != 1 || welcome["homework_revision"] != float64(1) {
		t.Fatalf("welcome snapshot missing: %+v", welcome)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	teacher, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws/teacher", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { teacher.CloseNow() })
	if err := wsjson.Write(ctx, teacher, map[string]any{"t": "hello", "d": map[string]string{"token": token}}); err != nil {
		t.Fatal(err)
	}
	rtRead(t, teacher, "welcome")
	var created Homework
	rtRequest(t, server.URL, "POST", "/homeworks", token, homeworkBody(classID, "数学", "线上作业", true, true), 200, &created)
	for _, conn := range []*websocket.Conn{device, teacher} {
		frame := rtRead(t, conn, "homework")
		data := frame.D.(map[string]any)
		if data["homework_revision"] != float64(2) || len(data["items"].([]any)) != 2 {
			t.Fatalf("incomplete live snapshot: %+v", data)
		}
	}
	device.CloseNow()
	rtRequest(t, server.URL, "PATCH", "/homeworks/"+created.ID, token, map[string]bool{"visible": false}, 200, nil)
	rtRequest(t, server.URL, "DELETE", "/homeworks/"+initial.ID, token, nil, 200, nil)
	_, welcome = rtConnect(t, server.URL, bound.Token, nil)
	items, ok := welcome["homeworks"].([]any)
	if !ok || len(items) != 1 || items[0].(map[string]any)["id"] != created.ID || items[0].(map[string]any)["visible"] != false || welcome["homework_revision"] != float64(4) {
		t.Fatalf("reconnect failed to restore complete snapshot: %+v", welcome)
	}
}

func TestHomeworkBackupCompatibilityAndValidation(t *testing.T) {
	src := prepareBackupFixture(t)
	legacy := alterBackup(t, getBackup(t, src), func(files map[string][]byte) {
		var manifest map[string]any
		if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
			t.Fatal(err)
		}
		delete(manifest["state"].(map[string]any), "homeworks")
		files["manifest.json"], _ = json.Marshal(manifest)
	})
	oldDestination := newSecurityFixture(t)
	expectStatus(t, restoreBackup(t, oldDestination, oldDestination.token("super"), legacy), 200)
	if oldDestination.app.state.Homeworks == nil || len(oldDestination.app.state.Homeworks) != 0 {
		t.Fatal("legacy backup did not initialize an empty homework collection")
	}
	// A restored legacy archive must still allow the next homework to be created.
	postHomework(t, oldDestination, "owner", homeworkBody("class_a", "语文", "兼容旧备份", true, false))

	item := postHomework(t, src, "collab", homeworkBody("class_a", "数学", "隐藏作业", false, true))
	archive := getBackup(t, src)
	destination := newSecurityFixture(t)
	expectStatus(t, restoreBackup(t, destination, destination.token("super"), archive), 200)
	if got := destination.app.state.Homeworks[item.ID]; got.Text != item.Text || got.Visible || destination.app.state.Classes["class_a"].HomeworkRevision != 1 {
		t.Fatalf("homework backup lost data: %+v", got)
	}
	var persisted State
	if err := destination.app.store.Load(context.Background(), &persisted); err != nil || persisted.Homeworks[item.ID].ID != item.ID {
		t.Fatalf("restored homework was not persisted: %v", err)
	}

	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"null collection", func(state map[string]any) { state["homeworks"] = nil }},
		{"missing collection with revision", func(state map[string]any) { delete(state, "homeworks") }},
		{"missing display option", func(state map[string]any) {
			delete(state["homeworks"].(map[string]any)[item.ID].(map[string]any), "visible")
		}},
		{"null display option", func(state map[string]any) {
			state["homeworks"].(map[string]any)[item.ID].(map[string]any)["always_on_top"] = nil
		}},
		{"missing teacher", func(state map[string]any) {
			state["homeworks"].(map[string]any)[item.ID].(map[string]any)["teacher_id"] = "missing"
		}},
		{"missing class", func(state map[string]any) {
			state["homeworks"].(map[string]any)[item.ID].(map[string]any)["class_id"] = "missing"
		}},
		{"empty subject", func(state map[string]any) {
			state["homeworks"].(map[string]any)[item.ID].(map[string]any)["subject"] = " "
		}},
		{"invalid timestamp", func(state map[string]any) {
			state["homeworks"].(map[string]any)[item.ID].(map[string]any)["updated_at"] = "invalid"
		}},
		{"missing revision", func(state map[string]any) {
			delete(state["classes"].(map[string]any)["class_a"].(map[string]any), "homework_revision")
		}},
		{"revision above safe integer", func(state map[string]any) {
			state["classes"].(map[string]any)["class_a"].(map[string]any)["homework_revision"] = float64(maxHomeworkRevision + 1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			corrupt := alterBackup(t, archive, func(files map[string][]byte) {
				var manifest map[string]any
				if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
					t.Fatal(err)
				}
				tc.mutate(manifest["state"].(map[string]any))
				files["manifest.json"], _ = json.Marshal(manifest)
			})
			f := newSecurityFixture(t)
			expectStatus(t, restoreBackup(t, f, f.token("super"), corrupt), 400)
		})
	}
}

func TestHomeworkMigrationRejectsStaleSnapshot(t *testing.T) {
	src := prepareBackupFixture(t)
	item := postHomework(t, src, "owner", homeworkBody("class_a", "数学", "旧备份包含", false, false))
	archive := getBackup(t, src)
	// Even deletion leaves a higher revision, so a stale backup cannot revive it.
	expectStatus(t, src.request(t, "DELETE", "/api/v1/homeworks/"+item.ID, src.token("owner"), nil, nil), 200)
	me := src.request(t, "GET", "/api/v1/device/me", "token-a", nil, nil)
	var source struct {
		Revision  int64      `json:"homework_revision"`
		Homeworks []Homework `json:"homeworks"`
	}
	if err := json.Unmarshal(me.Body.Bytes(), &source); err != nil || source.Revision != 2 || len(source.Homeworks) != 0 {
		t.Fatalf("source checkpoint incorrect: %s (%v)", me.Body.String(), err)
	}
	dst := newSecurityFixture(t)
	expectStatus(t, restoreBackup(t, dst, dst.token("super"), archive), 200)
	check := "/api/v1/device/migration-check"
	newDevice := map[string]any{"min_sequence": 0, "pending_ids": []string{}, "min_homework_revision": source.Revision}
	expectStatus(t, dst.request(t, "POST", check, "token-a", newDevice, nil), 409)
	newDevice["min_homework_revision"] = -1
	expectStatus(t, dst.request(t, "POST", check, "token-a", newDevice, nil), 400)
	newDevice["min_homework_revision"] = maxHomeworkRevision + 1
	expectStatus(t, dst.request(t, "POST", check, "token-a", newDevice, nil), 400)
	newDevice["min_homework_revision"] = 1
	w := dst.request(t, "POST", check, "token-a", newDevice, nil)
	expectStatus(t, w, 200)
	var result struct {
		Revision  int64      `json:"homework_revision"`
		Homeworks []Homework `json:"homeworks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Revision != 1 || len(result.Homeworks) != 1 || result.Homeworks[0].ID != item.ID || result.Homeworks[0].Visible {
		t.Fatalf("migration omitted hidden homework: %s (%v)", w.Body.String(), err)
	}
	// Existing clients that do not know about homework remain compatible.
	expectStatus(t, dst.request(t, "POST", check, "token-a", migrationPayload(0), nil), 200)
}
