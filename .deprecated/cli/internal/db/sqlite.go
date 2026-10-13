package db

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/ravsii/trackharbor/cli/internal/config"
)

func New() (*sqlx.DB, error) {
	if _, err := config.Load(); err != nil {
		return nil, err
	}
	path, err := config.DatabasePath()
	if err != nil {
		return nil, err
	}
	if err := migrateLegacy(path); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0600); err != nil {
		return nil, err
	}
	uriPath := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" && uriPath[0] != '/' {
		uriPath = "/" + uriPath
	}
	dsn := (&url.URL{Scheme: "file", Path: uriPath}).String()
	db, err := sqlx.Connect("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	return db, nil
}

// Snapshot the old database, including committed WAL contents. Never overwrite
// an existing vault and leave the original intact as a backup.
func migrateLegacy(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat("meshtune.db"); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	legacy, err := sqlx.Connect("sqlite3", "file:meshtune.db?mode=ro")
	if err != nil {
		return fmt.Errorf("open legacy database: %w", err)
	}
	defer legacy.Close()
	tmp, err := os.CreateTemp(filepath.Dir(path), "vault-migration-*.db")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Close(); err != nil {
		return err
	}
	if _, err := legacy.Exec("VACUUM INTO ?", tmpPath); err != nil {
		return fmt.Errorf("migrate meshtune.db to %s: %w", path, err)
	}
	if err := os.Link(tmpPath, path); err != nil && !os.IsExist(err) {
		return fmt.Errorf("publish migrated database: %w", err)
	}
	return nil
}
