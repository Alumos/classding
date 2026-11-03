//go:build windows

package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
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
	hwCompatibleDC = hwGDI.NewProc("CreateCompatibleDC")
	hwCompatibleBM = hwGDI.NewProc("CreateCompatibleBitmap")
	hwBitBlt       = hwGDI.NewProc("BitBlt")
	hwDeleteDC     = hwGDI.NewProc("DeleteDC")
	hwRoundRect    = hwGDI.NewProc("RoundRect")
	hwCreatePen    = hwGDI.NewProc("CreatePen")
	hwStockObject  = hwGDI.NewProc("GetStockObject")
	hwSaveDC       = hwGDI.NewProc("SaveDC")
	hwRestoreDC    = hwGDI.NewProc("RestoreDC")
	hwClipRect     = hwGDI.NewProc("IntersectClipRect")
	hwRoundRegion  = hwGDI.NewProc("CreateRoundRectRgn")
	hwSetRegion    = user32.NewProc("SetWindowRgn")
	hwLoadCursor   = user32.NewProc("LoadCursorW")
	hwSetCursor    = user32.NewProc("SetCursor")
	hwTrackMouse   = user32.NewProc("TrackMouseEvent")
	hwToClient     = user32.NewProc("ScreenToClient")
	hwDWM          = syscall.NewLazyDLL("dwmapi.dll")
	hwDWMAttribute = hwDWM.NewProc("DwmSetWindowAttribute")
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

type hwMouseTrack struct {
	size, flags uint32
	hwnd        uintptr
	hoverTime   uint32
}

