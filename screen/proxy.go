package main

import (
	"crypto/rand"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// The web view talks to the server through a loopback proxy, so that the
// server address can change without reloading the page. An unguessable path
// prefix keeps other local programs out.

func (a *App) startProxy() error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	a.secret = rand.Text()
	a.proxyURL = "http://" + listener.Addr().String() + "/" + a.secret
	a.proxy = &http.Server{Handler: http.HandlerFunc(a.proxyHandler), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := a.proxy.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("local proxy: %v", err)
		}
	}()
	return nil
}

// parseServer accepts only the root of an HTTP or HTTPS site.
func parseServer(server string) (*url.URL, error) {
	invalid := errors.New("服务器地址无效")
	if strings.ContainsAny(server, "?#\\") {
		return nil, invalid
	}
	u, err := url.Parse(server)
	if err != nil || u.Hostname() == "" || u.Scheme != "http" && u.Scheme != "https" || u.User != nil ||
		u.Opaque != "" || u.RawPath != "" || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
		return nil, invalid
	}
	return u, nil
}

// ConfigureServer points the proxy at server, or disconnects it when server
// is empty, and returns the proxy's base URL.
func (a *App) ConfigureServer(server string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
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
	// CORS is scoped to the requesting origin, which is the embedded web view.
	if origin := r.Header.Get("Origin"); origin != "" {
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
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			// The server checks Origin against its own host on writes.
			if pr.In.Header.Get("Origin") != "" {
				pr.Out.Header.Set("Origin", target.Scheme+"://"+target.Host)
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del("Access-Control-Allow-Origin")
			resp.Header.Del("Access-Control-Allow-Credentials")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			http.Error(w, "连接服务器失败："+err.Error(), http.StatusBadGateway)
		},
	}
	proxy.ServeHTTP(w, r)
}
