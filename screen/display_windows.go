//go:build windows

package main

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

type displayRect struct{ left, top, right, bottom int32 }
type monitorInfo struct {
	size          uint32
	monitor, work displayRect
	flags         uint32
	device        [32]uint16
}

var (
	enumDisplayMonitors      = user32.NewProc("EnumDisplayMonitors")
	getMonitorInfo           = user32.NewProc("GetMonitorInfoW")
	findWindow               = user32.NewProc("FindWindowW")
	getWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	getWindowRect            = user32.NewProc("GetWindowRect")
	setWindowPos             = user32.NewProc("SetWindowPos")
	displayEnumMu            sync.Mutex
	displayResults           []ScreenDisplay
	displayEnumCallback      = syscall.NewCallback(func(monitor, _, _, _ uintptr) uintptr {
		info := monitorInfo{size: uint32(unsafe.Sizeof(monitorInfo{}))}
		if ok, _, _ := getMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); ok != 0 {
			id := syscall.UTF16ToString(info.device[:])
			displayResults = append(displayResults, ScreenDisplay{ID: id, Primary: info.flags&1 != 0, Width: int(info.monitor.right - info.monitor.left), Height: int(info.monitor.bottom - info.monitor.top), rect: info.monitor})
		}
		return 1
	})
)

func listDisplays() ([]ScreenDisplay, error) {
	displayEnumMu.Lock()
	defer displayEnumMu.Unlock()
	displayResults = nil
	ok, _, err := enumDisplayMonitors.Call(0, 0, displayEnumCallback, 0)
	if ok == 0 {
		return nil, fmt.Errorf("无法枚举显示器：%w", err)
	}
	result := append(make([]ScreenDisplay, 0, len(displayResults)), displayResults...)
	for i := range result {
		result[i].Name = fmt.Sprintf("显示器 %d · %d × %d", i+1, result[i].Width, result[i].Height)
		if result[i].Primary {
			result[i].Name += "（主屏）"
		}
	}
	return result, nil
}
func moveToDisplay(id string) error {
	if id == "" {
		return nil
	}
	displays, err := listDisplays()
	if err != nil {
		return err
	}
	var selected *ScreenDisplay
	for i := range displays {
		if displays[i].ID == id {
			selected = &displays[i]
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("所选显示器已断开，将使用当前显示器")
	}
	class := syscall.StringToUTF16Ptr("ClassDingMainWindow")
	hwnd, _, _ := findWindow.Call(uintptr(unsafe.Pointer(class)), 0)
	if hwnd == 0 {
		return fmt.Errorf("无法找到大屏窗口")
	}
	var pid uint32
	getWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if int(pid) != os.Getpid() {
		return fmt.Errorf("无法确认大屏窗口")
	}
	var rect displayRect
	if ok, _, err := getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect))); ok == 0 {
		return fmt.Errorf("无法读取窗口位置：%w", err)
	}
	x := selected.rect.left + (selected.rect.right-selected.rect.left-(rect.right-rect.left))/2
	y := selected.rect.top + (selected.rect.bottom-selected.rect.top-(rect.bottom-rect.top))/2
	// Keep size and z-order, and do not activate the window just to move it.
	if ok, _, err := setWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), 0, 0, 0x0001|0x0004|0x0010); ok == 0 {
		return fmt.Errorf("无法切换显示器：%w", err)
	}
	return nil
}
