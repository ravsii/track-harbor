package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

type SpotifyHeaders struct {
	Authorization string `db:"authorization"`
	ClientToken   string `db:"client_token"`
}

type SpotifyHeadersRepository struct {
	db *sqlx.DB
}

func NewSpotifyHeadersRepository(db *sqlx.DB) *SpotifyHeadersRepository {
	return &SpotifyHeadersRepository{db: db}
}

func (r *SpotifyHeadersRepository) Load() (*SpotifyHeaders, error) {
	// Bootstrap the cache for existing installations without requiring a
	// manual migration before the first sync with saved headers.
	_, err := r.db.Exec(`CREATE TABLE IF NOT EXISTS spotify_headers (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		authorization TEXT NOT NULL,
		client_token TEXT NOT NULL
	)`)
	if err != nil {
		return nil, err
	}
	var headers SpotifyHeaders
	err = r.db.Get(&headers, "SELECT authorization, client_token FROM spotify_headers WHERE id = 1")
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(headers.Authorization) == "" || strings.TrimSpace(headers.ClientToken) == "" {
		return nil, nil
	}
	return &headers, nil
}

func (r *SpotifyHeadersRepository) Save(headers SpotifyHeaders) error {
	_, err := r.db.Exec(`INSERT INTO spotify_headers (id, authorization, client_token)
		VALUES (1, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			authorization = excluded.authorization,
			client_token = excluded.client_token`, headers.Authorization, headers.ClientToken)
	if err != nil {
		return fmt.Errorf("запись заголовков: %w", err)
	}
	return nil
}
