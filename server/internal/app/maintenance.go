package app

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"time"
)

const (
	messageRetention = 90 * 24 * time.Hour
	audioRetention   = 7 * 24 * time.Hour
)

func (a *App) maintenance() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.stop:
			return
		case at := <-ticker.C:
			a.mu.Lock()
			if err := a.maintenanceSweep(at); err != nil {
				slog.Error("maintenance save", "error", err)
			}
			a.mu.Unlock()
		}
	}
}

// expiry lists what a sweep removes or expires at one moment.
type expiry struct {
	sessions []string
	messages []string // Past retention; deleted.
	stale    []string // Expired, with deliveries still pending.
	audio    []string
}

func (e expiry) empty() bool {
	return len(e.sessions) == 0 && len(e.messages) == 0 && len(e.stale) == 0 && len(e.audio) == 0
}

func (a *App) expiredAt(at time.Time) expiry {
	past := func(value string) bool {
		stamp, ok := parseTime(value)
		return !ok || at.After(stamp)
	}
	older := func(value string, age time.Duration) bool {
		created, _ := parseTime(value)
		return at.Sub(created) > age
	}
	var e expiry
	for key, session := range a.state.Sessions {
		if past(session.ExpiresAt) || session.RotationUntil != "" && past(session.RotationUntil) {
			e.sessions = append(e.sessions, key)
		}
	}
	for key, message := range a.state.Messages {
		if older(message.CreatedAt, messageRetention) {
			e.messages = append(e.messages, key)
			continue
		}
		if !past(message.ExpiresAt) {
			continue
		}
		if slices.ContainsFunc(message.Targets, func(t Target) bool { return t.Status.Pending() }) {
			e.stale = append(e.stale, key)
		}
	}
	for key, audio := range a.state.Audio {
		if older(audio.CreatedAt, audioRetention) {
			e.audio = append(e.audio, key)
		}
	}
	return e
}

// maintenanceSweep runs with a.mu held. It forgets expired codes and limits,
// then removes expired records. Durable changes are committed before files are
// deleted or receipts are sent, and nothing is written when nothing expired.
func (a *App) maintenanceSweep(at time.Time) error {
	for code, b := range a.bindings {
		if stamp, ok := parseTime(b.Expires); !ok || at.After(stamp) {
			delete(a.bindings, code)
		}
	}
	for key, b := range a.limits {
		if at.After(b.Expires) {
			delete(a.limits, key)
		}
	}
	e := a.expiredAt(at)
	if e.empty() {
		return nil
	}
	backup, err := json.Marshal(a.state)
	if err != nil {
		return fmt.Errorf("snapshot state: %w", err)
	}

	for _, key := range e.sessions {
		delete(a.state.Sessions, key)
	}
	for _, key := range e.messages {
		delete(a.state.Messages, key)
	}
	var receipts []func()
	for _, key := range e.stale {
		message := a.state.Messages[key]
		for i := range message.Targets {
			if message.Targets[i].Status.Pending() {
				message.Targets[i].Status = DeliveryExpired
				target := message.Targets[i]
				receipts = append(receipts, func() { a.receipt(message, target) })
			}
		}
		a.state.Messages[key] = message
	}
	var files []string
	for _, key := range e.audio {
		files = append(files, a.state.Audio[key].Path)
		delete(a.state.Audio, key)
	}

	if err := a.Save(); err != nil {
		var restored State
		if restoreErr := json.Unmarshal(backup, &restored); restoreErr != nil {
			return fmt.Errorf("save state: %v; restore state: %w", err, restoreErr)
		}
		a.state = restored
		return err
	}
	for _, path := range files {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			slog.Error("remove expired audio", "path", path, "error", err)
		}
	}
	for _, send := range receipts {
		send()
	}
	return nil
}
