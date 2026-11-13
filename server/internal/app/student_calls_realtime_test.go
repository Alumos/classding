package app

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestStudentCallsRetiredPeerSessions(t *testing.T) {
	for _, mode := range []string{"expired", "version", "epoch", "subject"} {
		t.Run(mode, func(t *testing.T) {
			f := newSecurityFixture(t)
			good := callPeer(t, f.app, "owner_a")
			bad := callPeer(t, f.app, "owner_a")
			switch mode {
			case "expired":
				bad.teacherSession.Expires = time.Now().Unix()
			case "version":
				bad.teacherSession.Version++
			case "epoch":
				bad.teacherSession.Epoch = "retired"
			case "subject":
				bad.teacherSession.Subject = "owner_b"
			}
			item := postStudentCall(t, f, "token-a", "retired")
			takeCallFrame(t, good, item, "created")
			expectStatus(t, f.request(t, "POST", studentCallsPath+"/"+item.ID+"/read", f.token("collab"), nil, nil), 200)
			takeCallFrame(t, good, item, "read")
			if len(bad.queue) != 0 {
				t.Fatal("retired session received a call event")
			}
		})
	}
}

func connectCallTeacher(t *testing.T, base, token string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(base, "http")+"/ws/teacher", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	if err := wsjson.Write(ctx, conn, map[string]any{"t": "hello", "d": map[string]string{"token": token}}); err != nil {
		t.Fatal(err)
	}
	rtRead(t, conn, "welcome")
	return conn
}

func TestStudentCallsRealWebsocketAndReconnect(t *testing.T) {
	f := newSecurityFixture(t)
	commitState(f.app)
	server := httptest.NewServer(f.app.Handler())
	t.Cleanup(server.Close)
	owner := connectCallTeacher(t, server.URL, f.token("owner"))
	collab := connectCallTeacher(t, server.URL, f.token("collab"))
	item := postStudentCall(t, f, "token-a", "online")
	for _, conn := range []*websocket.Conn{owner, collab} {
		frame := rtRead(t, conn, "student_call")
		data := frame.D.(map[string]any)
		if data["id"] != item.ID || data["class_id"] != "class_a" || data["kind"] != "created" || len(data) != 3 {
			t.Fatalf("bad websocket payload: %+v", frame)
		}
	}
	collab.CloseNow()
	expectStatus(t, f.request(t, "POST", studentCallsPath+"/"+item.ID+"/read", f.token("owner"), nil, nil), 200)
	frame := rtRead(t, owner, "student_call")
	if frame.D.(map[string]any)["kind"] != "read" {
		t.Fatal("missing read event")
	}
	connectCallTeacher(t, server.URL, f.token("collab"))
	var got studentCallList
	rtRequest(t, server.URL, "GET", "/student-calls", f.token("collab"), nil, 200, &got)
	if got.UnreadCount != 0 || len(got.Items) != 1 || got.Items[0].ReadBy != "owner_a" {
		t.Fatalf("offline/reconnected shared read lost: %+v", got)
	}
}
