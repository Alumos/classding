package main

import (
	"classding/server/internal/app"
	"classding/server/static"
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	dev := os.Getenv("DEV_SEED") == "1"
	secret := os.Getenv("JWT_SECRET")
	if secret == "" && dev {
		secret = "development-only-change-me-classding-2026"
	}
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}
	service, err := app.New(app.Config{DataDir: dataDir, JWTSecret: secret, AdminTOTPSecret: os.Getenv("ADMIN_TOTP_SECRET"), TrustedProxies: strings.FieldsFunc(os.Getenv("TRUSTED_PROXY_CIDRS"), func(r rune) bool { return r == ',' || r == ' ' }), DevSeed: dev, SecureCookies: !dev, SendCooldown: 5 * time.Second})
	if err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
	defer service.Close()
	if username := os.Getenv("BOOTSTRAP_ADMIN_USERNAME"); username != "" {
		if err := service.BootstrapAdmin(username, os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"), os.Getenv("BOOTSTRAP_ADMIN_NAME")); err != nil {
			slog.Error("bootstrap admin failed", "error", err)
			os.Exit(1)
		}
	}
	handler := newHandler(service, static.Files)
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	server := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 20 * time.Second, MaxHeaderBytes: 32 << 10, IdleTimeout: 90 * time.Second}
	go func() {
		slog.Info("ClassDing ready", "addr", addr, "development_seed", dev)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server", "error", err)
			os.Exit(1)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
