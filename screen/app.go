package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type volumeSnapshot struct {
	DeviceID string  `json:"device_id"`
	Volume   float64 `json:"volume"`
	Muted    bool    `json:"muted"`
}
type ScreenDisplay struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Primary bool   `json:"primary"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	rect    displayRect
	work    displayRect
}
type DisplaySettings struct {
	Selected string          `json:"selected"`
	Displays []ScreenDisplay `json:"displays"`
}
type App struct {
	homework  homeworkState
	ctx       context.Context
	dataDir   string
	initErr   error
	initDone  chan struct{}
	trayErr   error
	tray      *trayHandle
	windowMu  sync.Mutex
	hidden    bool
	quitting  bool
	displayMu sync.Mutex
	displayID string
	proxy     *http.Server
	proxyURL  string
	secret    string
	target    *url.URL
	targetMu  sync.RWMutex
	audioMu   sync.Mutex
	original  *volumeSnapshot
}

func NewApp() (*App, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(filepath.Dir(executable), "data")
	hidden := supportsTray() && hasSilentArg(os.Args)
	return &App{dataDir: dir, initDone: make(chan struct{}), hidden: hidden}, nil
}
func hasSilentArg(args []string) bool {
	for _, arg := range args {
		if arg == "--silent" {
			return true
		}
	}
	return false
}
func (a *App) initialize() error {
	if err := os.MkdirAll(a.dataDir, 0700); err != nil {
		return fmt.Errorf("无法创建数据目录 %s: %w", a.dataDir, err)
	}
	if data, err := os.ReadFile(filepath.Join(a.dataDir, "display.json")); err == nil {
		var settings struct {
			Selected string `json:"selected"`
		}
		if json.Unmarshal(data, &settings) == nil {
			a.displayID = settings.Selected
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	secret := make([]byte, 24)
	if _, err := rand.Read(secret); err != nil {
		listener.Close()
		return err
	}
	a.loadHomeworkPreferences()
	a.secret = hex.EncodeToString(secret)
	a.proxyURL = "http://" + listener.Addr().String() + "/" + a.secret
	a.proxy = &http.Server{Handler: http.HandlerFunc(a.proxyHandler), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := a.proxy.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("local proxy: %v", err)
		}
	}()
	return nil
}
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	defer close(a.initDone)
	a.initErr = a.initialize()
	if a.initErr != nil {
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
		if err := a.RestoreAudio(); err != nil {
			log.Printf("恢复上次音量失败: %v", err)
		}
	}
}
func (a *App) domReady(_ context.Context) {
	<-a.initDone
	a.windowMu.Lock()
	hidden := a.hidden
	hasTray := a.tray != nil
	a.windowMu.Unlock()
	if hidden && (a.initErr != nil || (supportsTray() && !hasTray)) {
		a.showWindow()
	}
}
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
func (a *App) LoadState() (string, error) {
	<-a.initDone
	if a.initErr != nil {
		return "", a.initErr
	}
	data, err := os.ReadFile(filepath.Join(a.dataDir, "state.bin"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	plain, err := unprotect(data)
	if err != nil {
		return "", fmt.Errorf("无法读取本机加密数据：%w", err)
	}
	return string(plain), nil
}
func (a *App) SaveState(value string) error {
	<-a.initDone
	if a.initErr != nil {
		return a.initErr
	}
	if len(value) > 8*1024*1024 {
		return errors.New("本地状态过大")
	}
	data, err := protect([]byte(value))
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(a.dataDir, "state.bin"), data)
}
func atomicWrite(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".classding-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err = temp.Chmod(0600); err == nil {
		_, err = temp.Write(data)
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
func parseServer(server string) (*url.URL, error) {
	if strings.ContainsAny(server, "?#\\") {
		return nil, errors.New("服务器地址无效")
	}
	parsed, err := url.Parse(server)
	if err != nil || parsed.Host == "" || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Opaque != "" || parsed.RawPath != "" || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("服务器地址无效")
	}
	return parsed, nil
}

func (a *App) ConfigureServer(server string) (string, error) {
	<-a.initDone
	if a.initErr != nil {
		return "", a.initErr
	}
	var target *url.URL
	if server != "" {
		parsed, err := parseServer(server)
		if err != nil {
			return "", err
		}
		target = parsed
	}
	a.targetMu.Lock()
	a.target = target
	a.targetMu.Unlock()
	return a.proxyURL, nil
}
func (a *App) proxyHandler(w http.ResponseWriter, r *http.Request) {
	prefix := "/" + a.secret
	if !strings.HasPrefix(r.URL.Path, prefix+"/") {
		http.NotFound(w, r)
		return
	}
	a.targetMu.RLock()
	target := a.target
	a.targetMu.RUnlock()
	if target == nil {
		http.Error(w, "请配置服务器地址", http.StatusBadGateway)
		return
	}
	// The unguessable per-process path limits access to this local proxy. CORS is
	// scoped to the requesting origin so the embedded WebView can use it.
	origin := r.Header.Get("Origin")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Private-Network", "true")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	r.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
	proxy := httputil.NewSingleHostReverseProxy(target)
	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		director(req)
		req.Host = target.Host
		if req.Header.Get("Origin") != "" {
			req.Header.Set("Origin", target.Scheme+"://"+target.Host)
		}
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Del("Access-Control-Allow-Origin")
		resp.Header.Del("Access-Control-Allow-Credentials")
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		http.Error(w, "连接服务器失败："+err.Error(), http.StatusBadGateway)
	}
	proxy.ServeHTTP(w, r)
}
func (a *App) SetPresentationActive(active bool) error {
	if a.ctx == nil {
		return nil
	}
	<-a.initDone
	a.suspendHomework(active)
	var displayErr error
	if active {
		runtime.WindowShow(a.ctx)
		runtime.WindowUnminimise(a.ctx)
		a.displayMu.Lock()
		displayID := a.displayID
		a.displayMu.Unlock()
		displayErr = moveToDisplay(displayID)
		runtime.WindowFullscreen(a.ctx)
		runtime.WindowSetAlwaysOnTop(a.ctx, true)
	} else {
		runtime.WindowUnfullscreen(a.ctx)
		runtime.WindowSetAlwaysOnTop(a.ctx, false)
		a.windowMu.Lock()
		hidden := a.hidden
		a.windowMu.Unlock()
		if hidden {
			runtime.WindowHide(a.ctx)
		}
	}
	return displayErr
}
func (a *App) GetDisplaySettings() (DisplaySettings, error) {
	<-a.initDone
	if a.initErr != nil {
		return DisplaySettings{}, a.initErr
	}
	displays, err := listDisplays()
	if err != nil {
		return DisplaySettings{}, err
	}
	a.displayMu.Lock()
	selected := a.displayID
	a.displayMu.Unlock()
	return DisplaySettings{Selected: selected, Displays: displays}, nil
}
func (a *App) SetSelectedDisplay(id string) error {
	<-a.initDone
	if a.initErr != nil {
		return a.initErr
	}
	if id != "" {
		displays, err := listDisplays()
		if err != nil {
			return err
		}
		found := false
		for _, display := range displays {
			if display.ID == id {
				found = true
				break
			}
		}
		if !found {
			return errors.New("显示器已断开，请重新选择")
		}
	}
	data, err := json.Marshal(struct {
		Selected string `json:"selected"`
	}{Selected: id})
	if err != nil {
		return err
	}
	a.displayMu.Lock()
	defer a.displayMu.Unlock()
	if err = atomicWrite(filepath.Join(a.dataDir, "display.json"), data); err != nil {
		return err
	}
	a.displayID = id
	go func() {
		a.homework.mu.Lock()
		defer a.homework.mu.Unlock()
		if a.homework.windows != nil {
			_ = a.renderHomeworkLocked()
		}
	}()
	return nil
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
func (a *App) PrepareAudio(volume int) error {
	a.audioMu.Lock()
	defer a.audioMu.Unlock()
	if a.original != nil {
		return errors.New("已有播报正在进行")
	}
	old, err := audioGet()
	if err != nil {
		return err
	}
	recovery, err := json.Marshal(old)
	if err != nil {
		return err
	}
	path := filepath.Join(a.dataDir, "volume-recovery.json")
	if err = atomicWrite(path, recovery); err != nil {
		return err
	}
	a.original = old
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	if err = audioSet(&volumeSnapshot{DeviceID: old.DeviceID, Volume: float64(volume) / 100, Muted: false}); err != nil {
		if restoreErr := audioSet(old); restoreErr == nil {
			_ = os.Remove(path)
			a.original = nil
		}
		return err
	}
	return nil
}
func (a *App) RestoreAudio() error {
	a.audioMu.Lock()
	defer a.audioMu.Unlock()
	path := filepath.Join(a.dataDir, "volume-recovery.json")
	old := a.original
	if old == nil {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		old = new(volumeSnapshot)
		if err := json.Unmarshal(data, old); err != nil {
			return err
		}
	}
	if err := audioSet(old); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	a.original = nil
	return nil
}
func (a *App) SetAutostart(enabled bool) error { return setAutostart(enabled) }
func (a *App) GetAutostart() (bool, error)     { return getAutostart() }
