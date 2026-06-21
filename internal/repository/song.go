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
