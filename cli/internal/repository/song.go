package repository

import (
	"time"

	"github.com/jmoiron/sqlx"
)

type Song struct {
	ID        int64     `db:"id"`
	Artists   string    `db:"artists"`
	Title     string    `db:"title"`
	CreatedAt time.Time `db:"created_at"`
}

type SongRepository struct {
	db *sqlx.DB
}

func NewSongRepository(db *sqlx.DB) *SongRepository {
	return &SongRepository{db: db}
}

func (r *SongRepository) GetAll() ([]Song, error) {
	var songs []Song
	err := r.db.Select(&songs, "SELECT * FROM songs")
	return songs, err
}

func (r *SongRepository) DeleteAll() error {
	_, err := r.db.Exec("DELETE FROM songs;")
	return err
}

func (r *SongRepository) BulkInsert(songs []Song) error {
	tx, err := r.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareNamed(`
		INSERT INTO songs (artists, title, created_at)
		VALUES (:artists, :title, :created_at)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, s := range songs {
		_, err := stmt.Exec(s)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ReplaceAll atomically replaces the library, keeping one record per song.
// Repeated playlist entries do not violate the unique artists/title index.
func (r *SongRepository) ReplaceAll(songs []Song) (int, error) {
	tx, err := r.db.Beginx()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var existing []Song
	if err := tx.Select(&existing, "SELECT artists, title, created_at FROM songs"); err != nil {
		return 0, err
	}
	previous := make(map[string]time.Time, len(existing))
	for _, song := range existing {
		previous[song.Artists+"\x00"+song.Title] = song.CreatedAt
	}
	if _, err := tx.Exec("DELETE FROM songs"); err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareNamed(`
		INSERT INTO songs (artists, title, created_at)
		VALUES (:artists, :title, :created_at)
		ON CONFLICT (artists, title) DO NOTHING
	`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	saved := 0
	for _, song := range songs {
		if song.CreatedAt.IsZero() {
			song.CreatedAt = previous[song.Artists+"\x00"+song.Title]
			if song.CreatedAt.IsZero() {
				// The legacy CLI column is NOT NULL; use first discovery time
				// only when Spotify supplied no date and no previous row exists.
				song.CreatedAt = time.Now()
			}
		}
		result, err := stmt.Exec(song)
		if err != nil {
			return 0, err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		saved += int(rows)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return saved, nil
}
