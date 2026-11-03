//go:build windows

package main

import (
	"os"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// Run by the native Windows Actions runner, without an emulated desktop.
func TestNativeHomeworkWindows(t *testing.T) {
	displays, err := listDisplays()
	if err != nil || len(displays) == 0 {
		t.Skip("runner has no desktop monitor")
	}
	actions := make(chan string, 2)
	manager, err := newHomeworkWindows(func(id, action string) { actions <- id + ":" + action })
	if err != nil {
		t.Fatal(err)
	}
	defer manager.close()
	items := []HomeworkCard{{ID: "h1", Subject: "语文", Text: "完成练习册第一课", TeacherName: "林老师", Visible: true, AlwaysOnTop: true}, {ID: "h2", Subject: "数学", Text: "完成课后练习", TeacherName: "王老师", Visible: true}}
	if err := manager.update(displays[0].ID, items, false); err != nil {
		t.Fatal(err)
	}
	findVisible := func() []uintptr {
		var found []uintptr
		callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
			var pid uint32
			getWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
			if int(pid) != os.Getpid() {
				return 1
			}
			var class [128]uint16
			user32.NewProc("GetClassNameW").Call(hwnd, uintptr(unsafe.Pointer(&class[0])), 128)
			visible, _, _ := user32.NewProc("IsWindowVisible").Call(hwnd)
			if syscall.UTF16ToString(class[:]) == "ClassDingHomeworkWindow" && visible != 0 {
				found = append(found, hwnd)
			}
			return 1
		})
		user32.NewProc("EnumWindows").Call(callback, 0)
		return found
	}
	waitCount := func(n int) []uintptr {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			found := findVisible()
			if len(found) == n {
				return found
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("expected %d independent desktop windows, got %d", n, len(findVisible()))
		return nil
	}
	windows := waitCount(2)
	for _, hwnd := range windows {
		var rect displayRect
		getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
		work := displays[0].work
		if rect.left < work.left || rect.right > work.right || rect.top < work.top || rect.bottom > work.bottom {
			t.Fatalf("window overlaps taskbar: %+v work %+v", rect, work)
		}
		exstyle, _, _ := user32.NewProc("GetWindowLongPtrW").Call(hwnd, ^uintptr(19)) // GWL_EXSTYLE=-20
		if exstyle&0x08000000 == 0 || exstyle&0x80 == 0 {
			t.Fatal("homework must not steal focus or create taskbar entries")
		}
	}
	var rect displayRect
	getWindowRect.Call(windows[0], uintptr(unsafe.Pointer(&rect)))
	x, y := int(rect.right-rect.left)-20, int(rect.bottom-rect.top)-20
	postMessage.Call(windows[0], wmLeftButtonUp, 0, uintptr(uint32(y)<<16|uint32(x)))
	select {
	case action := <-actions:
		if action != "h1:hide" && action != "h2:hide" {
			t.Fatal(action)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("hide control did not dispatch")
	}
	if err := manager.update(displays[0].ID, items, true); err != nil {
		t.Fatal(err)
	}
	waitCount(0)
	if err := manager.update(displays[0].ID, items, false); err != nil {
		t.Fatal(err)
	}
	waitCount(2)
	items[0].Visible = false
	if err := manager.update(displays[0].ID, items, false); err != nil {
		t.Fatal(err)
	}
	waitCount(1)
	if err := manager.update(displays[0].ID, nil, false); err != nil {
		t.Fatal(err)
	}
	waitCount(0)
}
