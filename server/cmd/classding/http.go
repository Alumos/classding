package main

import (
	"bytes"
	"classding/server/internal/app"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

func newHandler(service *app.App, assets fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/") || r.URL.Path == "/healthz" || r.URL.Path == "/metrics" {
			service.Handler().ServeHTTP(w, r)
			return
		}
		root := "teacher"
		relative := strings.TrimPrefix(r.URL.Path, "/")
		if r.URL.Path == "/admin" || strings.HasPrefix(r.URL.Path, "/admin/") {
			root = "admin"
			relative = strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/admin"), "/")
		}
		files, err := fs.Sub(assets, root)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		clean := path.Clean("/" + relative)[1:]
		if clean == "" {
			clean = "index.html"
		}
		if _, err = fs.Stat(files, clean); err != nil {
			if strings.Contains(path.Base(clean), ".") {
				http.NotFound(w, r)
				return
			}
			clean = "index.html"
		}
		if strings.HasPrefix(clean, "assets/") {
			w.Header().Set("Cache-Control", "public,max-age=31536000,immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		if clean == "index.html" {
			content, err := fs.ReadFile(files, clean)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			// FileServer redirects /index.html to ./, which would loop after an SPA rewrite.
			http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(content))
			return
		}
		clone := r.Clone(r.Context())
		clone.URL.Path = "/" + clean
		http.FileServer(http.FS(files)).ServeHTTP(w, clone)
	})
}
