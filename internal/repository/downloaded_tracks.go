package repository

import (
	"database/sql"
	"errors"
	"sync"

	"github.com/jmoiron/sqlx"
)

// DownloadedTracksRepository is independent of songs: syncing the playlist
// must not erase the history of successfully downloaded files.
type DownloadedTracksRepository struct {
	db *sqlx.DB
	mu sync.Mutex
}

func NewDownloadedTracksRepository(db *sqlx.DB) (*DownloadedTracksRepository, error) {
	// Bootstrap existing installations as well as providing a Goose migration.
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS downloaded_tracks (
		source TEXT NOT NULL,
		track_id TEXT NOT NULL,
		file_path TEXT NOT NULL,
		sha256 TEXT NOT NULL CHECK (length(sha256) = 64),
		downloaded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (source, track_id, file_path)
	)`)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS downloaded_queries (
		source TEXT NOT NULL,
		query TEXT NOT NULL,
		track_id TEXT NOT NULL,
		file_path TEXT NOT NULL,
		PRIMARY KEY (source, query, file_path)
	)`)
	if err != nil {
		return nil, err
	}
	return &DownloadedTracksRepository{db: db}, nil
}

func (r *DownloadedTracksRepository) LookupQueryPaths(source, query string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var paths []string
	err := r.db.Select(&paths, `SELECT file_path FROM downloaded_queries
		WHERE source=? AND query=? ORDER BY file_path`, source, query)
	return paths, err
}

func (r *DownloadedTracksRepository) SaveQuery(source, query, trackID, filePath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.db.Exec(`INSERT INTO downloaded_queries (source,query,track_id,file_path)
		VALUES (?,?,?,?) ON CONFLICT (source,query,file_path) DO UPDATE SET track_id=excluded.track_id`,
		source, query, trackID, filePath)
	return err
}

func (r *DownloadedTracksRepository) LookupHash(source, trackID, filePath string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var hash string
	err := r.db.Get(&hash, `SELECT sha256 FROM downloaded_tracks
		WHERE source = ? AND track_id = ? AND file_path = ?`, source, trackID, filePath)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return hash, err
}

func (r *DownloadedTracksRepository) SaveHash(source, trackID, filePath, hash string) error {
	// Serialize SQLite access from the ten download workers.
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.db.Exec(`INSERT INTO downloaded_tracks (source, track_id, file_path, sha256)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (source, track_id, file_path) DO UPDATE SET
			sha256 = excluded.sha256,
			downloaded_at = CURRENT_TIMESTAMP`, source, trackID, filePath, hash)
	return err
}
