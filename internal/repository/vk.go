package repository

import (
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

// VK keeps its credentials and library separate from Spotify's tables.
type VKRepository struct {
	db *sqlx.DB
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
	err := r.db.Select(&songs, "SELECT * FROM vk_songs ORDER BY id")
	return songs, err
}

func (r *VKRepository) ReplaceAll(songs []Song) (int, error) {
	tx, err := r.db.Beginx()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM vk_songs"); err != nil {
		return 0, err
	}
	saved := 0
	for _, song := range songs {
		result, err := tx.Exec(`INSERT INTO vk_songs (artists, title, created_at)
			VALUES (?, ?, ?) ON CONFLICT (artists, title) DO NOTHING`, song.Artists, song.Title, song.CreatedAt)
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
