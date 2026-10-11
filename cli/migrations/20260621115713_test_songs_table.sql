-- +goose Up

CREATE TABLE songs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    artist TEXT NOT NULL,
    title TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_songs_artist_title
ON songs(artist, title);

-- +goose Down

DROP INDEX idx_songs_artist_title;

DROP TABLE songs;
