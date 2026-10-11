package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// VK keeps its credentials and library separate from Spotify's tables.
type VKRepository struct {
	db *sqlx.DB
}

type VKSong struct {
	Song
	OwnerID  int64  `db:"owner_id"`
	AudioID  int64  `db:"audio_id"`
	URL      string `db:"audio_url"`
	CoverURL string `db:"cover_url"`
}

func NewVKRepository(db *sqlx.DB) (*VKRepository, error) {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS vk_credentials (
		id INTEGER PRIMARY KEY CHECK (id = 1), access_token TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS vk_songs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		artists TEXT NOT NULL, title TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE (artists, title)
	)`)
	if err != nil {
		return nil, err
	}
	// Existing VK libraries predate audio URLs. Keep their tracks and request a
	// fresh sync before attempting direct downloads.
	for _, column := range []string{"owner_id INTEGER NOT NULL DEFAULT 0", "audio_id INTEGER NOT NULL DEFAULT 0", "audio_url TEXT NOT NULL DEFAULT ''", "cover_url TEXT NOT NULL DEFAULT ''"} {
		name := strings.Fields(column)[0]
		var present int
		rows, err := db.Queryx("PRAGMA table_info(vk_songs)")
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var cid, notNull, pk int
			var field, kind string
			var defaultValue sql.NullString
			if err = rows.Scan(&cid, &field, &kind, &notNull, &defaultValue, &pk); err != nil {
				break
			}
			if field == name {
				present = 1
			}
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return nil, err
		}
		if present == 0 {
			if _, err := db.Exec("ALTER TABLE vk_songs ADD COLUMN " + column); err != nil {
				return nil, fmt.Errorf("extend VK songs: %w", err)
			}
		}
	}
	return &VKRepository{db: db}, nil
}

func (r *VKRepository) SaveToken(token string) error {
	_, err := r.db.Exec(`INSERT INTO vk_credentials (id, access_token) VALUES (1, ?)
		ON CONFLICT (id) DO UPDATE SET access_token = excluded.access_token`, token)
	return err
}

func (r *VKRepository) LoadToken() (string, error) {
	var token string
	err := r.db.Get(&token, "SELECT access_token FROM vk_credentials WHERE id = 1")
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return token, err
}

func (r *VKRepository) GetAll() ([]Song, error) {
	var songs []Song
	err := r.db.Select(&songs, "SELECT id, artists, title, created_at FROM vk_songs ORDER BY id")
	return songs, err
}

func (r *VKRepository) GetAudio() ([]VKSong, error) {
	var songs []VKSong
	err := r.db.Select(&songs, "SELECT id, artists, title, created_at, owner_id, audio_id, audio_url, cover_url FROM vk_songs ORDER BY id")
	return songs, err
}

func (r *VKRepository) ReplaceAll(songs []VKSong) (int, error) {
	tx, err := r.db.Beginx()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var existing []VKSong
	if err := tx.Select(&existing, "SELECT artists, title, created_at, cover_url FROM vk_songs"); err != nil {
		return 0, err
	}
	addedAt := make(map[string]time.Time, len(existing))
	previousCovers := make(map[string]string, len(existing))
	for _, song := range existing {
		addedAt[song.Artists+"\x00"+song.Title] = song.CreatedAt
		previousCovers[song.Artists+"\x00"+song.Title] = song.CoverURL
	}
	if _, err := tx.Exec("DELETE FROM vk_songs"); err != nil {
		return 0, err
	}
	saved := 0
	for _, song := range songs {
		if song.CoverURL == "" {
			song.CoverURL = previousCovers[song.Artists+"\x00"+song.Title]
		}
		if song.CreatedAt.IsZero() {
			song.CreatedAt = addedAt[song.Artists+"\x00"+song.Title]
			if song.CreatedAt.IsZero() {
				song.CreatedAt = time.Now()
			}
		}
		result, err := tx.Exec(`INSERT INTO vk_songs (artists, title, created_at, owner_id, audio_id, audio_url, cover_url)
			VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT (artists, title) DO NOTHING`, song.Artists, song.Title, song.CreatedAt, song.OwnerID, song.AudioID, song.URL, song.CoverURL)
		if err != nil {
			return 0, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		saved += int(n)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return saved, nil
}
