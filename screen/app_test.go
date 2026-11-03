package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProbeServerKeepsActiveTargetAndRejectsRedirect(t *testing.T) {
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	app := &App{target: &url.URL{Scheme: "https", Host: "old.example"}}
	calls := 0
	verifyRequest := func(r *http.Request) {
		t.Helper()
		calls++
		if r.Method != http.MethodPost || r.URL.String() != "https://new.example/api/v1/device/migration-check" || r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected probe: %s %s, authorization %q, content type %q", r.Method, r.URL, r.Header.Get("Authorization"), r.Header.Get("Content-Type"))
		}
		var body struct {
			MinSequence int64    `json:"min_sequence"`
			PendingIDs  []string `json:"pending_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.MinSequence != 42 || !reflect.DeepEqual(body.PendingIDs, []string{"m1", "m2"}) {
			t.Fatalf("unexpected probe body: %+v, %v", body, err)
		}
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 5*time.Second {
			t.Fatal("probe must have a bounded timeout")
		}
	}
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		verifyRequest(r)
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"https://other.example/steal"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})
	if _, err := app.ProbeServer("https://new.example", "secret", 42, []string{"m1", "m2"}); err == nil {
		t.Fatal("redirect must fail")
	}
	if calls != 1 {
		t.Fatalf("redirect was followed: %d requests", calls)
	}
	if app.target.Host != "old.example" {
		t.Fatalf("probe changed active target: %s", app.target)
	}

	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		verifyRequest(r)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"device":{"id":"d1","class_id":"c1","name":"黑板"},"sequence":42,"notifications":[{"t":"notify","seq":42,"d":{"message_id":"m1","audio_url":"/audio?s=1"}}],"recalled_ids":["m2"]}`)), Request: r}, nil
	})
	got, err := app.ProbeServer("https://new.example/", "secret", 42, []string{"m1", "m2"})
	if err != nil || got.Device.ID != "d1" || got.Device.ClassID != "c1" || got.Sequence != 42 || !reflect.DeepEqual(got.RecalledIDs, []string{"m2"}) || len(got.Notifications) != 1 {
		t.Fatalf("valid migration result: %+v, %v", got, err)
	}
	var frame struct {
		Type string `json:"t"`
		Seq  int64  `json:"seq"`
		Data struct {
			MessageID string `json:"message_id"`
			AudioURL  string `json:"audio_url"`
		} `json:"d"`
	}
	if err := json.Unmarshal(got.Notifications[0], &frame); err != nil || frame.Type != "notify" || frame.Seq != 42 || frame.Data.MessageID != "m1" || frame.Data.AudioURL != "/audio?s=1" {
		t.Fatalf("notification frame was not preserved: %+v, %v", frame, err)
	}
	if app.target.Host != "old.example" {
		t.Fatalf("successful probe changed active target: %s", app.target)
	}
}

func TestServerURLValidation(t *testing.T) {
	for _, value := range []string{"http://example.com", "https://example.com:443/", "http://[::1]:8080"} {
		if _, err := parseServer(value); err != nil {
			t.Errorf("valid URL %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{"", "example.com", "ftp://example.com", "https://user:pass@example.com", "https://example.com/path", "https://example.com/?", "https://example.com/#", "https://example.com/%2f", "https://example.com\\evil"} {
		if _, err := parseServer(value); err == nil {
			t.Errorf("invalid URL %q accepted", value)
		}
	}
}
