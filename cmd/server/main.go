package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ravsii/trackharbor/internal/config"
	"github.com/ravsii/trackharbor/internal/logger"
	"github.com/ravsii/trackharbor/internal/store"
	"github.com/ravsii/trackharbor/internal/web"
	"go.uber.org/zap"
)

func main() {
	migrateDB := flag.Bool("migrate-db", false, "Apply database migrations and exit without starting the server")
	flag.Parse()
	if err := logger.Init("logs"); err != nil {
		fmt.Fprintln(os.Stderr, "initialize server logger:", err)
		os.Exit(1)
	}
	defer zap.L().Sync()

	ctx := context.Background()
	var err error
	if *migrateDB {
		err = migrateDatabase(ctx)
	} else {
		err = run(ctx)
	}
	if err != nil {
		logger.Err(ctx, "server stopped", zap.Error(err))
		_ = zap.L().Sync()
		os.Exit(1)
	}
}

func migrateDatabase(ctx context.Context) error {
	dbPath, err := config.DatabasePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return fmt.Errorf("create server database directory: %w", err)
	}
	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("migrate server database: %w", err)
	}
	if err := db.Close(); err != nil {
		return fmt.Errorf("close server database: %w", err)
	}
	logger.Info(ctx, "database migrations completed")
	return nil
}

func run(ctx context.Context) error {
	cfg, dbPath, err := config.Load()
	if err != nil {
		if errors.Is(err, config.ErrJWTSigningKey) {
			return err
		}
		// Config parsing may echo proxy URLs or credentials in an error.
		return errors.New("load server configuration failed; check config.yaml, the listen address and file permissions")
	}
	db, err := store.Open(dbPath, cfg.DownloadDir)
	if err != nil {
		return fmt.Errorf("open server database: %w", err)
	}
	defer db.Close()
	settings, err := db.InitDownloadSettings(store.DownloadSettings{PersonalProxy: cfg.PersonalProxy, PublicProxy: cfg.PublicProxy})
	if err != nil {
		return errors.New("load download proxies failed; check the server database and legacy proxies in config.yaml")
	}
	if err := config.RemoveImportedSettings(cfg); err != nil {
		return errors.New("move download settings from config.yaml failed; check file permissions or conflicting legacy values")
	}
	cfg.PersonalProxy, cfg.PublicProxy = settings.PersonalProxy, settings.PublicProxy
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
