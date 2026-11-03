//go:build windows

package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var (
	hwGDI          = syscall.NewLazyDLL("gdi32.dll")
	hwBeginPaint   = user32.NewProc("BeginPaint")
	hwEndPaint     = user32.NewProc("EndPaint")
	hwFillRect     = user32.NewProc("FillRect")
	hwDrawText     = user32.NewProc("DrawTextW")
	hwInvalidate   = user32.NewProc("InvalidateRect")
	hwShowWindow   = user32.NewProc("ShowWindow")
	hwSetTimer     = user32.NewProc("SetTimer")
	hwSetBkMode    = hwGDI.NewProc("SetBkMode")
	hwSetTextColor = hwGDI.NewProc("SetTextColor")
	hwCreateBrush  = hwGDI.NewProc("CreateSolidBrush")
	hwCreateFont   = hwGDI.NewProc("CreateFontW")
	hwSelectObject = hwGDI.NewProc("SelectObject")
	hwDeleteObject = hwGDI.NewProc("DeleteObject")
	hwWindows      sync.Map
	hwProc         = syscall.NewCallback(homeworkWndProc)
)

const hwUpdate = wmApp + 51

type hwPaint struct {
	DC                 uintptr
	Erase              int32
	Rect               displayRect
	Restore, IncUpdate int32
	Reserved           [32]byte
}
type homeworkNativeWindow struct {
	owner     *nativeHomeworkWindows
	card      HomeworkCard
	geometry  homeworkGeometry
	hwnd      uintptr
	offset    int
	maxOffset int
}
type nativeHomeworkWindows struct {
	mu        sync.Mutex
	display   string
	items     []HomeworkCard
	suspended bool
	hwnd      uintptr
	cards     map[string]*homeworkNativeWindow // Owned by the message-loop thread.
	onAction  func(string, string)
	done      chan struct{}
	closed    bool
}

