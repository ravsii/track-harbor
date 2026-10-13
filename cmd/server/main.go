package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/ravsii/trackharbor/internal/config"
	"github.com/ravsii/trackharbor/internal/logger"
	"github.com/ravsii/trackharbor/internal/store"
	"github.com/ravsii/trackharbor/internal/web"
	"go.uber.org/zap"
)

func main() {
	if err := logger.Init("logs"); err != nil {
		fmt.Fprintln(os.Stderr, "initialize server logger:", err)
		os.Exit(1)
	}
	defer zap.L().Sync()

	ctx := context.Background()
	if err := run(ctx); err != nil {
		logger.Err(ctx, "server stopped", zap.Error(err))
	}
}

func run(ctx context.Context) error {
	cfg, dbPath, err := config.Load()
	if err != nil {
		// Config parsing may echo proxy URLs or credentials in an error.
		return errors.New("load server configuration failed; check config.yaml, the listen address and file permissions")
	}
	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open server database: %w", err)
	}
	defer db.Close()
	settings, err := db.InitDownloadSettings(store.DownloadSettings{Strategy: cfg.Strategy, PersonalProxy: cfg.PersonalProxy, PublicProxy: cfg.PublicProxy})
	if err != nil {
		return errors.New("load download settings failed; check the server database or legacy strategy in config.yaml")
	}
	if err := config.RemoveImportedSettings(cfg, settings.Strategy, settings.PersonalProxy, settings.PublicProxy); err != nil {
		return errors.New("move download settings from config.yaml failed; check file permissions or conflicting legacy values")
	}
	cfg.Strategy, cfg.PersonalProxy, cfg.PublicProxy = settings.Strategy, settings.PersonalProxy, settings.PublicProxy
	httpLog, err := zap.NewStdLogAt(zap.L(), zap.ErrorLevel)
	if err != nil {
		return fmt.Errorf("configure HTTP error logging: %w", err)
	}
	srv := &http.Server{
		Addr: cfg.Listen, Handler: web.New(db, zap.L(), cfg),
		ErrorLog:          httpLog,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second,
		WriteTimeout: 0, IdleTimeout: time.Minute, // SSE stays open while downloads run.
	}
	logger.Info(ctx, "starting local server", zap.String("address", cfg.Listen))
	if host, _, err := net.SplitHostPort(cfg.Listen); err == nil {
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			logger.Warn(ctx, "non-loopback HTTP bind: use HTTPS at a trusted reverse proxy and restrict direct access to this port")
		}
	}
	return srv.ListenAndServe()
}
