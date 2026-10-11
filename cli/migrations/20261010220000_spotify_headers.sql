-- +goose Up
CREATE TABLE IF NOT EXISTS spotify_headers (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    authorization TEXT NOT NULL,
    client_token TEXT NOT NULL
);

-- +goose Down
DROP TABLE spotify_headers;
