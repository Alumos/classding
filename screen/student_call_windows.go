//go:build windows

package main

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

const scUpdate = wmApp + 61

var scWindows sync.Map
var scProc = syscall.NewCallback(studentCallWndProc)

type nativeStudentCallWindow struct {
	mu        sync.Mutex
	pending   studentCallView
	closed    bool
	hwnd      uintptr
	done      chan struct{}
	onConfirm func(int)
	// All fields below are owned by the native UI thread.
	view                                studentCallView
	expanded, down, dragged, positioned bool
	origin, cursor                      trayPoint
	x, y, width, height, dpi            int
	confirm, cancel                     uintptr
	awaiting                            bool
	sentRevision                        int
	buttonFont                          uintptr
	fontDPI                             int
}

func newStudentCallWindow(onConfirm func(int)) (studentCallWindow, error) {
	m := &nativeStudentCallWindow{done: make(chan struct{}), onConfirm: onConfirm, dpi: 96}
	ready := make(chan error, 1)
	go m.run(ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return m, nil
}
func (m *nativeStudentCallWindow) run(ready chan error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(m.done)
	name := syscall.StringToUTF16Ptr("ClassDingStudentCall")
	title := syscall.StringToUTF16Ptr("呼叫老师 · 课堂叮咚")
	instance, _, _ := getModuleHandle.Call(0)
	arrow, _, _ := hwLoadCursor.Call(0, 32512)
	class := trayWindowClass{size: uint32(unsafe.Sizeof(trayWindowClass{})), windowProc: scProc, instance: instance, cursor: arrow, className: uintptr(unsafe.Pointer(name))}
	if atom, _, err := registerClassEx.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		ready <- fmt.Errorf("无法注册呼叫窗口：%w", err)
		return
	}
	defer unregisterClass.Call(uintptr(unsafe.Pointer(name)), instance)
	hwnd, _, err := createWindowEx.Call(0x80|0x08000000, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(title)), 0x80000000, 0, 0, 80, 64, 0, 0, instance, 0) // TOOLWINDOW | NOACTIVATE / POPUP
	if hwnd == 0 {
		ready <- fmt.Errorf("无法创建呼叫窗口：%w", err)
		return
	}
	m.hwnd = hwnd
	scWindows.Store(hwnd, m)
	defer scWindows.Delete(hwnd)
	button := syscall.StringToUTF16Ptr("BUTTON")
	for i, label := range []string{"取消", "确认呼叫"} {
		text := syscall.StringToUTF16Ptr(label)
		child, _, _ := createWindowEx.Call(0, uintptr(unsafe.Pointer(button)), uintptr(unsafe.Pointer(text)), 0x40000000|0x10000, 0, 0, 0, 0, hwnd, uintptr(i+1), instance, 0) // CHILD | TABSTOP
		if child == 0 {
			destroyWindow.Call(hwnd)
			ready <- errors.New("无法创建呼叫确认按钮")
			return
		}
		if i == 0 {
			m.cancel = child
		} else {
			m.confirm = child
		}
	}
	defer destroyWindow.Call(hwnd)
	defer func() {
		if m.buttonFont != 0 {
			hwDeleteObject.Call(m.buttonFont)
		}
	}()
	hwSetTimer.Call(hwnd, 1, 2000, 0)
	ready <- nil
	var msg trayMessageData
	for {
		result, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) <= 0 {
			break
		}
		if msg.message == 0x0100 && m.expanded { // keyboard applies to native child buttons too
			switch msg.wParam {
			case 27:
				m.collapse()
				continue
			case 13:
				focus, _, _ := user32.NewProc("GetFocus").Call()
				if focus == m.confirm {
					m.send()
				} else {
					m.collapse()
				}
				continue
			case 9:
				focus, _, _ := user32.NewProc("GetFocus").Call()
				next := m.cancel
				if focus == m.cancel && !m.view.Sending {
					next = m.confirm
				}
				user32.NewProc("SetFocus").Call(next)
				continue
			}
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}
func (m *nativeStudentCallWindow) update(view studentCallView) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("呼叫窗口已关闭")
	}
	m.pending = view
	if ok, _, err := postMessage.Call(m.hwnd, scUpdate, 0, 0); ok == 0 {
		return err
	}
	return nil
}
func (m *nativeStudentCallWindow) close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	postMessage.Call(m.hwnd, wmClose, 0, 0)
	m.mu.Unlock()
	<-m.done
}
func (m *nativeStudentCallWindow) px(v int) int { return v * m.dpi / 96 }
func (m *nativeStudentCallWindow) place() {
	style := uintptr(0x80 | 0x08000000)
	if m.expanded {
		style = 0x80
	}
	user32.NewProc("SetWindowLongPtrW").Call(m.hwnd, ^uintptr(19), style)
	if dpiProc := user32.NewProc("GetDpiForWindow"); dpiProc.Find() == nil {
		if dpi, _, _ := dpiProc.Call(m.hwnd); dpi != 0 {
			m.dpi = int(dpi)
		}
	}
	m.width, m.height = m.px(studentCallBallWidth), m.px(studentCallBallHeight)
	if m.expanded {
		m.width, m.height = m.px(studentCallPanelWidth), m.px(studentCallPanelHeight)
	}
	monitor, _, _ := user32.NewProc("MonitorFromWindow").Call(m.hwnd, 2)
	info := monitorInfo{size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, _ := getMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return
	}
	w := info.work
	if !m.positioned {
		m.x, m.y = int(w.left)+m.px(20), int(w.top)+int(w.bottom-w.top)/2
		m.positioned = true
	}
	m.width = min(m.width, int(w.right-w.left))
	m.height = min(m.height, int(w.bottom-w.top))
	m.x, m.y = studentCallClamp(m.x, m.y, m.width, m.height, int(w.left), int(w.top), int(w.right), int(w.bottom))
	// The ball lives in the normal window layer: it sits on the desktop and
	// stays visible beside ordinary windows, but a PowerPoint slide show or any
	// maximised window covers it. NOZORDER keeps the periodic re-layout from
	// ever raising it; only a click on the ball brings the confirmation forward.
	setWindowPos.Call(m.hwnd, 0, uintptr(m.x), uintptr(m.y), uintptr(m.width), uintptr(m.height), 0x0004|0x0010) // NOZORDER / NOACTIVATE
	radius := m.height / 2
	if m.expanded {
		radius = m.px(16)
	}
	region, _, _ := hwRoundRegion.Call(0, 0, uintptr(m.width+1), uintptr(m.height+1), uintptr(radius*2), uintptr(radius*2))
	if region != 0 {
		if ok, _, _ := hwSetRegion.Call(m.hwnd, region, 1); ok == 0 {
			hwDeleteObject.Call(region)
		}
	}
	if m.fontDPI != m.dpi {
		if font := hwFont(m.px(studentCallButtonFont), 600); font != 0 {
			previous := m.buttonFont
			m.buttonFont, m.fontDPI = font, m.dpi
			for _, button := range []uintptr{m.cancel, m.confirm} {
				user32.NewProc("SendMessageW").Call(button, 0x0030, font, 1) // WM_SETFONT
			}
			if previous != 0 {
				hwDeleteObject.Call(previous)
			}
		}
	}
	layout := studentCallButtons(m.width, m.height, m.px(studentCallMargin), m.px(12), m.px(studentCallButtonHeight))
	for i, button := range []uintptr{m.cancel, m.confirm} {
		b := layout[i]
		setWindowPos.Call(button, 0, uintptr(b[0]), uintptr(b[1]), uintptr(b[2]), uintptr(b[3]), 0x0004|0x0010)
		show := uintptr(0)
		if m.expanded {
			show = 4
		}
		hwShowWindow.Call(button, show)
	}
	user32.NewProc("EnableWindow").Call(m.confirm, boolUint(!m.view.Sending && m.view.Bound && m.view.Online))
	if m.view.Suspended || !m.view.Bound {
		hwShowWindow.Call(m.hwnd, 0)
	} else {
		hwShowWindow.Call(m.hwnd, 4)
	}
	hwInvalidate.Call(m.hwnd, 0, 1)
}
func boolUint(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}
func (m *nativeStudentCallWindow) expand() {
	m.expanded = true
	// Only this explicit user action activates the confirmation. Idle ball never steals focus.
	user32.NewProc("SetWindowLongPtrW").Call(m.hwnd, ^uintptr(19), 0x80)
	m.place()
	setWindowPos.Call(m.hwnd, 0, 0, 0, 0, 0, 0x0001|0x0002) // HWND_TOP, keep size and position
	setForegroundWindow.Call(m.hwnd)
	user32.NewProc("SetFocus").Call(m.cancel) // Cancel is the safe default.
}
func (m *nativeStudentCallWindow) collapse() {
	m.expanded = false
	user32.NewProc("SetWindowLongPtrW").Call(m.hwnd, ^uintptr(19), 0x80|0x08000000)
	m.place()
}
func (m *nativeStudentCallWindow) send() {
	if !m.expanded || m.view.Sending || !m.view.Bound || !m.view.Online || m.view.Suspended {
		return
	}
	m.awaiting, m.sentRevision = true, m.view.Revision
	m.view.Sending = true
	m.view.Message = "正在发送，请稍候…"
	m.place()
	// This only queues a runtime event; it does not perform HTTP or wait on WebView.
	// Calling before returning keeps pending native snapshots ordered with send state.
	m.onConfirm(m.view.Binding)
}
func studentCallWndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	if value, ok := scWindows.Load(hwnd); ok {
		m := value.(*nativeStudentCallWindow)
		switch message {
		case scUpdate:
			m.mu.Lock()
			view := m.pending
			m.mu.Unlock()
			if m.view.Bound != view.Bound || m.view.Binding != view.Binding || view.Suspended {
				m.expanded = false
			}
			if studentCallAcknowledged(m.view, view, m.sentRevision) {
				m.awaiting = false
			}
			if m.awaiting {
				view.Sending = true
			}
			m.view = view
			m.place()
			return 0
		case 0x0113, 0x007e, 0x001a:
			if !m.down {
				m.place()
			}
			return 0 // timer, display/work-area changes
		case 0x02e0: // Keep physical drag position; resize and clamp for the new DPI.
			// No dereference of the callback uintptr (also safe under checkptr/vet).
			m.dpi = int(wParam & 0xffff)
			m.place()
			return 0
		case 0x0021:
			if !m.expanded {
				return 3
			} // MA_NOACTIVATE
		case 0x0014:
			return 1
		case 0x000f:
			m.paint()
			return 0
		case 0x0201:
			if !m.expanded {
				m.down, m.dragged = true, false
				getCursorPos.Call(uintptr(unsafe.Pointer(&m.cursor)))
				m.origin = trayPoint{int32(m.x), int32(m.y)}
				user32.NewProc("SetCapture").Call(hwnd)
			}
			return 0
		case 0x0200:
			if m.down {
				var point trayPoint
				getCursorPos.Call(uintptr(unsafe.Pointer(&point)))
				dx, dy := int(point.x-m.cursor.x), int(point.y-m.cursor.y)
				m.dragged = m.dragged || studentCallDragged(dx, dy, m.dpi)
				if m.dragged {
					m.x, m.y = int(m.origin.x)+dx, int(m.origin.y)+dy
					setWindowPos.Call(hwnd, 0, uintptr(m.x), uintptr(m.y), 0, 0, 0x0001|0x0004|0x0010)
				}
			}
			return 0
		case wmLeftButtonUp:
			if m.down {
				m.down = false
				user32.NewProc("ReleaseCapture").Call()
				if m.dragged {
					m.place()
				} else {
					m.expand()
				}
			}
			return 0
		case 0x0215:
			m.down = false
			return 0 // capture cancelled: never turn into a click
		case 0x0111:
			if wParam&0xffff == 1 {
				m.collapse()
			}
			if wParam&0xffff == 2 {
				m.send()
			}
			return 0
		case wmClose:
			m.mu.Lock()
			closed := m.closed
			m.mu.Unlock()
			if closed {
				destroyWindow.Call(hwnd)
			} else {
				m.collapse()
			}
			return 0
		case wmDestroy:
			postQuitMessage.Call(0)
			return 0
		}
	}
	result, _, _ := defWindowProc.Call(hwnd, message, wParam, lParam)
	return result
}
func (m *nativeStudentCallWindow) paint() {
	var paint hwPaint
	dc, _, _ := hwBeginPaint.Call(m.hwnd, uintptr(unsafe.Pointer(&paint)))
	if dc == 0 {
		return
	}
	defer hwEndPaint.Call(m.hwnd, uintptr(unsafe.Pointer(&paint)))
	full := displayRect{0, 0, int32(m.width), int32(m.height)}
	hwSetBkMode.Call(dc, 1)
	if !m.expanded {
		m.paintBall(dc, full)
		return
	}
	background := [3]uint8{255, 250, 245}
	brush, _, _ := hwCreateBrush.Call(hwRGB(background))
	hwFillRect.Call(dc, uintptr(unsafe.Pointer(&full)), brush)
	hwDeleteObject.Call(brush)
	// An orange band on top ties the panel to the ball that opened it.
	band := displayRect{0, 0, int32(m.width), int32(m.px(8))}
	hwFillRound(dc, band, 0, studentCallBallFill)

	margin := m.px(studentCallMargin)
	title := displayRect{int32(margin), int32(m.px(24)), int32(m.width - margin), int32(m.px(24 + 40))}
	titleFont := hwFont(m.px(studentCallTitleFont), 700)
	old, _, _ := hwSelectObject.Call(dc, titleFont)
	hwSetTextColor.Call(dc, hwRGB(studentCallAccent))
	hwText(dc, "呼叫老师", &title, 0x0020|0x0004) // SINGLELINE / VCENTER
	bodyFont := hwFont(m.px(studentCallBodyFont), 500)
	hwSelectObject.Call(dc, bodyFont)
	hwDeleteObject.Call(titleFont)
	defer func() { hwSelectObject.Call(dc, old); hwDeleteObject.Call(bodyFont) }()
	hwSetTextColor.Call(dc, hwRGB([3]uint8{41, 37, 36}))
	body := displayRect{title.left, title.bottom + int32(m.px(12)), title.right, int32(m.height - margin - m.px(studentCallButtonHeight) - m.px(12))}
	text := m.view.ClassName + "：确定呼叫本班所有关联老师吗？\n点“取消”不会发送。"
	if !m.view.Online {
		text = "连接中断，呼叫未发送。请恢复网络后重试。"
	}
	if m.view.Message != "" {
		text = m.view.Message
	}
	hwText(dc, text, &body, 0x0010) // WORDBREAK
}

// paintBall draws the collapsed ball: an orange pill with a darker rim and two
// centred lines of bold white text.
func (m *nativeStudentCallWindow) paintBall(dc uintptr, full displayRect) {
	radius := int(full.bottom) / 2
	hwFillRound(dc, full, radius, studentCallBallEdge)
	rim := int32(max(2, m.px(3)))
	inner := displayRect{full.left + rim, full.top + rim, full.right - rim, full.bottom - rim}
	hwFillRound(dc, inner, radius-int(rim), studentCallBallFill)
	font := hwFont(m.px(studentCallBallFont), 700)
	old, _, _ := hwSelectObject.Call(dc, font)
	defer func() { hwSelectObject.Call(dc, old); hwDeleteObject.Call(font) }()
	hwSetTextColor.Call(dc, hwRGB([3]uint8{255, 255, 255}))
	line := int32(m.px(studentCallBallFont) * 5 / 4)
	top := (full.bottom - 2*line) / 2
	for i, label := range []string{"呼叫", "老师"} {
		row := displayRect{full.left, top + int32(i)*line, full.right, top + int32(i+1)*line}
		hwText(dc, label, &row, 0x0001|0x0020|0x0004) // CENTER / SINGLELINE / VCENTER
	}
}
