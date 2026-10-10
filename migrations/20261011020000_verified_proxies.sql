-- +goose Up
CREATE TABLE IF NOT EXISTS verified_proxies (
    url TEXT PRIMARY KEY,
    valid_until INTEGER NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS verified_proxies;
