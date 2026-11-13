package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is bound to the web view. Exported methods are callable from the
// frontend; they wait for startup to finish before touching local data.
type App struct {
	ctx      context.Context
	dataDir  string
	initErr  error
	initDone chan struct{}

	windowMu sync.Mutex
	tray     *trayHandle
	trayErr  error
	hidden   bool
	quitting bool

	displayMu sync.Mutex
	displayID string

	// Local reverse proxy that lets the web view reach the configured server.
	proxy    *http.Server
	proxyURL string
	secret   string
	targetMu sync.RWMutex
	target   *url.URL

	audioMu  sync.Mutex
	original *volumeSnapshot

	homework    homeworkState
	studentCall studentCallState
}

func NewApp() (*App, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return &App{
		dataDir:  filepath.Join(filepath.Dir(executable), "data"),
		initDone: make(chan struct{}),
		hidden:   supportsTray() && hasSilentArg(os.Args),
	}, nil
}

// hasSilentArg reports whether the app was started in the background, as the
// autostart entry does.
func hasSilentArg(args []string) bool { return slices.Contains(args, "--silent") }

// ready waits for startup and reports whether local data is usable.
func (a *App) ready() error {
	<-a.initDone
	return a.initErr
}

func (a *App) initialize() error {
	if err := os.MkdirAll(a.dataDir, 0700); err != nil {
		return fmt.Errorf("无法创建数据目录 %s: %w", a.dataDir, err)
	}
	a.loadDisplayPreference()
	a.loadHomeworkPreferences()
	return a.startProxy()
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	defer close(a.initDone)
	if a.initErr = a.initialize(); a.initErr != nil {
		log.Printf("初始化大屏失败: %v", a.initErr)
	}
	if supportsTray() {
		tray, trayErr := startTray(a.showWindow, a.quit, a.trayUnavailable)
		a.windowMu.Lock()
		a.tray, a.trayErr = tray, trayErr
		a.windowMu.Unlock()
		if trayErr != nil {
			log.Printf("托盘初始化失败: %v", trayErr)
		}
	}
	if a.initErr == nil {
		if err := a.startStudentCall(); err != nil {
			log.Printf("学生呼叫悬浮入口初始化失败: %v", err)
		}
		if err := a.RestoreAudio(); err != nil {
			log.Printf("恢复上次音量失败: %v", err)
		}
	}
}

// domReady shows a window that started hidden when nothing else can reveal it.
func (a *App) domReady(_ context.Context) {
	<-a.initDone
	a.windowMu.Lock()
	hidden, hasTray := a.hidden, a.tray != nil
	a.windowMu.Unlock()
	if hidden && (a.initErr != nil || supportsTray() && !hasTray) {
		a.showWindow()
	}
}

// beforeClose hides the window to the tray instead of quitting, when it can.
func (a *App) beforeClose(ctx context.Context) bool {
	<-a.initDone
	a.windowMu.Lock()
	if a.quitting || a.tray == nil {
		a.windowMu.Unlock()
		return false
	}
	a.hidden = true
	a.windowMu.Unlock()
	runtime.WindowHide(ctx)
	return true
}

func (a *App) showWindow() {
	if a.ctx == nil {
		return
	}
	a.windowMu.Lock()
	a.hidden = false
	a.windowMu.Unlock()
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
}

func (a *App) quit() {
	if a.ctx == nil {
		return
	}
	a.windowMu.Lock()
	a.quitting = true
	a.windowMu.Unlock()
	runtime.Quit(a.ctx)
}

func (a *App) trayUnavailable() {
	<-a.initDone
	a.windowMu.Lock()
	tray := a.tray
	a.tray = nil
	a.trayErr = errors.New("系统托盘不可用，关闭窗口将退出程序")
	a.windowMu.Unlock()
	if tray != nil {
		tray.close()
	}
	a.showWindow()
	runtime.EventsEmit(a.ctx, "tray:unavailable")
}

func (a *App) shutdown(_ context.Context) {
	a.closeStudentCall()
	a.closeHomework()
	a.windowMu.Lock()
	tray := a.tray
	a.windowMu.Unlock()
	if tray != nil {
		tray.close()
	}
	if err := a.RestoreAudio(); err != nil {
		log.Printf("恢复音量失败: %v", err)
	}
	if a.proxy != nil {
		_ = a.proxy.Close()
	}
}

func (a *App) GetTrayError() string {
	<-a.initDone
	a.windowMu.Lock()
	defer a.windowMu.Unlock()
	if a.trayErr != nil {
		return a.trayErr.Error()
	}
	return ""
}

// SetPresentationActive brings the window full screen on the chosen display
// while a notification plays, and puts it back afterwards.
func (a *App) SetPresentationActive(active bool) error {
	if a.ctx == nil {
		return nil
	}
	<-a.initDone
	a.suspendHomework(active)
	a.suspendStudentCall(active)
	if !active {
		runtime.WindowUnfullscreen(a.ctx)
		runtime.WindowSetAlwaysOnTop(a.ctx, false)
		a.windowMu.Lock()
		hidden := a.hidden
		a.windowMu.Unlock()
		if hidden {
			runtime.WindowHide(a.ctx)
		}
		return nil
	}
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
	err := moveToDisplay(a.selectedDisplay())
	runtime.WindowFullscreen(a.ctx)
	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	return err
}

func (a *App) SetAutostart(enabled bool) error { return setAutostart(enabled) }
func (a *App) GetAutostart() (bool, error)     { return getAutostart() }
