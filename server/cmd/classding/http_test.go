package main

import (
	"classding/server/internal/app"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedSitesServeWithoutRedirectLoops(t *testing.T) {
	service, err := app.New(app.Config{DataDir: t.TempDir(), JWTSecret: strings.Repeat("k", 40)})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	files := fstest.MapFS{
		"teacher/index.html":  {Data: []byte("<!doctype html><title>teacher</title>")},
		"admin/index.html":    {Data: []byte("<!doctype html><title>admin</title>")},
		"admin/assets/app.js": {Data: []byte("console.log('admin')")},
	}
	handler := newHandler(service, files)
	for _, tc := range []struct {
		path, body string
		status     int
	}{
		{"/", "teacher", 200}, {"/send", "teacher", 200}, {"/index.html", "teacher", 200},
		{"/admin", "admin", 200}, {"/admin/", "admin", 200}, {"/admin/classes", "admin", 200},
		{"/admin/assets/app.js", "console.log", 200}, {"/admin/assets/missing.js", "404", 404},
		{"/api/v1/config", "speech_enabled", 200},
	} {
		t.Run(tc.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if recorder.Code != tc.status || !strings.Contains(recorder.Body.String(), tc.body) {
				t.Fatalf("unexpected response %d %q", recorder.Code, recorder.Body.String())
			}
			if location := recorder.Header().Get("Location"); location != "" {
				t.Fatalf("unexpected redirect to %q", location)
			}
			if strings.Contains(tc.path, "/assets/app.js") && !strings.Contains(recorder.Header().Get("Cache-Control"), "immutable") {
				t.Fatal("hashed asset should be immutable")
			}
		})
	}
}
