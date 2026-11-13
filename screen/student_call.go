package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// No credentials cross this bridge. The existing WebView service performs the
// authenticated request through its configured loopback proxy, even in the tray.
type studentCallView struct {
	Bound, Online, Suspended, Sending bool
	ClassName, Message                string
	Binding, Revision                 int
}
type studentCallWindow interface {
	update(studentCallView) error
	close()
}
type studentCallState struct {
	mu        sync.Mutex
	window    studentCallWindow
	initErr   error
	view      studentCallView
	requestID string
	binding   int
	timer     *time.Timer
	closed    bool
	emit      func(string, int)
}

func (a *App) startStudentCall() error {
	a.studentCall.mu.Lock()
	defer a.studentCall.mu.Unlock()
	a.studentCall.emit = func(id string, binding int) {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "student-call:send", id, binding)
		}
	}
	window, err := newStudentCallWindow(a.beginStudentCall)
	if err != nil {
		a.studentCall.initErr = err
		return err
	}
	a.studentCall.window = window
	return window.update(a.studentCall.view)
}

// UpdateStudentCallStatus receives non-secret binding/connectivity metadata.
func (a *App) UpdateStudentCallStatus(bound, online bool, className string, binding int) error {
	if err := a.ready(); err != nil {
		return err
	}
	s := &a.studentCall
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("学生呼叫已关闭")
	}
	if s.view.Bound != bound || s.view.ClassName != className || s.binding != binding {
		if s.timer != nil {
			s.timer.Stop()
			s.timer = nil
		}
		s.requestID = ""
		s.view.Sending = false
		s.view.Message = ""
	}
	s.binding, s.view.Binding = binding, binding
	s.view.Bound, s.view.Online, s.view.ClassName = bound, online, className
	return s.renderLocked()
}
func (s *studentCallState) renderLocked() error {
	s.view.Revision++
	if s.window != nil {
		return s.window.update(s.view)
	}
	return s.initErr
}
func (a *App) beginStudentCall(binding int) {
	s := &a.studentCall
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.view.Sending || s.view.Suspended || binding != s.binding {
		return
	}
	if !s.view.Bound {
		s.view.Message = "设备未绑定，请打开课堂叮咚重新绑定"
		_ = s.renderLocked()
		return
	}
	if !s.view.Online {
		s.view.Message = "连接中断，呼叫未发送。请恢复网络后重试"
		_ = s.renderLocked()
		return
	}
	if s.requestID == "" {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			s.view.Message = "无法生成呼叫编号，请重试"
			_ = s.renderLocked()
			return
		}
		s.requestID = hex.EncodeToString(id[:])
	}
	id := s.requestID
	s.view.Sending, s.view.Message = true, "正在发送，请稍候…"
	_ = s.renderLocked()
	// This also covers a stalled/crashed WebView. JS retains the canonical request id.
	s.timer = time.AfterFunc(12*time.Second, func() {
		a.finishStudentCall(id, false, "发送结果未确认，请重试（不会重复创建同一呼叫）")
	})
	if s.emit != nil {
		s.emit(id, binding)
	}
}
func (a *App) CompleteStudentCall(id string, success bool, message string) {
	a.finishStudentCall(id, success, message)
}
func (a *App) finishStudentCall(id string, success bool, message string) {
	s := &a.studentCall
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || id == "" || id != s.requestID {
		return
	}
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.view.Sending = false
	s.view.Message = message
	// The JS service owns the canonical idempotency key across uncertain retries.
	// This id only correlates this native attempt, so stale callbacks cannot finish a retry.
	s.requestID = ""
	_ = s.renderLocked()
}
func (a *App) suspendStudentCall(active bool) {
	s := &a.studentCall
	s.mu.Lock()
	defer s.mu.Unlock()
	s.view.Suspended = active
	_ = s.renderLocked()
}
func (a *App) closeStudentCall() {
	s := &a.studentCall
	s.mu.Lock()
	s.closed = true
	s.emit = nil
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	window := s.window
	s.window = nil
	s.mu.Unlock()
	// Never wait for the UI thread while holding the callback/state mutex.
	if window != nil {
		window.close()
	}
}

// A suspend/unbind can win the state mutex before a native confirmation.
// It must clear the native optimistic spinner even when begin exits without emitting.
func studentCallAcknowledged(previous, next studentCallView, sentRevision int) bool {
	return next.Suspended || !next.Bound || next.Binding != previous.Binding ||
		(next.Revision > sentRevision && (next.Sending || next.Message != ""))
}

// Sizes are in 96-DPI pixels. The ball is large enough to read from a desk and
// to hit on a touch board; the confirmation leaves room for two big buttons.
const (
	studentCallBallWidth, studentCallBallHeight   = 112, 92
	studentCallPanelWidth, studentCallPanelHeight = 480, 280
	studentCallBallFont, studentCallTitleFont     = 26, 28
	studentCallBodyFont, studentCallButtonFont    = 21, 22
	studentCallButtonHeight, studentCallMargin    = 54, 24
)

// The ball is a vivid orange so students find it at a glance; text is white.
var (
	studentCallBallFill = [3]uint8{249, 115, 22}
	studentCallBallEdge = [3]uint8{194, 65, 12}
	studentCallAccent   = [3]uint8{194, 65, 12}
)

// studentCallButtons lays out the cancel and confirm buttons side by side along
// the bottom of the panel, with equal outer margins and one gap between them.
func studentCallButtons(width, height, margin, gap, buttonHeight int) [2][4]int {
	buttonWidth := max(1, (width-2*margin-gap)/2)
	top := height - margin - buttonHeight
	return [2][4]int{{margin, top, buttonWidth, buttonHeight}, {width - margin - buttonWidth, top, buttonWidth, buttonHeight}}
}

// Geometry is in physical pixels, including negative monitor origins.
func studentCallClamp(x, y, width, height, left, top, right, bottom int) (int, int) {
	return max(left, min(x, right-width)), max(top, min(y, bottom-height))
}
func studentCallDragged(dx, dy, dpi int) bool {
	threshold := max(4, 6*dpi/96)
	return dx*dx+dy*dy >= threshold*threshold
}
