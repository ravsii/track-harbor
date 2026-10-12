package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"trackharbor/internal/config"
	"trackharbor/internal/store"
	"trackharbor/internal/web"
)

func main() {
	logging := zap.NewProductionConfig()
	logging.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	logging.Sampling = nil
	logging.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	logger, err := logging.Build()
	if err != nil {
		fmt.Fprintln(os.Stderr, "initialize server logger:", err)
		os.Exit(1)
	}
	if err := run(logger); err != nil {
		logger.Error("server stopped", zap.Error(err))
		_ = logger.Sync()
		os.Exit(1)
	}
	_ = logger.Sync()
}

func run(logger *zap.Logger) error {
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
	httpLog, err := zap.NewStdLogAt(logger, zap.ErrorLevel)
	if err != nil {
		return fmt.Errorf("configure HTTP error logging: %w", err)
	}
	srv := &http.Server{
		Addr: cfg.Listen, Handler: web.New(db, logger, cfg),
		ErrorLog:          httpLog,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second,
		WriteTimeout: 0, IdleTimeout: time.Minute, // SSE stays open while downloads run.
	}
	logger.Info("starting local server", zap.String("address", cfg.Listen))
	if host, _, err := net.SplitHostPort(cfg.Listen); err == nil {
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			logger.Warn("non-loopback HTTP bind: use HTTPS at a trusted reverse proxy and restrict direct access to this port")
		}
	}
	return srv.ListenAndServe()
}
