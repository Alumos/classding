//go:build windows

package main

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

const (
	wmDestroy        = 0x0002
	wmClose          = 0x0010
	wmNull           = 0x0000
	wmApp            = 0x8000
	wmLeftButtonUp   = 0x0202
	wmRightButtonUp  = 0x0205
	wmContextMenu    = 0x007b
	trayMessage      = wmApp + 1
	nimAdd           = 0x00000000
	nimDelete        = 0x00000002
	nifMessage       = 0x00000001
	nifIcon          = 0x00000002
	nifTip           = 0x00000004
	tpmRightButton   = 0x0002
	tpmNonotify      = 0x0080
	tpmReturnCommand = 0x0100
)

var (
	user32                = syscall.NewLazyDLL("user32.dll")
	shell32               = syscall.NewLazyDLL("shell32.dll")
	registerClassEx       = user32.NewProc("RegisterClassExW")
	unregisterClass       = user32.NewProc("UnregisterClassW")
	createWindowEx        = user32.NewProc("CreateWindowExW")
	defWindowProc         = user32.NewProc("DefWindowProcW")
	destroyWindow         = user32.NewProc("DestroyWindow")
	getMessage            = user32.NewProc("GetMessageW")
	translateMessage      = user32.NewProc("TranslateMessage")
	dispatchMessage       = user32.NewProc("DispatchMessageW")
	postMessage           = user32.NewProc("PostMessageW")
	postQuitMessage       = user32.NewProc("PostQuitMessage")
	registerWindowMessage = user32.NewProc("RegisterWindowMessageW")
	createPopupMenu       = user32.NewProc("CreatePopupMenu")
	appendMenu            = user32.NewProc("AppendMenuW")
	trackPopupMenu        = user32.NewProc("TrackPopupMenu")
	destroyMenu           = user32.NewProc("DestroyMenu")
	getCursorPos          = user32.NewProc("GetCursorPos")
	setForegroundWindow   = user32.NewProc("SetForegroundWindow")
	loadIcon              = user32.NewProc("LoadIconW")
	destroyIcon           = user32.NewProc("DestroyIcon")
	shellNotifyIcon       = shell32.NewProc("Shell_NotifyIconW")
	extractIconEx         = shell32.NewProc("ExtractIconExW")
	getModuleHandle       = kernel32.NewProc("GetModuleHandleW")
	trayWindows           sync.Map
	trayWindowProc        = syscall.NewCallback(windowProc)
)

type trayWindowClass struct {
	size, style                                                        uint32
	windowProc                                                         uintptr
	classExtra, windowExtra                                            int32
	instance, icon, cursor, background, menuName, className, smallIcon uintptr
}
type trayPoint struct{ x, y int32 }
type trayMessageData struct {
	window         uintptr
	message        uint32
	wParam, lParam uintptr
	time           uint32
	point          trayPoint
	private        uint32
}
type trayIconData struct {
	size                uint32
	window              uintptr
	id, flags, callback uint32
	icon                uintptr
	tip                 [128]uint16
	state, stateMask    uint32
	info                [256]uint16
	version             uint32
	infoTitle           [64]uint16
	infoFlags           uint32
	guid                [16]byte
	balloonIcon         uintptr
}
type trayHandle struct {
	window         uintptr
	icon           uintptr
	ownedIcon      bool
	restartMessage uint32
	onShow         func()
	onQuit         func()
	onUnavailable  func()
}

func supportsTray() bool { return true }