func newHomeworkWindows(onAction func(string, string)) (homeworkWindowManager, error) {
	m := &nativeHomeworkWindows{cards: map[string]*homeworkNativeWindow{}, onAction: onAction, done: make(chan struct{})}
	ready := make(chan error, 1)
	go m.run(ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return m, nil
}
func (m *nativeHomeworkWindows) run(ready chan error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(m.done)
	className := syscall.StringToUTF16Ptr("ClassDingHomeworkWindow")
	instance, _, _ := getModuleHandle.Call(0)
	class := trayWindowClass{size: uint32(unsafe.Sizeof(trayWindowClass{})), windowProc: hwProc, instance: instance, className: uintptr(unsafe.Pointer(className))}
	if atom, _, err := registerClassEx.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		ready <- fmt.Errorf("无法注册桌面作业窗口：%w", err)
		return
	}
	defer unregisterClass.Call(uintptr(unsafe.Pointer(className)), instance)
	// The invisible controller and all cards share one native UI thread, with no
	// extra WebViews or secondary application processes.
	hwnd, _, err := createWindowEx.Call(0, uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, 0, 0, instance, 0)
	if hwnd == 0 {
		ready <- fmt.Errorf("无法创建桌面作业窗口：%w", err)
		return
	}
	m.hwnd = hwnd
	hwWindows.Store(hwnd, m)
	hwSetTimer.Call(hwnd, 1, 2000, 0)
	ready <- nil
	var message trayMessageData
	for {
		result, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if result == 0 || int32(result) == -1 {
			break
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&message)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&message)))
	}
	for _, card := range m.cards {
		destroyWindow.Call(card.hwnd)
		hwWindows.Delete(card.hwnd)
	}
	hwWindows.Delete(hwnd)
}
func (m *nativeHomeworkWindows) update(display string, items []HomeworkCard, suspended bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fmt.Errorf("桌面作业窗口已关闭")
	}
	m.display = display
	m.items = append([]HomeworkCard{}, items...)
	m.suspended = suspended
	if ok, _, err := postMessage.Call(m.hwnd, hwUpdate, 0, 0); ok == 0 {
		return fmt.Errorf("无法更新桌面作业窗口：%w", err)
	}
	return nil
}
func (m *nativeHomeworkWindows) close() {
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
func (m *nativeHomeworkWindows) layout() {
	m.mu.Lock()
	display := m.display
	items := append([]HomeworkCard{}, m.items...)
	suspended := m.suspended
	m.mu.Unlock()
	displays, err := listDisplays()
	if err != nil || len(displays) == 0 {
		return
	}
	selected := displays[0]
	if display == "" {
		name := syscall.StringToUTF16Ptr("ClassDingMainWindow")
		hwnd, _, _ := findWindow.Call(uintptr(unsafe.Pointer(name)), 0)
		var pid uint32
		getWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if hwnd != 0 && int(pid) == os.Getpid() {
			monitor, _, _ := user32.NewProc("MonitorFromWindow").Call(hwnd, 2)
			info := monitorInfo{size: uint32(unsafe.Sizeof(monitorInfo{}))}
			if ok, _, _ := getMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); ok != 0 {
				display = syscall.UTF16ToString(info.device[:])
			}
		}
	}
	for _, d := range displays {
		if d.Primary {
			selected = d
		}
	}
	for _, d := range displays {
		if d.ID == display {
			selected = d
			break
		}
	}
	work := selected.work
	if work.right <= work.left || work.bottom <= work.top {
		work = selected.rect
	}
	visible := []HomeworkCard{}
	if !suspended {
		for _, item := range items {
			if item.Visible {
				visible = append(visible, item)
			}
		}
	}
	geometry := homeworkLayout(int(work.left), int(work.top), int(work.right-work.left), int(work.bottom-work.top), len(visible))
	keep := map[string]bool{}
	instance, _, _ := getModuleHandle.Call(0)
	className := syscall.StringToUTF16Ptr("ClassDingHomeworkWindow")
	for i, item := range visible {
		if i >= len(geometry) {
			break
		}
		keep[item.ID] = true
		card, ok := m.cards[item.ID]
		created := !ok
		wasTop := false
		if ok {
			wasTop = card.card.AlwaysOnTop
		}
		if !ok {
			// TOOLWINDOW keeps the taskbar clean; NOACTIVATE never takes focus
			// from the lesson currently being taught.
			hwnd, _, _ := createWindowEx.Call(0x00000080|0x08000000, uintptr(unsafe.Pointer(className)), 0, 0x80000000|0x00800000, 0, 0, 0, 0, m.hwnd, 0, instance, 0)
			if hwnd == 0 {
				continue
			}
			card = &homeworkNativeWindow{owner: m, hwnd: hwnd}
			m.cards[item.ID] = card
			hwWindows.Store(hwnd, card)
		}
		if card.card.UpdatedAt != item.UpdatedAt {
			card.offset = 0
		}
		card.card = item
		card.geometry = geometry[i]
		z := ^uintptr(1) // HWND_NOTOPMOST
		if item.AlwaysOnTop {
			z = ^uintptr(0)
		} // HWND_TOPMOST
		g := card.geometry
		flags := uintptr(0x0010 | 0x0040)
		if !created && wasTop == item.AlwaysOnTop {
			flags |= 0x0004
		}
		setWindowPos.Call(card.hwnd, z, uintptr(g.X), uintptr(g.Y), uintptr(g.Width), uintptr(g.Height), flags)
		hwInvalidate.Call(card.hwnd, 0, 0)
	}
	for id, card := range m.cards {
		if !keep[id] {
			destroyWindow.Call(card.hwnd)
			hwWindows.Delete(card.hwnd)
			delete(m.cards, id)
		}
	}
}
func homeworkWndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	if value, ok := hwWindows.Load(hwnd); ok {
		switch target := value.(type) {
		case *nativeHomeworkWindows:
			switch message {
			case hwUpdate, 0x0113, 0x007e, 0x001a:
				target.layout()
				return 0 // timer / display / work area changes
			case wmClose:
				destroyWindow.Call(hwnd)
				return 0
			case wmDestroy:
				postQuitMessage.Call(0)
				return 0
			}
		case *homeworkNativeWindow:
			switch message {
			case 0x0021:
				return 3 // MA_NOACTIVATE
			case 0x0014:
				return 1 // fully painted below
			case 0x000f:
				target.paint()
				return 0
			case 0x020a:
				delta := int(int16(wParam >> 16))
				target.offset -= delta / 120 * target.geometry.Font * 3
				if target.offset < 0 {
					target.offset = 0
				}
				if target.offset > target.maxOffset {
					target.offset = target.maxOffset
				}
				hwInvalidate.Call(hwnd, 0, 0)
				return 0
			case wmLeftButtonUp:
				x, y := int(int16(lParam)), int(int16(lParam>>16))
				g := target.geometry
				if y >= g.Height-50*g.Scale/100 {
					action := "hide"
					if x < g.Width/2 {
						action = "pin"
						if target.card.AlwaysOnTop {
							action = "unpin"
						}
					}
					go target.owner.onAction(target.card.ID, action)
				} else if y >= 88*g.Scale/100 && target.maxOffset > 0 {
					// A classroom touch display must not require a mouse wheel.
					if target.offset >= target.maxOffset {
						target.offset = 0
					} else {
						target.offset += g.Height - 166*g.Scale/100
						if target.offset > target.maxOffset {
							target.offset = target.maxOffset
						}
					}
					hwInvalidate.Call(hwnd, 0, 0)
				}
				return 0
			case wmClose:
				go target.owner.onAction(target.card.ID, "hide")
				return 0
			}
		}
	}
	result, _, _ := defWindowProc.Call(hwnd, message, wParam, lParam)
	return result
}
func hwColor(r, g, b uint32) uintptr { return uintptr(r | g<<8 | b<<16) }
func hwText(dc uintptr, text string, rect *displayRect, flags uintptr) {
	text = strings.ReplaceAll(text, "\x00", "")
	ptr := syscall.StringToUTF16Ptr(text)
	hwDrawText.Call(dc, uintptr(unsafe.Pointer(ptr)), ^uintptr(0), uintptr(unsafe.Pointer(rect)), flags|0x0800) // DT_NOPREFIX
}
func hwFont(size, weight int) uintptr {
	face := syscall.StringToUTF16Ptr("Microsoft YaHei UI")
	font, _, _ := hwCreateFont.Call(uintptr(-size), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
	return font
}
func (c *homeworkNativeWindow) paint() {
	var paint hwPaint
	dc, _, _ := hwBeginPaint.Call(c.hwnd, uintptr(unsafe.Pointer(&paint)))
	if dc == 0 {
		return
	}
	defer hwEndPaint.Call(c.hwnd, uintptr(unsafe.Pointer(&paint)))
	g := c.geometry
	s := g.Scale
	pad := 18 * s / 100
	full := displayRect{0, 0, int32(g.Width), int32(g.Height)}
	brush, _, _ := hwCreateBrush.Call(hwColor(247, 250, 245))
	hwFillRect.Call(dc, uintptr(unsafe.Pointer(&full)), brush)
	hwDeleteObject.Call(brush)
	hwSetBkMode.Call(dc, 1)
	titleFont := hwFont(g.Font+4*s/100, 650)
	font := hwFont(g.Font, 500)
	smallSize := 20 * s / 100
	if s >= 160 && smallSize < 40 {
		smallSize = 40
	}
	smallFont := hwFont(smallSize, 500)
	defer hwDeleteObject.Call(titleFont)
	defer hwDeleteObject.Call(font)
	defer hwDeleteObject.Call(smallFont)
	old, _, _ := hwSelectObject.Call(dc, titleFont)
	defer hwSelectObject.Call(dc, old)
	hwSetTextColor.Call(dc, hwColor(25, 80, 58))
	header := displayRect{int32(pad), int32(12 * s / 100), int32(g.Width - pad), int32(52 * s / 100)}
	hwText(dc, c.card.Subject+"作业", &header, 0x0020|0x8000) // SINGLELINE / END_ELLIPSIS
	hwSelectObject.Call(dc, smallFont)
	hwSetTextColor.Call(dc, hwColor(67, 83, 71))
	author := displayRect{int32(pad), int32(54 * s / 100), int32(g.Width - pad), int32(82 * s / 100)}
	hwText(dc, c.card.TeacherName, &author, 0x0020|0x8000)
	hwSelectObject.Call(dc, font)
	hwSetTextColor.Call(dc, hwColor(27, 43, 32))
	bodyTop, bodyBottom := 88*s/100, g.Height-78*s/100
	measure := displayRect{int32(pad), 0, int32(g.Width - pad), 0}
	hwText(dc, c.card.Text, &measure, 0x0010|0x0400) // WORDBREAK / CALCRECT
	c.maxOffset = int(measure.bottom) - (bodyBottom - bodyTop)
	if c.maxOffset < 0 {
		c.maxOffset = 0
	}
	if c.offset > c.maxOffset {
		c.offset = c.maxOffset
	}
	// DrawText clips to the body rectangle. Scroll offsets are implemented by
	// a GDI clip region, so long assignments never overwrite the controls.
	saveDC := hwGDI.NewProc("SaveDC")
	restoreDC := hwGDI.NewProc("RestoreDC")
	clip := hwGDI.NewProc("IntersectClipRect")
	saved, _, _ := saveDC.Call(dc)
	clip.Call(dc, uintptr(pad), uintptr(bodyTop), uintptr(g.Width-pad), uintptr(bodyBottom))
	body := displayRect{int32(pad), int32(bodyTop - c.offset), int32(g.Width - pad), int32(bodyTop-c.offset) + measure.bottom}
	hwText(dc, c.card.Text, &body, 0x0010)
	restoreDC.Call(dc, saved)
	hwSelectObject.Call(dc, smallFont)
	hwSetTextColor.Call(dc, hwColor(67, 83, 71))
	if c.maxOffset > 0 {
		hint := displayRect{int32(pad), int32(bodyBottom + 4*s/100), int32(g.Width - pad), int32(g.Height - 49*s/100)}
		hwText(dc, "点击正文翻页，或滚动查看", &hint, 0x0020|0x8000)
	}
	buttonTop := g.Height - 48*s/100
	buttonBg := displayRect{0, int32(buttonTop), int32(g.Width), int32(g.Height)}
	brush, _, _ = hwCreateBrush.Call(hwColor(222, 234, 221))
	hwFillRect.Call(dc, uintptr(unsafe.Pointer(&buttonBg)), brush)
	hwDeleteObject.Call(brush)
	left := displayRect{int32(pad), int32(buttonTop), int32(g.Width / 2), int32(g.Height)}
	right := displayRect{int32(g.Width / 2), int32(buttonTop), int32(g.Width - pad), int32(g.Height)}
	hwSetTextColor.Call(dc, hwColor(25, 80, 58))
	pin := "保持在最上层"
	if c.card.AlwaysOnTop {
		pin = "取消置顶"
	}
	hwText(dc, pin, &left, 0x0020|0x0004)
	hwText(dc, "隐藏作业", &right, 0x0020|0x0004|0x0002)
}
