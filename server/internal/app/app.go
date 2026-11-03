package app

import (
	"bytes"
	"classding/server/internal/repo"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Config struct {
	TrustedProxies                      []string
	DataDir, JWTSecret, AdminTOTPSecret string
	DevSeed, SecureCookies              bool
	SendCooldown                        time.Duration
}
type bucket struct {
	Count   int
	Expires time.Time
}
type binding struct {
	ClassID string
	Expires string
}
type App struct {
	mu           sync.Mutex
	state        State
	store        *repo.Store
	config       Config
	router       http.Handler
	hub          *Hub
	bindings     map[string]binding
	limits       map[string]bucket
	lastSent     map[string]time.Time
	stop         chan struct{}
	jobs         []func()
	rollbackJobs []func()
}
type apiError struct {
	Status        int
	Code, Message string
}

func (e *apiError) Error() string                 { return e.Message }
func fail(status int, code, message string) error { return &apiError{status, code, message} }
func bad(message string) error                    { return fail(400, "INVALID_INPUT", message) }
func forbidden() error                            { return fail(403, "FORBIDDEN", "你没有权限执行此操作。") }
func missing() error                              { return fail(404, "NOT_FOUND", "请求的内容不存在或已被移除。") }
func New(c Config) (*App, error) {
	if len(c.JWTSecret) < 32 {
		return nil, errors.New("JWT_SECRET must contain at least 32 characters")
	}
	if c.DataDir == "" {
		c.DataDir = "data"
	}
	if err := os.MkdirAll(filepath.Join(c.DataDir, "audio"), 0700); err != nil {
		return nil, err
	}
	store, err := repo.Open(filepath.Join(c.DataDir, "classding.db"))
	if err != nil {
		return nil, err
	}
	a := &App{config: c, state: newState(), store: store, hub: NewHub(), bindings: map[string]binding{}, limits: map[string]bucket{}, lastSent: map[string]time.Time{}, stop: make(chan struct{})}
	if err = store.Load(context.Background(), &a.state); err != nil {
		store.Close()
		return nil, err
	}
	for key, d := range a.state.Devices {
		d.Online = false
		a.state.Devices[key] = d
	}
	// Preserve the sequence of databases created before devices stored a counter.
	for _, message := range a.state.Messages {
		for _, target := range message.Targets {
			if device, exists := a.state.Devices[target.DeviceID]; exists && target.Seq > device.Sequence {
				device.Sequence = target.Seq
				a.state.Devices[device.ID] = device
			}
		}
	}
	if c.DevSeed {
		a.seed()
	}
	if err = store.Save(context.Background(), a.state); err != nil {
		store.Close()
		return nil, err
	}
	a.routes()
	go a.maintenance()
	return a, nil
}
func (a *App) Handler() http.Handler { return a.router }
func (a *App) Close() error {
	close(a.stop)
	a.hub.Close()
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.store.Close()
}
func (a *App) Save() error { return a.store.Save(context.Background(), a.state) }

type endpoint func(http.ResponseWriter, *http.Request, *User) (any, error)

func (a *App) handle(auth, write bool, fn endpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			if origin := r.Header.Get("Origin"); origin != "" {
				scheme := "http"
				if r.TLS != nil {
					scheme = "https"
				}
				if origin != scheme+"://"+r.Host && origin != "https://"+r.Host {
					writeError(w, forbidden())
					return
				}
			}
		}

		if r.Body != nil && r.Body != http.NoBody {
			limit := int64(1 << 20)
			if strings.HasSuffix(r.URL.Path, "/audio") {
				limit = 9 << 20
			}
			if r.ContentLength > limit {
				writeError(w, fail(413, "BODY_TOO_LARGE", "请求内容过大。"))
				return
			}
			controller := http.NewResponseController(w)
			_ = controller.SetReadDeadline(time.Now().Add(15 * time.Second))
			raw, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
			_ = r.Body.Close()
			_ = controller.SetReadDeadline(time.Time{})
			if err != nil {
				writeError(w, bad("读取请求超时或连接已中断。"))
				return
			}
			if int64(len(raw)) > limit {
				writeError(w, fail(413, "BODY_TOO_LARGE", "请求内容过大。"))
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(raw))
		}
		a.mu.Lock()
		var u *User
		if auth {
			user, err := a.authenticate(r)
			if err != nil {
				a.mu.Unlock()
				writeError(w, err)
				return
			}
			u = &user
		}
		var backup []byte
		var savedBindings map[string]binding
		var savedLastSent map[string]time.Time
		var savedHeaders http.Header
		a.jobs = nil
		a.rollbackJobs = nil
		if write {
			var err error
			backup, err = json.Marshal(a.state)
			if err != nil {
				slog.Error("snapshot state", "error", err)
				a.mu.Unlock()
				writeError(w, fail(500, "STORAGE_ERROR", "保存失败，请稍后重试。"))
				return
			}
			savedBindings = maps.Clone(a.bindings)
			savedLastSent = maps.Clone(a.lastSent)
			savedHeaders = w.Header().Clone()
		}
		result, err := fn(w, r, u)
		storageFailed := false
		if err == nil && write {
			if err = a.Save(); err != nil {
				slog.Error("persist state", "error", err)
				storageFailed = true
				err = fail(500, "STORAGE_ERROR", "保存失败，请稍后重试。")
			}
		}
		if err != nil && write {
			var restored State
			if restoreErr := json.Unmarshal(backup, &restored); restoreErr != nil {
				slog.Error("restore state", "error", restoreErr)
			} else {
				a.state = restored
			}
			a.bindings = savedBindings
			a.lastSent = savedLastSent
			if storageFailed {
				clear(w.Header())
				maps.Copy(w.Header(), savedHeaders)
			}
		}
		jobs := a.jobs
		rollbackJobs := a.rollbackJobs
		a.jobs = nil
		a.rollbackJobs = nil
		a.mu.Unlock()
		if err != nil {
			for i := len(rollbackJobs) - 1; i >= 0; i-- {
				rollbackJobs[i]()
			}
			writeError(w, err)
			return
		}
		for _, job := range jobs {
			job()
		}
		writeJSON(w, 200, result)
	}
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, err error) {
	var e *apiError
	if !errors.As(err, &e) {
		slog.Error("request failed", "error", err)
		e = &apiError{500, "INTERNAL_ERROR", "服务暂时不可用，请稍后重试。"}
	}
	writeJSON(w, e.Status, map[string]string{"code": e.Code, "message": e.Message})
}
func decode(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil {
		return bad("请求格式不正确。")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return bad("请求包含多余内容。")
	}
	return nil
}
func ok() any { return map[string]bool{"ok": true} }
func (a *App) limit(key string, max int, window time.Duration) bool {
	b := a.limits[key]
	if time.Now().After(b.Expires) {
		b = bucket{Expires: time.Now().Add(window)}
	}
	b.Count++
	a.limits[key] = b
	return b.Count <= max
}
func (a *App) audit(u *User, action, target string) {
	actor := "system"
	if u != nil {
		actor = u.Username
	}
	entry := Audit{id("a"), actor, action, target, now()}
	a.state.Audit[entry.ID] = entry
}
func (a *App) authenticate(r *http.Request) (User, error) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	c, err := a.verify(token)
	if err != nil {
		return User{}, fail(401, "UNAUTHORIZED", "登录已过期，请重新登录。")
	}
	u, exists := a.state.Users[c.Subject]
	if !exists || u.Status != "active" || u.TokenVersion != c.Version {
		return User{}, fail(401, "UNAUTHORIZED", "账号已失效，请重新登录。")
	}
	if u.Role != "user" && !c.Admin {
		return User{}, forbidden()
	}
	return u, nil
}
func (a *App) isAdmin(u *User) bool {
	return u != nil && (u.Role == "super_admin" || u.Role == "school_admin")
}
func (a *App) canClass(u *User, c Class) bool {
	if u == nil {
		return false
	}
	if u.Role == "super_admin" || u.Role == "school_admin" && u.OrgID == c.OrgID {
		return true
	}
	_, exists := a.state.Members[c.ID+"|"+u.ID]
	return exists
}
func (a *App) owns(u *User, c Class) bool {
	return u != nil && (c.OwnerID == u.ID || u.Role == "super_admin" || u.Role == "school_admin" && u.OrgID == c.OrgID)
}
func (a *App) getClass(u *User, classID string, owner bool) (Class, error) {
	c, exists := a.state.Classes[classID]
	if !exists {
		return c, missing()
	}
	if !a.canClass(u, c) || owner && !a.owns(u, c) {
		return c, forbidden()
	}
	return c, nil
}
func (a *App) licensed(c Class) bool {
	code, exists := a.state.Codes[c.ActivationCode]
	return exists && code.ClassID != nil && *code.ClassID == c.ID && code.Status == "active" && (code.ExpiresAt == nil || !elapsed(*code.ExpiresAt))
}
func (a *App) classView(c Class, u *User) Class {
	c.Role = "collaborator"
	if a.owns(u, c) {
		c.Role = "owner"
	}
	c.TeachersCount = 0
	c.DevicesCount = 0
	c.OnlineDevices = 0
	for _, m := range a.state.Members {
		if m.ClassID == c.ID {
			c.TeachersCount++
		}
	}
	for _, d := range a.state.Devices {
		if d.ClassID == c.ID {
			c.DevicesCount++
			if a.hub.DeviceOnline(d.ID) {
				c.OnlineDevices++
			}
		}
	}
	c.ActivationCode = ""
	return c
}
func (a *App) routes() {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "same-origin")
			next.ServeHTTP(w, r)
		})
	})
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok", "version": "0.1.0"})
	})
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "classding_online_devices %d\n", a.hub.DeviceCount())
	})
	r.Route("/api/v1", func(r chi.Router) {
		h := a.handle
		r.Get("/config", h(false, false, func(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
			return map[string]any{"speech_enabled": false, "registration_enabled": true, "version": "0.1.0"}, nil
		}))
		r.Post("/auth/register", h(false, true, a.register))
		r.Post("/auth/login", h(false, true, a.login))
		r.Post("/auth/refresh", h(false, true, a.refresh))
		r.Post("/auth/logout", h(false, true, a.logout))
		r.Get("/me", h(true, false, func(w http.ResponseWriter, r *http.Request, u *User) (any, error) { return publicUser(*u), nil }))
		r.Patch("/me", h(true, true, a.updateMe))
		r.Put("/me/password", h(true, true, a.changePassword))
		r.Get("/classes", h(true, false, a.listClasses))
		r.Post("/classes", h(true, true, a.createClass))
		r.Post("/classes/join", h(true, true, a.joinClass))
		r.Patch("/classes/{id}", h(true, true, a.updateClass))
		r.Delete("/classes/{id}", h(true, true, a.deleteClass))
		r.Get("/classes/{id}/members", h(true, false, a.members))
		r.Post("/classes/{id}/invites", h(true, true, a.invite))
		r.Delete("/classes/{id}/members/{uid}", h(true, true, a.removeMember))
		r.Post("/classes/{id}/transfer", h(true, true, a.transfer))
		r.Post("/classes/{id}/leave", h(true, true, a.leave))
		r.Get("/classes/{id}/devices", h(true, false, a.devices))
		r.Post("/classes/{id}/bind-code", h(true, false, a.bindCode))
		r.Post("/device/bind", h(false, true, a.bindDevice))
		r.Get("/device/me", h(false, false, a.deviceMe))
		r.Patch("/devices/{id}", h(true, true, a.renameDevice))
		r.Delete("/devices/{id}", h(true, true, a.removeDevice))
		r.Get("/messages", h(true, false, a.listMessages))
		r.Post("/messages", h(true, true, a.sendMessage))
		r.Get("/messages/{id}", h(true, false, a.getMessage))
		r.Post("/messages/{id}/recall", h(true, true, a.recallMessage))
		r.Post("/messages/{id}/resend", h(true, true, a.resendMessage))
		r.Get("/phrases", h(true, false, a.listPhrases))
		r.Post("/phrases", h(true, true, a.addPhrase))
		r.Patch("/phrases/{id}", h(true, true, a.editPhrase))
		r.Delete("/phrases/{id}", h(true, true, a.deletePhrase))
		r.Post("/audio", h(true, true, a.uploadAudio))
		r.Get("/audio/{id}", a.downloadAudio)
		for _, tool := range []string{"countdown", "rollcall", "stop"} {
			r.Post("/classes/{id}/tools/"+tool, h(true, true, a.tool))
		}
		r.Get("/client/latest", h(false, false, func(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
			return map[string]string{"version": "0.1.0", "url": "", "sha256": "", "notes": "尚未发布签名安装包", "min_supported": "0.1.0"}, nil
		}))
		r.Get("/admin/overview", h(true, false, a.overview))
		r.Get("/admin/activation-codes", h(true, false, a.listCodes))
		r.Post("/admin/activation-codes", h(true, true, a.createCodes))
		r.Patch("/admin/activation-codes/{code}", h(true, true, a.updateCode))
		r.Get("/admin/users", h(true, false, a.adminUsers))
		r.Patch("/admin/users/{id}", h(true, true, a.adminUpdateUser))
		r.Get("/admin/classes", h(true, false, a.adminClasses))
		r.Get("/admin/devices", h(true, false, a.adminDevices))
		r.Get("/admin/messages", h(true, true, a.adminMessages))
		r.Get("/admin/audit-logs", h(true, false, a.auditLogs))
	})
	r.Get("/ws/device", a.deviceWS)
	r.Get("/ws/teacher", a.teacherWS)
	a.router = r
}
func (a *App) maintenance() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.stop:
			return
		case <-ticker.C:
			a.mu.Lock()
			if err := a.maintenanceSweep(time.Now()); err != nil {
				slog.Error("maintenance save", "error", err)
			}
			a.mu.Unlock()
		}
	}
}

