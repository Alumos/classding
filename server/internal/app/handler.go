package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"time"
)

// endpoint is a request handler that runs while App.mu is held. A command's
// changes are saved when it returns nil and rolled back when it returns an error.
type endpoint func(http.ResponseWriter, *http.Request, *User) (any, error)

// access is the least privilege an endpoint requires.
type access int

const (
	public access = iota
	signedIn
	adminOnly
	superAdminOnly
)

const (
	maxBodyBytes  = 1 << 20
	maxAudioBytes = 8 << 20
)

type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e *apiError) Error() string { return e.Message }

func fail(status int, code, message string) error { return &apiError{status, code, message} }
func bad(message string) error                    { return fail(400, "INVALID_INPUT", message) }
func unauthorized(message string) error           { return fail(401, "UNAUTHORIZED", message) }
func forbidden() error                            { return fail(403, "FORBIDDEN", "你没有权限执行此操作。") }
func missing() error                              { return fail(404, "NOT_FOUND", "请求的内容不存在或已被移除。") }
func tooMany(message string) error                { return fail(429, "RATE_LIMITED", message) }
func licenseInactive() error {
	return fail(403, "LICENSE_INACTIVE", "班级授权已失效，请联系管理员。")
}

func deviceRevoked(message string) error { return fail(401, "DEVICE_REVOKED", message) }
func storageFailed() error               { return fail(500, "STORAGE_ERROR", "保存失败，请稍后重试。") }
func tooLarge(message string) error      { return fail(413, "BODY_TOO_LARGE", message) }

func ok() any { return map[string]bool{"ok": true} }

func (a *App) query(level access, fn endpoint) http.HandlerFunc   { return a.handle(level, false, fn) }
func (a *App) command(level access, fn endpoint) http.HandlerFunc { return a.handle(level, true, fn) }

func (a *App) handle(level access, persist bool, fn endpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !validOrigin(r) {
			writeError(w, forbidden())
			return
		}
		if err := bufferBody(w, r); err != nil {
			writeError(w, err)
			return
		}

		a.mu.Lock()
		user, err := a.authorize(r, level)
		if err != nil {
			a.mu.Unlock()
			writeError(w, err)
			return
		}
		var tx *transaction
		if persist {
			if tx, err = a.begin(w); err != nil {
				a.mu.Unlock()
				writeError(w, err)
				return
			}
		}
		a.jobs, a.rollbackJobs = nil, nil
		result, err := fn(w, r, user)
		if tx != nil {
			err = tx.finish(err)
		}
		jobs, rollbackJobs := a.jobs, a.rollbackJobs
		a.jobs, a.rollbackJobs = nil, nil
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

// authorize resolves the signed-in account and checks it against the level.
func (a *App) authorize(r *http.Request, level access) (*User, error) {
	if level == public {
		return nil, nil
	}
	u, err := a.authenticate(r)
	if err != nil {
		return nil, err
	}
	if level == adminOnly && !u.IsAdmin() || level == superAdminOnly && !u.IsSuperAdmin() {
		return nil, forbidden()
	}
	return &u, nil
}

func (a *App) authenticate(r *http.Request) (User, error) {
	c, err := a.verify(bearerToken(r))
	if err != nil {
		return User{}, unauthorized("登录已过期，请重新登录。")
	}
	u, exists := a.state.Users[c.Subject]
	if !exists || !u.Active() || u.TokenVersion != c.Version {
		return User{}, unauthorized("账号已失效，请重新登录。")
	}
	return u, nil
}

func bearerToken(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

// transaction remembers everything a command may change so that a failed
// command, or a failed save, leaves no trace.
type transaction struct {
	app      *App
	w        http.ResponseWriter
	state    []byte
	bindings map[string]binding
	lastSent map[string]time.Time
	headers  http.Header
}

func (a *App) begin(w http.ResponseWriter) (*transaction, error) {
	state, err := json.Marshal(a.state)
	if err != nil {
		slog.Error("snapshot state", "error", err)
		return nil, storageFailed()
	}
	return &transaction{a, w, state, maps.Clone(a.bindings), maps.Clone(a.lastSent), w.Header().Clone()}, nil
}

// finish commits the command's changes, or restores the snapshot on failure.
func (tx *transaction) finish(err error) error {
	a := tx.app
	if err == nil {
		if err = a.Save(); err == nil {
			return nil
		}
		slog.Error("persist state", "error", err)
		err = storageFailed()
		// Cookies set by the command must not survive a failed save.
		clear(tx.w.Header())
		maps.Copy(tx.w.Header(), tx.headers)
	}
	var restored State
	if restoreErr := json.Unmarshal(tx.state, &restored); restoreErr != nil {
		slog.Error("restore state", "error", restoreErr)
	} else {
		a.state = restored
	}
	a.bindings, a.lastSent = tx.bindings, tx.lastSent
	return err
}

// bufferBody reads the whole request body up front, under a deadline, so that a
// slow client never holds App.mu.
func bufferBody(w http.ResponseWriter, r *http.Request) error {
	if r.Body == nil || r.Body == http.NoBody {
		return nil
	}
	limit := int64(maxBodyBytes)
	if strings.HasSuffix(r.URL.Path, "/audio") {
		limit = maxAudioBytes + maxBodyBytes
	}
	if r.ContentLength > limit {
		return tooLarge("请求内容过大。")
	}
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(15 * time.Second))
	raw, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	_ = r.Body.Close()
	_ = controller.SetReadDeadline(time.Time{})
	if err != nil {
		return bad("读取请求超时或连接已中断。")
	}
	if int64(len(raw)) > limit {
		return tooLarge("请求内容过大。")
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	return nil
}

// validOrigin rejects cross-site writes; requests without an Origin header come
// from native clients and scripts.
func validOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return origin == scheme+"://"+r.Host || origin == "https://"+r.Host
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

// decode reads exactly one JSON value into dst.
func decode(r *http.Request, dst any) error { return decodeJSON(r, dst, false) }

// decodeStrict is decode that also rejects fields dst does not declare.
func decodeStrict(r *http.Request, dst any) error { return decodeJSON(r, dst, true) }

func decodeJSON(r *http.Request, dst any, strict bool) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes))
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(dst); err != nil {
		return bad("请求格式不正确。")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return bad("请求包含多余内容。")
	}
	return nil
}
