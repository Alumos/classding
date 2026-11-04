package app

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func rtLogin(t *testing.T, base, username, password string) string {
	t.Helper()
	var auth struct {
		Token string `json:"access_token"`
	}
	rtRequest(t, base, "POST", "/auth/login", "", map[string]string{"username": username, "password": password}, 200, &auth)
	return auth.Token
}

func TestDeleteMessageRecallsPendingDeliveryForTeacherAndAdmin(t *testing.T) {
	_, s, token := rtSetup(t)
	var classes []Class
	rtRequest(t, s.URL, "GET", "/classes", token, nil, 200, &classes)
	device := rtBind(t, s.URL, token, classes[0].ID)
	conn, _ := rtConnect(t, s.URL, device, nil)

	var first Message
	rtRequest(t, s.URL, "POST", "/messages", token, rtInput(classes[0].ID), 200, &first)
	rtRead(t, conn, "notify")
	var stranger struct {
		Token string `json:"access_token"`
	}
	rtRequest(t, s.URL, "POST", "/auth/register", "", map[string]string{"username": "stranger", "password": "Stranger2026!", "display_name": "外校老师"}, 200, &stranger)
	rtRequest(t, s.URL, "DELETE", "/messages/"+first.ID, stranger.Token, nil, 403, nil)
	rtRequest(t, s.URL, "DELETE", "/messages/"+first.ID, token, nil, 200, nil)
	if recall := rtRead(t, conn, "recall"); recall.D.(map[string]any)["message_id"] != first.ID {
		t.Fatalf("deleted message was not recalled: %+v", recall)
	}
	rtRequest(t, s.URL, "GET", "/messages/"+first.ID, token, nil, 404, nil)

	time.Sleep(5100 * time.Millisecond) // The class send cooldown is five seconds.
	var second Message
	rtRequest(t, s.URL, "POST", "/messages", token, rtInput(classes[0].ID), 200, &second)
	rtRead(t, conn, "notify")
	admin := rtLogin(t, s.URL, "admin", "AdminDing2026!")
	rtRequest(t, s.URL, "DELETE", "/admin/messages/"+second.ID, token, nil, 403, nil)
	rtRequest(t, s.URL, "DELETE", "/admin/messages/"+second.ID, admin, nil, 200, nil)
	rtRead(t, conn, "recall")
	rtRequest(t, s.URL, "GET", "/messages/"+second.ID, token, nil, 404, nil)
	rtRequest(t, s.URL, "DELETE", "/admin/messages/"+second.ID, admin, nil, 404, nil)
}

func TestUserInterfaceScaleIsRemembered(t *testing.T) {
	_, s, token := rtSetup(t)
	var user User
	rtRequest(t, s.URL, "PATCH", "/me", token, map[string]int{"ui_scale": 120}, 200, &user)
	if user.UIScale != 120 || user.DisplayName == "" {
		t.Fatalf("scale update changed the wrong fields: %+v", user)
	}
	rtRequest(t, s.URL, "PATCH", "/me", token, map[string]string{"display_name": "林老师"}, 200, &user)
	rtRequest(t, s.URL, "GET", "/me", token, nil, 200, &user)
	if user.UIScale != 120 {
		t.Fatalf("scale was not remembered: %+v", user)
	}
	rtRequest(t, s.URL, "PATCH", "/me", token, map[string]int{"ui_scale": 200}, 400, nil)
	rtRequest(t, s.URL, "PATCH", "/me", token, map[string]any{}, 400, nil)
}

func TestDeviceHelloRecordsNetworkAndServer(t *testing.T) {
	_, s, token := rtSetup(t)
	var classes []Class
	rtRequest(t, s.URL, "GET", "/classes", token, nil, 200, &classes)
	device := rtBind(t, s.URL, token, classes[0].ID)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(s.URL, "http")+"/ws/device", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	hello := map[string]any{"token": device, "last_seq": 0, "client_version": "0.2.0", "os": "windows", "lan_ips": []string{"192.168.1.20", "not-an-ip", "fe80::1"}, "server_url": "https://school.example/", "applied_revision": "rev_1"}
	if err = wsjson.Write(ctx, conn, map[string]any{"t": "hello", "d": hello}); err != nil {
		t.Fatal(err)
	}
	rtRead(t, conn, "welcome")
	admin := rtLogin(t, s.URL, "admin", "AdminDing2026!")
	var devices []Device
	rtRequest(t, s.URL, "GET", "/admin/devices", admin, nil, 200, &devices)
	if len(devices) != 1 {
		t.Fatalf("devices = %+v", devices)
	}
	got := devices[0]
	if !reflect.DeepEqual(got.LANIPs, []string{"192.168.1.20"}) || got.ServerURL != "https://school.example" || got.AppliedRevision != "rev_1" || got.PublicIP != "127.0.0.1" || got.ClientVersion != "0.2.0" {
		t.Fatalf("device network details were not recorded: %+v", got)
	}
}

func TestTextNoticesMayBeSilentButRecordingsPlayAtLeastOnce(t *testing.T) {
	f := newSecurityFixture(t)
	silent := securityText("class_a")
	silent["repeat"] = 0
	expectStatus(t, f.request(t, "POST", "/api/v1/messages", f.token("owner"), silent, nil), 200)
	for _, repeat := range []int{-1, 4} {
		body := securityText("class_a")
		body["repeat"] = repeat
		expectStatus(t, f.request(t, "POST", "/api/v1/messages", f.token("owner"), body, nil), 400)
	}
	f.app.state.Audio["clip"] = Audio{ID: "clip", UserID: "owner_a", MIME: "audio/webm", CreatedAt: now()}
	voice := map[string]any{"class_ids": []string{"class_a"}, "kind": "voice", "style": "notice", "display_sec": 60, "repeat": 0, "play_mode": "voice", "audio_id": "clip"}
	expectStatus(t, f.request(t, "POST", "/api/v1/messages", f.token("owner"), voice, nil), 400)
}