func (t *trayHandle) notify(action uintptr) bool {
	data := trayIconData{size: uint32(unsafe.Sizeof(trayIconData{})), window: t.window, id: 1, flags: nifMessage | nifIcon | nifTip, callback: trayMessage, icon: t.icon}
	copy(data.tip[:], syscall.StringToUTF16("课堂叮咚 · 正在后台守候"))
	ok, _, _ := shellNotifyIcon.Call(action, uintptr(unsafe.Pointer(&data)))
	return ok != 0
}
func (t *trayHandle) menu() {
	menu, _, _ := createPopupMenu.Call()
	if menu == 0 {
		return
	}
	defer destroyMenu.Call(menu)
	show := syscall.StringToUTF16Ptr("打开课堂叮咚")
	quit := syscall.StringToUTF16Ptr("退出课堂叮咚")
	appendMenu.Call(menu, 0, 1, uintptr(unsafe.Pointer(show)))
	appendMenu.Call(menu, 0, 2, uintptr(unsafe.Pointer(quit)))
	var point trayPoint
	getCursorPos.Call(uintptr(unsafe.Pointer(&point)))
	setForegroundWindow.Call(t.window)
	command, _, _ := trackPopupMenu.Call(menu, tpmRightButton|tpmNonotify|tpmReturnCommand, uintptr(point.x), uintptr(point.y), 0, t.window, 0)
	postMessage.Call(t.window, wmNull, 0, 0)
	switch command {
	case 1:
		go t.onShow()
	case 2:
		go t.onQuit()
	}
}
func windowProc(hwnd, message, wParam, lParam uintptr) uintptr {
	value, found := trayWindows.Load(hwnd)
	if found {
		t := value.(*trayHandle)
		if t.restartMessage != 0 && uint32(message) == t.restartMessage {
			if !t.notify(nimAdd) {
				go t.onUnavailable()
			}
			return 0
		}
		switch message {
		case trayMessage:
			switch uint32(lParam) {
			case wmLeftButtonUp:
				go t.onShow()
			case wmRightButtonUp, wmContextMenu:
				t.menu()
			}
			return 0
		case wmClose:
			destroyWindow.Call(hwnd)
			return 0
		case wmDestroy:
			t.notify(nimDelete)
			trayWindows.Delete(hwnd)
			postQuitMessage.Call(0)
			return 0
		}
	}
	result, _, _ := defWindowProc.Call(hwnd, message, wParam, lParam)
	return result
}
func startTray(onShow, onQuit, onUnavailable func()) (*trayHandle, error) {
	type result struct {
		tray *trayHandle
		err  error
	}
	ready := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		className := syscall.StringToUTF16Ptr("ClassDingTrayWindow")
		instance, _, _ := getModuleHandle.Call(0)
		class := trayWindowClass{size: uint32(unsafe.Sizeof(trayWindowClass{})), windowProc: trayWindowProc, instance: instance, className: uintptr(unsafe.Pointer(className))}
		if atom, _, err := registerClassEx.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
			ready <- result{err: fmt.Errorf("无法注册托盘窗口：%w", err)}
			return
		}
		defer unregisterClass.Call(uintptr(unsafe.Pointer(className)), instance)
		hwnd, _, err := createWindowEx.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, 0, instance, 0)
		if hwnd == 0 {
			ready <- result{err: fmt.Errorf("无法创建托盘窗口：%w", err)}
			return
		}
		t := &trayHandle{window: hwnd, onShow: onShow, onQuit: onQuit, onUnavailable: onUnavailable}
		if executable, e := os.Executable(); e == nil {
			path := syscall.StringToUTF16Ptr(executable)
			count, _, _ := extractIconEx.Call(uintptr(unsafe.Pointer(path)), 0, 0, uintptr(unsafe.Pointer(&t.icon)), 1)
			t.ownedIcon = count > 0 && t.icon != 0
		}
		if t.icon == 0 {
			t.icon, _, _ = loadIcon.Call(0, 32512)
		}
		if t.ownedIcon {
			defer destroyIcon.Call(t.icon)
		}
		name := syscall.StringToUTF16Ptr("TaskbarCreated")
		message, _, _ := registerWindowMessage.Call(uintptr(unsafe.Pointer(name)))
		t.restartMessage = uint32(message)
		trayWindows.Store(hwnd, t)
		if t.icon == 0 || !t.notify(nimAdd) {
			trayWindows.Delete(hwnd)
			destroyWindow.Call(hwnd)
			ready <- result{err: fmt.Errorf("无法显示系统托盘图标")}
			return
		}
		ready <- result{tray: t}
		var event trayMessageData
		for {
			ret, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&event)), 0, 0, 0)
			if int32(ret) <= 0 {
				break
			}
			translateMessage.Call(uintptr(unsafe.Pointer(&event)))
			dispatchMessage.Call(uintptr(unsafe.Pointer(&event)))
		}
	}()
	r := <-ready
	return r.tray, r.err
}
func (t *trayHandle) close() {
	if t != nil {
		postMessage.Call(t.window, wmClose, 0, 0)
	}
}
