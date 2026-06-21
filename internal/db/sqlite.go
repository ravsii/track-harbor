package db

import (
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func New() (*sqlx.DB, error) {
	db, err := sqlx.Open("sqlite3", "meshtune.db")
	if err != nil {
		return nil, err
	}

	return db, nil
}
