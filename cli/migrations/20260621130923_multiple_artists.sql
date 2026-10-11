-- +goose Up
CREATE TABLE songs_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    artists TEXT NOT NULL, -- JSON array of strings
    title TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- перенос данных (старый artist -> массив из одного элемента)
INSERT INTO songs_new (id, artists, title, created_at)
SELECT
    id,
    json_array(artist),
    title,
    created_at
FROM songs;

DROP TABLE songs;

ALTER TABLE songs_new RENAME TO songs;

-- индекс (оставляем как был по смыслу)
CREATE UNIQUE INDEX idx_songs_artists_title
ON songs(artists, title);


-- +goose Down

CREATE TABLE songs_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    artist TEXT NOT NULL,
    title TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO songs_old (id, artist, title, created_at)
SELECT
    id,
    artist,
    title,
    created_at
FROM songs;

DROP TABLE songs;

ALTER TABLE songs_old RENAME TO songs;

CREATE UNIQUE INDEX idx_songs_artist_title
ON songs(artist, title);
