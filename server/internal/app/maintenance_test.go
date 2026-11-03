package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMaintenanceRollbackAndRecovery(t *testing.T) {
	f := newSecurityFixture(t)
	when := time.Now().UTC()
	format := func(t time.Time) string { return t.Format(time.RFC3339Nano) }
	statuses := []string{"queued", "delivered", "playing", "played", "dismissed", "failed", "expired"}
	targets := make([]Target, len(statuses))
	for i, status := range statuses {
		targets[i] = Target{ClassID: "class_a", DeviceID: status, Status: status}
	}
	f.app.state.Messages["expire_me"] = Message{ID: "expire_me", CreatedAt: format(when.Add(-time.Hour)), ExpiresAt: format(when.Add(-time.Minute)), Targets: targets}
	f.app.state.Messages["old_message"] = Message{ID: "old_message", CreatedAt: format(when.Add(-91 * 24 * time.Hour)), ExpiresAt: format(when.Add(-time.Hour))}
	f.app.state.Sessions["expired_session"] = Session{ID: "expired_session", UserID: "owner_a", ExpiresAt: format(when.Add(-time.Minute))}
	file := filepath.Join(f.app.config.DataDir, "audio", "expired.webm")
	if err := os.WriteFile(file, []byte("recording"), 0600); err != nil {
		t.Fatal(err)
	}
	f.app.state.Audio["expired_audio"] = Audio{ID: "expired_audio", UserID: "owner_a", Path: file, MIME: "audio/webm", CreatedAt: format(when.Add(-8 * 24 * time.Hour))}
	f.app.bindings["expired"] = binding{Expires: format(when.Add(-time.Minute))}
	f.app.limits["expired"] = bucket{Expires: when.Add(-time.Minute)}

	peer := &Peer{ctx: context.Background(), queue: make(chan Frame, 8)}
	f.app.hub.teachers["owner_a"] = map[*Peer]bool{peer: true}
	defer func() {
		f.app.hub.mu.Lock()
		delete(f.app.hub.teachers, "owner_a")
		f.app.hub.mu.Unlock()
	}()
	restore := closeStoreForRollback(t, f.app)
	sweep := func() error {
		f.app.mu.Lock()
		defer f.app.mu.Unlock()
		return f.app.maintenanceSweep(when)
	}
	if err := sweep(); err == nil {
		t.Fatal("closed database should make maintenance fail")
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("failed maintenance removed recording: %v", err)
	}
	if _, exists := f.app.state.Audio["expired_audio"]; !exists {
		t.Fatal("failed maintenance removed recording metadata")
	}
	if _, exists := f.app.state.Sessions["expired_session"]; !exists {
		t.Fatal("failed maintenance removed a session")
	}
	if _, exists := f.app.state.Messages["old_message"]; !exists {
		t.Fatal("failed maintenance removed an old message")
	}
	for i, status := range statuses {
		if got := f.app.state.Messages["expire_me"].Targets[i].Status; got != status {
			t.Fatalf("failed maintenance changed target %d from %q to %q", i, status, got)
		}
	}
	if len(peer.queue) != 0 {
		t.Fatal("failed maintenance sent a receipt")
	}
	if _, exists := f.app.bindings["expired"]; exists {
		t.Fatal("expired in-memory binding was not cleaned")
	}
	if _, exists := f.app.limits["expired"]; exists {
		t.Fatal("expired in-memory limit was not cleaned")
	}

	restore()
	if err := sweep(); err != nil {
		t.Fatalf("maintenance after recovery: %v", err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("successful maintenance did not remove recording: %v", err)
	}
	if _, exists := f.app.state.Audio["expired_audio"]; exists {
		t.Fatal("successful maintenance retained recording metadata")
	}
	if _, exists := f.app.state.Sessions["expired_session"]; exists {
		t.Fatal("successful maintenance retained an expired session")
	}
	if _, exists := f.app.state.Messages["old_message"]; exists {
		t.Fatal("successful maintenance retained an old message")
	}
	want := []string{"expired", "expired", "expired", "played", "dismissed", "failed", "expired"}
	for i, status := range want {
		if got := f.app.state.Messages["expire_me"].Targets[i].Status; got != status {
			t.Fatalf("target %d status = %q, want %q", i, got, status)
		}
	}
	if len(peer.queue) != 3 {
		t.Fatalf("successful maintenance sent %d receipts, want 3", len(peer.queue))
	}
	for len(peer.queue) > 0 {
		frame := <-peer.queue
		data, ok := frame.D.(map[string]any)
		if !ok || frame.T != "receipt" || data["message_id"] != "expire_me" || data["status"] != "expired" {
			t.Fatalf("unexpected receipt: %+v", frame)
		}
	}
	var persisted State
	f.app.mu.Lock()
	err := f.app.store.Load(context.Background(), &persisted)
	f.app.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := persisted.Audio["expired_audio"]; exists {
		t.Fatal("recording metadata remains in the database")
	}
	for i, status := range want {
		if got := persisted.Messages["expire_me"].Targets[i].Status; got != status {
			t.Fatalf("persisted target %d status = %q, want %q", i, got, status)
		}
	}
}

func TestMaintenanceSkipsSaveWhenNothingExpired(t *testing.T) {
	f := newSecurityFixture(t)
	when := time.Now().UTC()
	format := func(t time.Time) string { return t.Format(time.RFC3339Nano) }
	f.app.state.Messages["fresh"] = Message{ID: "fresh", CreatedAt: format(when), ExpiresAt: format(when.Add(time.Hour)), Targets: []Target{{ClassID: "class_a", Status: "queued"}}}
	f.app.state.Messages["settled"] = Message{ID: "settled", CreatedAt: format(when.Add(-time.Hour)), ExpiresAt: format(when.Add(-time.Minute)), Targets: []Target{{ClassID: "class_a", Status: "played"}}}
	f.app.bindings["expired"] = binding{Expires: format(when.Add(-time.Minute))}
	restore := closeStoreForRollback(t, f.app)
	defer restore()
	f.app.mu.Lock()
	err := f.app.maintenanceSweep(when)
	f.app.mu.Unlock()
	if err != nil {
		t.Fatalf("maintenance without durable expirations should not write state: %v", err)
	}
	if _, exists := f.app.bindings["expired"]; exists {
		t.Fatal("expired in-memory binding was not cleaned")
	}
}
