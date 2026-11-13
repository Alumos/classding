package main

import (
	"strings"
	"sync"
	"testing"
)

type callTestWindow struct {
	mu     sync.Mutex
	latest studentCallView
	closed bool
}

func (w *callTestWindow) update(v studentCallView) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.latest = v
	return nil
}
func (w *callTestWindow) close() { w.mu.Lock(); defer w.mu.Unlock(); w.closed = true }
func callTestApp(t *testing.T) (*App, *[]string) {
	t.Helper()
	ready := make(chan struct{})
	close(ready)
	ids := []string{}
	a := &App{initDone: ready}
	a.studentCall.window = &callTestWindow{}
	a.studentCall.emit = func(id string, _ int) { ids = append(ids, id) }
	t.Cleanup(a.closeStudentCall)
	return a, &ids
}
func TestStudentCallGatesAndCompletion(t *testing.T) {
	a, ids := callTestApp(t)
	a.beginStudentCall(0)
	if len(*ids) != 0 || !strings.Contains(a.studentCall.view.Message, "未绑定") {
		t.Fatal("unbound call accepted")
	}
	_ = a.UpdateStudentCallStatus(true, false, "同名班", 1)
	a.beginStudentCall(1)
	if len(*ids) != 0 || !strings.Contains(a.studentCall.view.Message, "连接中断") {
		t.Fatal("offline call accepted")
	}
	_ = a.UpdateStudentCallStatus(true, true, "同名班", 1)
	a.beginStudentCall(0)
	if len(*ids) != 0 {
		t.Fatal("stale confirmation accepted")
	}
	a.beginStudentCall(1)
	a.beginStudentCall(1)
	if len(*ids) != 1 || len((*ids)[0]) != 32 {
		t.Fatal("missing duplicate guard or invalid id")
	}
	id := (*ids)[0]
	a.CompleteStudentCall("stale", true, "wrong")
	if !a.studentCall.view.Sending {
		t.Fatal("stale callback applied")
	}
	a.CompleteStudentCall(id, false, "失败可见")
	if a.studentCall.view.Sending || a.studentCall.view.Message != "失败可见" {
		t.Fatal("failure not rendered")
	}
	a.beginStudentCall(1)
	if len(*ids) != 2 || (*ids)[1] == id {
		t.Fatal("native attempt correlation id not renewed")
	}
	a.CompleteStudentCall(id, true, "late")
	if !a.studentCall.view.Sending {
		t.Fatal("previous attempt completed new retry")
	}
	a.CompleteStudentCall((*ids)[1], true, "成功可见")
	if a.studentCall.view.Sending || a.studentCall.view.Message != "成功可见" {
		t.Fatal("success not rendered")
	}
}
func TestStudentCallPendingSnapshot(t *testing.T) {
	previous := studentCallView{Bound: true, Binding: 1, Revision: 3}
	for _, next := range []studentCallView{
		{Bound: true, Binding: 1, Revision: 4, Suspended: true},
		{Bound: false, Binding: 1, Revision: 4},
		{Bound: true, Binding: 2, Revision: 4},
		{Bound: true, Binding: 1, Revision: 4, Sending: true},
		{Bound: true, Binding: 1, Revision: 4, Message: "done"},
	} {
		if !studentCallAcknowledged(previous, next, 3) {
			t.Fatalf("native pending spinner not cleared by %#v", next)
		}
	}
	if studentCallAcknowledged(previous, previous, 3) {
		t.Fatal("stale pre-send snapshot cleared native pending guard")
	}
}

func TestStudentCallRebindSuspendClose(t *testing.T) {
	a, ids := callTestApp(t)
	_ = a.UpdateStudentCallStatus(true, true, "同名班", 1)
	a.suspendStudentCall(true)
	a.beginStudentCall(1)
	if len(*ids) != 0 {
		t.Fatal("presentation accepted call")
	}
	a.suspendStudentCall(false)
	a.beginStudentCall(1)
	id := (*ids)[0]
	_ = a.UpdateStudentCallStatus(true, true, "同名班", 2)
	a.CompleteStudentCall(id, true, "old")
	if a.studentCall.view.Sending || a.studentCall.view.Message != "" {
		t.Fatal("same-name rebind reused result")
	}
	a.closeStudentCall()
	a.beginStudentCall(2)
	if len(*ids) != 1 {
		t.Fatal("closed manager emitted")
	}
	if a.UpdateStudentCallStatus(true, true, "x", 3) == nil {
		t.Fatal("closed manager accepted state")
	}
}
func TestStudentCallPhysicalGeometry(t *testing.T) {
	x, y := studentCallClamp(-2200, 1200, 100, 80, -1920, -200, 0, 1000)
	if x != -1920 || y != 920 {
		t.Fatalf("negative monitor clamp: %d,%d", x, y)
	}
	x, y = studentCallClamp(2100, -100, 120, 80, 1920, 0, 3840, 1080)
	if x != 2100 || y != 0 {
		t.Fatalf("second workarea clamp: %d,%d", x, y)
	}
	if studentCallDragged(3, 2, 96) || !studentCallDragged(6, 0, 96) || studentCallDragged(8, 0, 192) || !studentCallDragged(12, 0, 192) {
		t.Fatal("DPI drag threshold")
	}
}

func TestStudentCallButtonsAlignToPanelMargins(t *testing.T) {
	for _, dpi := range []int{96, 144, 192} {
		px := func(v int) int { return v * dpi / 96 }
		width, height, margin, gap := px(studentCallPanelWidth), px(studentCallPanelHeight), px(studentCallMargin), px(12)
		b := studentCallButtons(width, height, margin, gap, px(studentCallButtonHeight))
		if b[0][0] != margin || b[1][0]+b[1][2] != width-margin {
			t.Fatalf("dpi %d: buttons not inside equal margins: %v", dpi, b)
		}
		if b[0][2] != b[1][2] || b[0][1] != b[1][1] || b[0][1]+b[0][3] != height-margin {
			t.Fatalf("dpi %d: buttons not aligned: %v", dpi, b)
		}
		if gotGap := b[1][0] - (b[0][0] + b[0][2]); gotGap < gap || gotGap > gap+1 {
			t.Fatalf("dpi %d: gap %d, want %d", dpi, gotGap, gap)
		}
	}
	if studentCallBallFont < 24 || studentCallBallWidth < 100 {
		t.Fatal("ball text must stay large enough to read from a desk")
	}
}
