// Package app is the ClassDing service: accounts, classes, notifications,
// homework and the realtime channel to classroom screens.
package app

import (
	"classding/server/internal/repo"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Config struct {
	DataDir        string
	JWTSecret      string
	CookieSecure   string // "auto" (default), "true" or "false".
	TrustedProxies []string
	DevSeed        bool
	SendCooldown   time.Duration
}

// App owns the whole state. Every handler runs with mu held; work that must
// happen outside the lock, such as pushing frames, is queued with deferJob.
type App struct {
	mu     sync.Mutex
	state  State
	store  *repo.Store
	config Config
	router http.Handler
	hub    *Hub
	stop   chan struct{}

	// Short-lived data that is never persisted.
	bindings map[string]binding
	limits   map[string]bucket
	lastSent map[string]time.Time

	jobs         []func()
	rollbackJobs []func()
}

// binding is a six-digit code that pairs a classroom screen with a class.
type binding struct {
	ClassID string
	Expires string
}

// bucket counts attempts within a fixed window.
type bucket struct {
	Count   int
	Expires time.Time
}

func New(c Config) (*App, error) {
	if len(c.JWTSecret) < 32 {
		return nil, errors.New("JWT_SECRET must contain at least 32 characters")
	}
	switch c.CookieSecure {
	case "", "auto", "true", "false":
	default:
		return nil, errors.New("COOKIE_SECURE must be auto, true or false")
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
	a := &App{
		config:   c,
		state:    newState(),
		store:    store,
		hub:      NewHub(),
		stop:     make(chan struct{}),
		bindings: map[string]binding{},
		limits:   map[string]bucket{},
		lastSent: map[string]time.Time{},
	}
	if err = store.Load(context.Background(), &a.state); err != nil {
		store.Close()
		return nil, err
	}
	upgradeState(&a.state)
	if c.DevSeed {
		a.seed()
	}
	if err = a.Save(); err != nil {
		store.Close()
		return nil, err
	}
	a.routes()
	go a.maintenance()
	return a, nil
}

// upgradeState fills in data that databases written by older releases lack.
func upgradeState(s *State) {
	initializeSettings(s)
	if s.Schools == nil {
		s.Schools = map[string]School{}
	}
	// Before 0.3.2 a school was only an ID typed into forms. Give every ID that
	// stood for a school a record, named after the ID until the operator renames it.
	for _, u := range s.Users {
		if u.Role == RoleSchoolAdmin {
			s.ensureSchool(u.OrgID)
		}
	}
	for _, code := range s.Codes {
		if code.ClassID == nil && code.Status == LicenseUnused {
			s.ensureSchool(code.OrgID)
		}
	}
	// Licenses redeemed before they inherited their class organization.
	for key, code := range s.Codes {
		if code.OrgID == "" && code.ClassID != nil {
			if class, exists := s.Classes[*code.ClassID]; exists {
				code.OrgID = class.OrgID
				s.Codes[key] = code
			}
		}
	}
	// Devices created before they stored their own notification counter.
	for _, message := range s.Messages {
		for _, target := range message.Targets {
			if device, exists := s.Devices[target.DeviceID]; exists && target.Seq > device.Sequence {
				device.Sequence = target.Seq
				s.Devices[device.ID] = device
			}
		}
	}
}

func (a *App) Handler() http.Handler { return a.router }

func (a *App) Close() error {
	close(a.stop)
	a.hub.Close()
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.store.Close()
}

// Save writes the state through to the database. Callers hold a.mu.
func (a *App) Save() error { return a.store.Save(context.Background(), a.state) }

func (a *App) deferJob(fn func())      { a.jobs = append(a.jobs, fn) }
func (a *App) deferRollback(fn func()) { a.rollbackJobs = append(a.rollbackJobs, fn) }

// limit counts one attempt and reports whether it is still within max per window.
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
	entry := Audit{ID: id("a"), Actor: actor, Action: action, Target: target, At: now()}
	a.state.Audit[entry.ID] = entry
}

func (a *App) userByUsername(username string) (User, bool) {
	for _, u := range a.state.Users {
		if u.Username == username {
			return u, true
		}
	}
	return User{}, false
}

// revokeSessions signs the account out everywhere, including open sockets.
func (a *App) revokeSessions(u *User) {
	u.TokenVersion++
	for key, session := range a.state.Sessions {
		if session.UserID == u.ID {
			delete(a.state.Sessions, key)
		}
	}
	userID := u.ID
	a.deferJob(func() { a.hub.RemoveTeacher(userID) })
}
