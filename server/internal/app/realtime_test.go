package app

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func rtRequest(t *testing.T, base, method, path, token string, body any, status int, out any) {
	t.Helper()
	var data io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		data = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, base+"/api/v1"+path, data)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != status {
		t.Fatalf("%s %s want %d got %d %s", method, path, status, resp.StatusCode, raw)
	}
	if out != nil {
		if err = json.Unmarshal(raw, out); err != nil {
			t.Fatal(err)
		}
	}
}
func rtSetup(t *testing.T) (*App, *httptest.Server, string) {
	t.Helper()
	a, err := New(Config{DataDir: t.TempDir(), JWTSecret: strings.Repeat("k", 40), DevSeed: true})
	if err != nil {
		t.Fatal(err)
	}
	s := httptest.NewServer(a.Handler())
	t.Cleanup(func() { s.Close(); a.Close() })
	var auth struct {
		Token string `json:"access_token"`
	}
	rtRequest(t, s.URL, "POST", "/auth/login", "", map[string]string{"username": "teacher", "password": "ClassDing2026!"}, 200, &auth)
	return a, s, auth.Token
}
func rtBind(t *testing.T, base, token, cid string) string {
	t.Helper()
	var code struct{ Code string }
	rtRequest(t, base, "POST", "/classes/"+cid+"/bind-code", token, nil, 200, &code)
	if len(code.Code) != 6 {
		t.Fatal("bad binding code", code.Code)
	}
	var bound struct {
		Token string `json:"device_token"`
	}
	rtRequest(t, base, "POST", "/device/bind", "", map[string]string{"code": code.Code, "name": "测试大屏", "os": "windows", "client_version": "0.1.0"}, 200, &bound)
	rtRequest(t, base, "POST", "/device/bind", "", map[string]string{"code": code.Code}, 400, nil)
	return bound.Token
}
func rtRead(t *testing.T, conn *websocket.Conn, kind string) Frame {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		var frame Frame
		if err := wsjson.Read(ctx, conn, &frame); err != nil {
			t.Fatalf("read %s: %v", kind, err)
		}
		if frame.T == kind {
			return frame
		}
	}
}
func rtConnect(t *testing.T, base, token string, pending []string) (*websocket.Conn, map[string]any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(base, "http")+"/ws/device", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	if err = wsjson.Write(ctx, conn, map[string]any{"t": "hello", "d": map[string]any{"token": token, "last_seq": 9999, "pending_ids": pending}}); err != nil {
		t.Fatal(err)
	}
	return conn, rtRead(t, conn, "welcome").D.(map[string]any)
}
func rtInput(ids ...string) messageInput {
	return messageInput{ClassIDs: ids, Kind: "text", Text: "请带上作业到办公室", StudentName: "李沐阳", Style: "notice", DisplaySec: 15, Repeat: 1, Chime: true, PlayMode: "tts"}
}
func TestRealtimeTwoClassDeliveryAndReplay(t *testing.T) {
	_, s, token := rtSetup(t)
	var classes []Class
	rtRequest(t, s.URL, "GET", "/classes", token, nil, 200, &classes)
	d1 := rtBind(t, s.URL, token, classes[0].ID)
	d2 := rtBind(t, s.URL, token, classes[1].ID)
	c1, _ := rtConnect(t, s.URL, d1, nil)
	c2, _ := rtConnect(t, s.URL, d2, nil)
	var m Message
	start := time.Now()
	rtRequest(t, s.URL, "POST", "/messages", token, rtInput(classes[0].ID, classes[1].ID), 200, &m)
	f1, f2 := rtRead(t, c1, "notify"), rtRead(t, c2, "notify")
	if f1.ID != m.ID || f2.ID != m.ID || time.Since(start) > 2*time.Second {
		t.Fatal("two-screen delivery failed")
	}
	for _, conn := range []*websocket.Conn{c1, c2} {
		for _, status := range []string{"delivered", "playing", "played", "delivered"} {
			if err := wsjson.Write(context.Background(), conn, map[string]any{"t": "ack", "d": map[string]string{"message_id": m.ID, "status": status}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		rtRequest(t, s.URL, "GET", "/messages/"+m.ID, token, nil, 200, &m)
		if m.Targets[0].Status == "played" && m.Targets[1].Status == "played" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("receipt missing", m.Targets)
		}
		time.Sleep(10 * time.Millisecond)
	}
	c2.CloseNow()
	var queued Message
	rtRequest(t, s.URL, "POST", "/messages", token, rtInput(classes[1].ID), 200, &queued)
	reconnected, _ := rtConnect(t, s.URL, d2, nil)
	replay := rtRead(t, reconnected, "notify")
	if replay.ID != queued.ID || replay.Seq <= f2.Seq {
		t.Fatal("offline replay failed")
	}
	reconnected.CloseNow()
	rtRequest(t, s.URL, "POST", "/messages/"+queued.ID+"/recall", token, nil, 200, nil)
	_, welcome := rtConnect(t, s.URL, d2, []string{queued.ID})
	if values, ok := welcome["recalled_ids"].([]any); !ok || len(values) != 1 || values[0] != queued.ID {
		t.Fatal("offline recall failed", welcome)
	}
}
func TestCollaborationPermissionAndFilteredReceipts(t *testing.T) {
	_, s, owner := rtSetup(t)
	var classes []Class
	rtRequest(t, s.URL, "GET", "/classes", owner, nil, 200, &classes)
	var invite struct{ Code string }
	rtRequest(t, s.URL, "POST", "/classes/"+classes[0].ID+"/invites", owner, nil, 200, &invite)
	var auth struct {
		Token string `json:"access_token"`
	}
	rtRequest(t, s.URL, "POST", "/auth/register", "", map[string]string{"username": "colleague", "password": "TestPassword2026!", "display_name": "陈老师"}, 200, &auth)
	token := auth.Token
	rtRequest(t, s.URL, "POST", "/classes/join", token, map[string]string{"code": invite.Code}, 200, nil)
	rtRequest(t, s.URL, "POST", "/classes/join", token, map[string]string{"code": invite.Code}, 400, nil)
	rtRequest(t, s.URL, "POST", "/messages", token, rtInput(classes[0].ID), 200, nil)
	rtRequest(t, s.URL, "POST", "/messages", token, rtInput(classes[1].ID), 403, nil)
	rtRequest(t, s.URL, "POST", "/classes/"+classes[0].ID+"/bind-code", token, nil, 403, nil)
	rtRequest(t, s.URL, "PATCH", "/classes/"+classes[0].ID, token, map[string]int{"volume": 100}, 403, nil)
	var m Message
	rtRequest(t, s.URL, "POST", "/messages", owner, rtInput(classes[0].ID, classes[1].ID), 200, &m)
	rtRequest(t, s.URL, "POST", "/messages/"+m.ID+"/recall", token, nil, 403, nil)
	var view Message
	rtRequest(t, s.URL, "GET", "/messages/"+m.ID, token, nil, 200, &view)
	if len(view.Targets) != 1 || view.Targets[0].ClassID != classes[0].ID {
		t.Fatal("cross-class receipt leak")
	}
}