// maintenanceSweep is called with a.mu held. Durable changes are committed
// before deleting files or sending receipts.
func (a *App) maintenanceSweep(at time.Time) error {
	expired := func(value string) bool {
		stamp, err := time.Parse(time.RFC3339Nano, value)
		return err != nil || at.After(stamp)
	}
	for code, b := range a.bindings {
		if expired(b.Expires) {
			delete(a.bindings, code)
		}
	}
	for key, b := range a.limits {
		if at.After(b.Expires) {
			delete(a.limits, key)
		}
	}
	backup, err := json.Marshal(a.state)
	if err != nil {
		return fmt.Errorf("snapshot state: %w", err)
	}

	type pendingReceipt struct {
		message Message
		target  Target
	}
	var receipts []pendingReceipt
	var audioPaths []string
	for key, session := range a.state.Sessions {
		if expired(session.ExpiresAt) {
			delete(a.state.Sessions, key)
		}
	}
	for key, message := range a.state.Messages {
		created, _ := time.Parse(time.RFC3339Nano, message.CreatedAt)
		if at.Sub(created) > 90*24*time.Hour {
			delete(a.state.Messages, key)
			continue
		}
		if expired(message.ExpiresAt) {
			for i := range message.Targets {
				switch message.Targets[i].Status {
				case "queued", "delivered", "playing":
					message.Targets[i].Status = "expired"
					receipts = append(receipts, pendingReceipt{message, message.Targets[i]})
				}
			}
			a.state.Messages[key] = message
		}
	}
	for key, audio := range a.state.Audio {
		created, _ := time.Parse(time.RFC3339Nano, audio.CreatedAt)
		if at.Sub(created) > 7*24*time.Hour {
			audioPaths = append(audioPaths, audio.Path)
			delete(a.state.Audio, key)
		}
	}
	if err := a.Save(); err != nil {
		var restored State
		if restoreErr := json.Unmarshal(backup, &restored); restoreErr != nil {
			return fmt.Errorf("save state: %v; restore state: %w", err, restoreErr)
		}
		a.state = restored
		return err
	}
	for _, path := range audioPaths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			slog.Error("remove expired audio", "path", path, "error", err)
		}
	}
	for _, receipt := range receipts {
		a.receipt(receipt.message, receipt.target)
	}
	return nil
}

func (a *App) deferJob(fn func())      { a.jobs = append(a.jobs, fn) }
func (a *App) deferRollback(fn func()) { a.rollbackJobs = append(a.rollbackJobs, fn) }