type homeworkNativeWindow struct {
	owner      *nativeHomeworkWindows
	card       HomeworkCard
	geometry   homeworkGeometry
	hwnd       uintptr
	offset     int
	maxOffset  int
	hover      string // "pin", "hide" or "" for the control under the pointer
	tracking   bool
	dwmCorners bool
	regionSize [2]int
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
	arrow, _, _ := hwLoadCursor.Call(0, 32512)                                                                                                                                                        // IDC_ARROW
	class := trayWindowClass{size: uint32(unsafe.Sizeof(trayWindowClass{})), style: 0x00020000, windowProc: hwProc, instance: instance, cursor: arrow, className: uintptr(unsafe.Pointer(className))} // CS_DROPSHADOW
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
			hwnd, _, _ := createWindowEx.Call(0x00000080|0x08000000, uintptr(unsafe.Pointer(className)), 0, 0x80000000, 0, 0, 0, 0, m.hwnd, 0, instance, 0)
			if hwnd == 0 {
				continue
			}
			card = &homeworkNativeWindow{owner: m, hwnd: hwnd}
			card.applyRoundedFrame()
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
		card.fitRegion()
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
			case 0x0200: // WM_MOUSEMOVE
				if !target.tracking {
					track := hwMouseTrack{size: uint32(unsafe.Sizeof(hwMouseTrack{})), flags: 0x00000002, hwnd: hwnd} // TME_LEAVE
					hwTrackMouse.Call(uintptr(unsafe.Pointer(&track)))
					target.tracking = true
				}
				target.setHover(target.hitTest(int(int16(lParam)), int(int16(lParam>>16))))
				return 0
			case 0x02a3: // WM_MOUSELEAVE
				target.tracking = false
				target.setHover("")
				return 0
			case 0x0020: // WM_SETCURSOR
				if lParam&0xffff == 1 && target.hover != "" { // HTCLIENT over a control
					hand, _, _ := hwLoadCursor.Call(0, 32649) // IDC_HAND
					hwSetCursor.Call(hand)
					return 1
				}
			case wmLeftButtonUp:
				switch area := target.hitTest(int(int16(lParam)), int(int16(lParam>>16))); area {
				case "pin":
					action := "pin"
					if target.card.AlwaysOnTop {
						action = "unpin"
					}
					go target.owner.onAction(target.card.ID, action)
				case "hide":
					go target.owner.onAction(target.card.ID, "hide")
				case "body":
					// A classroom touch display must not require a mouse wheel.
					if target.offset >= target.maxOffset {
						target.offset = 0
					} else {
						target.offset += target.pageStep()
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
func hwRGB(c [3]uint8) uintptr       { return hwColor(uint32(c[0]), uint32(c[1]), uint32(c[2])) }

// hwMix blends a color toward white (amount 0..100 keeps that share of c).
func hwMix(c [3]uint8, keep int) [3]uint8 {
	var out [3]uint8
	for i, v := range c {
		out[i] = uint8(255 - (255-int(v))*keep/100)
	}
	return out
}

func hwText(dc uintptr, text string, rect *displayRect, flags uintptr) {
	text = strings.ReplaceAll(text, "\x00", "")
	ptr := syscall.StringToUTF16Ptr(text)
	hwDrawText.Call(dc, uintptr(unsafe.Pointer(ptr)), ^uintptr(0), uintptr(unsafe.Pointer(rect)), flags|0x0800) // DT_NOPREFIX
}

func hwTextWidth(dc uintptr, text string) int {
	rect := displayRect{}
	hwText(dc, text, &rect, 0x0020|0x0400) // SINGLELINE / CALCRECT
	return int(rect.right)
}

func hwFont(size, weight int) uintptr { return hwFontFace(size, weight, "Microsoft YaHei UI") }
func hwFontFace(size, weight int, name string) uintptr {
	face := syscall.StringToUTF16Ptr(name)
	font, _, _ := hwCreateFont.Call(uintptr(-size), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
	return font
}

// hwFillRound paints an outline-free rounded rectangle.
func hwFillRound(dc uintptr, rect displayRect, radius int, color [3]uint8) {
	brush, _, _ := hwCreateBrush.Call(hwRGB(color))
	pen, _, _ := hwStockObject.Call(8) // NULL_PEN
	oldBrush, _, _ := hwSelectObject.Call(dc, brush)
	oldPen, _, _ := hwSelectObject.Call(dc, pen)
	// A null pen leaves out the right and bottom edge, so extend by one pixel.
	hwRoundRect.Call(dc, uintptr(rect.left), uintptr(rect.top), uintptr(rect.right+1), uintptr(rect.bottom+1), uintptr(2*radius), uintptr(2*radius))
	hwSelectObject.Call(dc, oldPen)
	hwSelectObject.Call(dc, oldBrush)
	hwDeleteObject.Call(brush)
}

// applyRoundedFrame asks Windows 11 for native rounded corners and a thin
// border; older systems keep the class drop shadow and use a window region.
func (c *homeworkNativeWindow) applyRoundedFrame() {
	if hwDWMAttribute.Find() != nil {
		return
	}
	round := uint32(2) // DWMWCP_ROUND
	if result, _, _ := hwDWMAttribute.Call(c.hwnd, 33, uintptr(unsafe.Pointer(&round)), 4); result != 0 {
		return
	}
	border := uint32(hwColor(214, 223, 214))
	hwDWMAttribute.Call(c.hwnd, 34, uintptr(unsafe.Pointer(&border)), 4) // DWMWA_BORDER_COLOR
	c.dwmCorners = true
}

func (c *homeworkNativeWindow) fitRegion() {
	g := c.geometry
	if c.dwmCorners || c.regionSize == [2]int{g.Width, g.Height} {
		return
	}
	radius := 2 * 12 * g.Scale / 100
	region, _, _ := hwRoundRegion.Call(0, 0, uintptr(g.Width+1), uintptr(g.Height+1), uintptr(radius), uintptr(radius))
	if region == 0 {
		return
	}
	if ok, _, _ := hwSetRegion.Call(c.hwnd, region, 1); ok == 0 {
		hwDeleteObject.Call(region)
		return
	}
	c.regionSize = [2]int{g.Width, g.Height} // The window now owns the region.
}

func (c *homeworkNativeWindow) hitTest(x, y int) string {
	g := c.geometry
	m := homeworkCardMetrics(g, c.maxOffset > 0)
	switch {
	case x < 0 || y < 0 || x >= g.Width || y >= g.Height:
		return ""
	case y >= m.HitTop && x < g.Width/2:
		return "pin"
	case y >= m.HitTop:
		return "hide"
	case y >= m.BodyTop && c.maxOffset > 0:
		return "body"
	}
	return ""
}

func (c *homeworkNativeWindow) setHover(area string) {
	if area == "body" {
		area = ""
	}
	if area != c.hover {
		c.hover = area
		hwInvalidate.Call(c.hwnd, 0, 0)
	}
}

func (c *homeworkNativeWindow) pageStep() int {
	m := homeworkCardMetrics(c.geometry, true)
	if step := m.BodyBottom - m.BodyTop - c.geometry.Font; step > 0 {
		return step
	}
	return c.geometry.Font
}

func (c *homeworkNativeWindow) paint() {
	var paint hwPaint
	screen, _, _ := hwBeginPaint.Call(c.hwnd, uintptr(unsafe.Pointer(&paint)))
	if screen == 0 {
		return
	}
	defer hwEndPaint.Call(c.hwnd, uintptr(unsafe.Pointer(&paint)))
	g := c.geometry
	// Draw into an off-screen bitmap so scrolling and hover never flicker.
	dc, _, _ := hwCompatibleDC.Call(screen)
	if dc == 0 {
		return
	}
	defer hwDeleteDC.Call(dc)
	bitmap, _, _ := hwCompatibleBM.Call(screen, uintptr(g.Width), uintptr(g.Height))
	if bitmap == 0 {
		return
	}
	defer hwDeleteObject.Call(bitmap)
	oldBitmap, _, _ := hwSelectObject.Call(dc, bitmap)
	defer hwSelectObject.Call(dc, oldBitmap)
	c.draw(dc)
	hwBitBlt.Call(screen, 0, 0, uintptr(g.Width), uintptr(g.Height), dc, 0, 0, 0x00CC0020) // SRCCOPY
}

func (c *homeworkNativeWindow) draw(dc uintptr) {
	g := c.geometry
	s := g.Scale
	px := func(v int) int { return v * s / 100 }
	accent := homeworkSubjectColor(c.card.Subject)
	ink := [3]uint8{28, 40, 33}
	muted := [3]uint8{98, 112, 103}
	neutral := [3]uint8{240, 243, 239}

	full := displayRect{0, 0, int32(g.Width), int32(g.Height)}
	brush, _, _ := hwCreateBrush.Call(hwColor(252, 253, 251))
	hwFillRect.Call(dc, uintptr(unsafe.Pointer(&full)), brush)
	hwDeleteObject.Call(brush)
	strip := displayRect{0, 0, int32(g.Width), int32(px(4))}
	brush, _, _ = hwCreateBrush.Call(hwRGB(accent))
	hwFillRect.Call(dc, uintptr(unsafe.Pointer(&strip)), brush)
	hwDeleteObject.Call(brush)
	if !c.dwmCorners {
		pen, _, _ := hwCreatePen.Call(0, 1, hwColor(214, 223, 214))
		hollow, _, _ := hwStockObject.Call(5) // NULL_BRUSH
		oldPen, _, _ := hwSelectObject.Call(dc, pen)
		oldBrush, _, _ := hwSelectObject.Call(dc, hollow)
		radius := 2 * px(12)
		hwRoundRect.Call(dc, 0, 0, uintptr(g.Width), uintptr(g.Height), uintptr(radius), uintptr(radius))
		hwSelectObject.Call(dc, oldBrush)
		hwSelectObject.Call(dc, oldPen)
		hwDeleteObject.Call(pen)
	}
	hwSetBkMode.Call(dc, 1)

	smallSize := px(20)
	if s >= 160 && smallSize < 40 {
		smallSize = 40
	}
	chipFont := hwFont(px(20), 700)
	smallFont := hwFont(smallSize, 500)
	buttonFont := hwFont(smallSize, 600)
	iconFont := hwFontFace(smallSize, 400, "Segoe MDL2 Assets")
	font := hwFont(g.Font, 500)
	for _, f := range []uintptr{chipFont, smallFont, buttonFont, iconFont, font} {
		defer hwDeleteObject.Call(f)
	}
	old, _, _ := hwSelectObject.Call(dc, font)
	defer hwSelectObject.Call(dc, old)

	// Measure the assignment first; a scrollable card reserves a hint line.
	pad := px(18)
	measure := displayRect{int32(pad), 0, int32(g.Width - pad), 0}
	hwText(dc, c.card.Text, &measure, 0x0010|0x0400) // WORDBREAK / CALCRECT
	m := homeworkCardMetrics(g, false)
	if int(measure.bottom) > m.BodyBottom-m.BodyTop {
		m = homeworkCardMetrics(g, true)
	}
	bodyHeight := m.BodyBottom - m.BodyTop
	c.maxOffset = int(measure.bottom) - bodyHeight
	if c.maxOffset < 0 {
		c.maxOffset = 0
	}
	if c.offset > c.maxOffset {
		c.offset = c.maxOffset
	}

	// Header: subject chip on the left, teacher and update time on the right.
	hwSelectObject.Call(dc, chipFont)
	subject := strings.TrimSpace(c.card.Subject)
	if subject == "" {
		subject = "作业"
	}
	chipWidth := hwTextWidth(dc, subject) + 2*px(14)
	if chipWidth > g.Width/2 {
		chipWidth = g.Width / 2
	}
	chip := displayRect{int32(m.Pad), int32(m.Pad), int32(m.Pad + chipWidth), int32(m.Pad + m.ChipHeight)}
	hwFillRound(dc, chip, px(9), hwMix(accent, 13))
	hwSetTextColor.Call(dc, hwRGB(accent))
	hwText(dc, subject, &chip, 0x0001|0x0004|0x0020|0x8000) // CENTER / VCENTER / SINGLELINE / END_ELLIPSIS
	meta := strings.TrimSpace(c.card.TeacherName)
	if updated, err := time.Parse(time.RFC3339Nano, c.card.UpdatedAt); err == nil {
		updated = updated.Local()
		stamp := updated.Format("1月2日 15:04")
		if y, mo, d := time.Now().Date(); updated.Year() == y && updated.Month() == mo && updated.Day() == d {
			stamp = "今天 " + updated.Format("15:04")
		}
		if meta != "" {
			meta += " · "
		}
		meta += stamp
	}
	hwSelectObject.Call(dc, smallFont)
	hwSetTextColor.Call(dc, hwRGB(muted))
	metaRect := displayRect{chip.right + int32(px(12)), chip.top, int32(g.Width - m.Pad), chip.bottom}
	hwText(dc, meta, &metaRect, 0x0002|0x0004|0x0020|0x8000) // RIGHT / VCENTER / SINGLELINE / END_ELLIPSIS

	// Body: GDI clipping keeps long assignments away from the controls.
	hwSelectObject.Call(dc, font)
	hwSetTextColor.Call(dc, hwRGB(ink))
	saved, _, _ := hwSaveDC.Call(dc)
	hwClipRect.Call(dc, uintptr(pad), uintptr(m.BodyTop), uintptr(g.Width-pad), uintptr(m.BodyBottom))
	body := displayRect{int32(pad), int32(m.BodyTop - c.offset), int32(g.Width - pad), int32(m.BodyTop-c.offset) + measure.bottom}
	hwText(dc, c.card.Text, &body, 0x0010)
	hwRestoreDC.Call(dc, saved)

	if c.maxOffset > 0 {
		// A slim scrollbar in the right padding shows position without a wheel.
		barWidth := px(4)
		if barWidth < 3 {
			barWidth = 3
		}
		barX := g.Width - pad/2 - barWidth/2
		track := displayRect{int32(barX), int32(m.BodyTop), int32(barX + barWidth), int32(m.BodyBottom)}
		hwFillRound(dc, track, barWidth/2, [3]uint8{232, 237, 231})
		thumbHeight := bodyHeight * bodyHeight / int(measure.bottom)
		if thumbHeight < px(24) {
			thumbHeight = px(24)
		}
		thumbTop := m.BodyTop + (bodyHeight-thumbHeight)*c.offset/c.maxOffset
		thumb := displayRect{int32(barX), int32(thumbTop), int32(barX + barWidth), int32(thumbTop + thumbHeight)}
		hwFillRound(dc, thumb, barWidth/2, hwMix(accent, 55))
		step := c.pageStep()
		pages := 1 + (c.maxOffset+step-1)/step
		page := 1 + c.offset/step
		if c.offset >= c.maxOffset {
			page = pages
		}
		hwSelectObject.Call(dc, smallFont)
		hwSetTextColor.Call(dc, hwRGB(muted))
		hint := displayRect{int32(pad), int32(m.HintTop), int32(g.Width - pad), int32(m.HintTop + px(26))}
		hwText(dc, fmt.Sprintf("点按正文翻页 · %d/%d", page, pages), &hint, 0x0004|0x0020|0x8000)
	}

	// Footer: two rounded buttons; the whole bottom strip stays clickable.
	gap := px(10)
	half := (g.Width - 2*m.Pad - gap) / 2
	top, bottom := int32(m.FooterTop), int32(m.FooterTop+m.FooterHeight)
	buttons := []struct {
		area, icon, label string
		rect              displayRect
		fill, text        [3]uint8
	}{
		{"pin", "", "置顶显示", displayRect{int32(m.Pad), top, int32(m.Pad + half), bottom}, neutral, [3]uint8{52, 66, 57}},
		{"hide", "", "隐藏", displayRect{int32(m.Pad + half + gap), top, int32(g.Width - m.Pad), bottom}, neutral, [3]uint8{52, 66, 57}},
	}
	if c.card.AlwaysOnTop {
		buttons[0].icon, buttons[0].label, buttons[0].fill, buttons[0].text = "", "取消置顶", hwMix(accent, 13), accent
	}
	for _, b := range buttons {
		fill := b.fill
		if c.hover == b.area {
			for i := range fill {
				fill[i] = uint8(int(fill[i]) * 93 / 100)
			}
		}
		hwFillRound(dc, b.rect, px(10), fill)
		hwSetTextColor.Call(dc, hwRGB(b.text))
		hwSelectObject.Call(dc, buttonFont)
		labelWidth := hwTextWidth(dc, b.label)
		iconWidth, spacing := smallSize, px(8)
		width := int(b.rect.right - b.rect.left)
		if iconWidth+spacing+labelWidth > width-2*px(8) {
			hwText(dc, b.label, &b.rect, 0x0001|0x0004|0x0020|0x8000)
			continue
		}
		start := int(b.rect.left) + (width-iconWidth-spacing-labelWidth)/2
		icon := displayRect{int32(start), b.rect.top, int32(start + iconWidth), b.rect.bottom}
		label := displayRect{int32(start + iconWidth + spacing), b.rect.top, b.rect.right, b.rect.bottom}
		hwText(dc, b.label, &label, 0x0004|0x0020)
		hwSelectObject.Call(dc, iconFont)
		hwText(dc, b.icon, &icon, 0x0001|0x0004|0x0020)
	}
}
