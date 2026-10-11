package store

import (
	"database/sql"
	"embed"
	"fmt"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Server migrations are applied automatically and atomically on startup.
// SQLite's user_version belongs only to the server's independent database.
func migrate(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current int
	if err := tx.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	latest := 0
	for _, entry := range entries {
		name := entry.Name()
		prefix, _, ok := strings.Cut(name, "_")
		version, err := strconv.Atoi(prefix)
		if !ok || err != nil || version != latest+1 || !strings.HasSuffix(name, ".sql") {
			return fmt.Errorf("invalid server migration %q", name)
		}
		latest = version
		if version <= current {
			continue
		}
		contents, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(contents)); err != nil {
			return fmt.Errorf("apply server migration %s: %w", name, err)
		}
		if version == 1 {
			if err := upgradeLegacyTracks(tx); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
			return err
		}
	}
	if current > latest {
		return fmt.Errorf("server database version %d is newer than this server (%d)", current, latest)
	}
	return tx.Commit()
}

// Existing server databases may predate the full tracks schema. Preserve
// their rows while bringing them to the versioned initial schema.
func upgradeLegacyTracks(tx *sql.Tx) error {
	rows, err := tx.Query("PRAGMA table_info(tracks)")
	if err != nil {
		return err
	}
	columns := make(map[string]bool)
	for rows.Next() {
		var cid, notNull, pk int
		var name, kind string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk); err != nil {
			rows.Close()
			return err
		}
		columns[name] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, column := range []struct{ name, definition string }{
		{"added_at", "INTEGER NOT NULL DEFAULT 0"},
		{"owner_id", "INTEGER NOT NULL DEFAULT 0"},
		{"audio_id", "INTEGER NOT NULL DEFAULT 0"},
		{"audio_url", "TEXT NOT NULL DEFAULT ''"},
		{"cover_url", "TEXT NOT NULL DEFAULT ''"},
	} {
		if !columns[column.name] {
			if _, err := tx.Exec("ALTER TABLE tracks ADD COLUMN " + column.name + " " + column.definition); err != nil {
				return fmt.Errorf("upgrade server tracks column %s: %w", column.name, err)
			}
		}
	}
	return nil
}
