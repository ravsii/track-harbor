package store

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

type Track struct{ Artist, Title string }
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
		CREATE TABLE IF NOT EXISTS tracks (source TEXT NOT NULL, artist TEXT NOT NULL, title TEXT NOT NULL, PRIMARY KEY(source, artist, title));`)
	if err != nil {
		db.Close()
		return nil, err
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
	rows, err := s.db.Query(`SELECT artist, title FROM tracks WHERE source=? ORDER BY artist, title`, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tracks []Track
	for rows.Next() {
		var track Track
		if err := rows.Scan(&track.Artist, &track.Title); err != nil {
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
	if _, err := tx.Exec(`DELETE FROM tracks WHERE source=?`, source); err != nil {
		return 0, err
	}
	count := 0
	for _, track := range tracks {
		result, err := tx.Exec(`INSERT OR IGNORE INTO tracks(source, artist, title) VALUES(?, ?, ?)`, source, track.Artist, track.Title)
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
