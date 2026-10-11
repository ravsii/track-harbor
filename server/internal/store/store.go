package store

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

type Track struct {
	Source, Artist, Title string
	AddedAt               int64 // Unix seconds; zero means the source did not provide a date.
}
type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
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
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS credentials (source TEXT PRIMARY KEY, first TEXT NOT NULL, second TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS tracks (source TEXT NOT NULL, artist TEXT NOT NULL, title TEXT NOT NULL, added_at INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(source, artist, title));`)
	if err != nil {
		db.Close()
		return nil, err
	}
	var hasAddedAt bool
	rows, err := db.Query("PRAGMA table_info(tracks)")
	if err != nil {
		db.Close()
		return nil, err
	}
	for rows.Next() {
		var cid, notNull, pk int
		var name, kind string
		var defaultValue sql.NullString
		if err = rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk); err != nil {
			break
		}
		if name == "added_at" {
			hasAddedAt = true
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		db.Close()
		return nil, err
	}
	if !hasAddedAt {
		if _, err := db.Exec("ALTER TABLE tracks ADD COLUMN added_at INTEGER NOT NULL DEFAULT 0"); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) SaveCredentials(source, first, second string) error {
	if source != "spotify" && source != "vk" {
		return fmt.Errorf("unknown source")
	}
	_, err := s.db.Exec(`INSERT INTO credentials(source, first, second) VALUES(?, ?, ?)
		ON CONFLICT(source) DO UPDATE SET first=excluded.first, second=excluded.second`, source, first, second)
	return err
}

func (s *Store) Credentials(source string) (string, string, error) {
	var first, second string
	err := s.db.QueryRow(`SELECT first, second FROM credentials WHERE source=?`, source).Scan(&first, &second)
	return first, second, err
}

func (s *Store) Connected(source string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM credentials WHERE source=?`, source).Scan(&n)
	return n > 0, err
}

func (s *Store) List(source string) ([]Track, error) {
	rows, err := s.db.Query(`SELECT source, artist, title, added_at FROM tracks WHERE source=? ORDER BY added_at DESC, artist, title`, source)
	if err != nil {
		return nil, err
	}
	return scanTracks(rows)
}

func (s *Store) ListAll() ([]Track, error) {
	rows, err := s.db.Query(`SELECT source, artist, title, added_at FROM tracks ORDER BY added_at DESC, source, artist, title`)
	if err != nil {
		return nil, err
	}
	return scanTracks(rows)
}

func scanTracks(rows *sql.Rows) ([]Track, error) {
	defer rows.Close()
	var tracks []Track
	for rows.Next() {
		var track Track
		if err := rows.Scan(&track.Source, &track.Artist, &track.Title, &track.AddedAt); err != nil {
			return nil, err
		}
		tracks = append(tracks, track)
	}
	return tracks, rows.Err()
}

func (s *Store) Replace(source string, tracks []Track) (int, error) {
	if source != "spotify" && source != "vk" {
		return 0, fmt.Errorf("unknown source")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	previous := make(map[string]int64)
	if source == "vk" || source == "spotify" {
		rows, err := tx.Query(`SELECT artist, title, added_at FROM tracks WHERE source=?`, source)
		if err != nil {
			return 0, err
		}
		for rows.Next() {
			var artist, title string
			var addedAt int64
			if err = rows.Scan(&artist, &title, &addedAt); err != nil {
				break
			}
			if addedAt != 0 {
				previous[artist+"\x00"+title] = addedAt
			}
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return 0, err
		}
	}
	if _, err := tx.Exec(`DELETE FROM tracks WHERE source=?`, source); err != nil {
		return 0, err
	}
	count := 0
	for _, track := range tracks {
		if addedAt, ok := previous[track.Artist+"\x00"+track.Title]; ok && (source == "vk" || track.AddedAt == 0) {
			track.AddedAt = addedAt
		}
		result, err := tx.Exec(`INSERT OR IGNORE INTO tracks(source, artist, title, added_at) VALUES(?, ?, ?, ?)`, source, track.Artist, track.Title, track.AddedAt)
		if err != nil {
			return 0, err
		}
		if n, err := result.RowsAffected(); err == nil {
			count += int(n)
		} else {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}
