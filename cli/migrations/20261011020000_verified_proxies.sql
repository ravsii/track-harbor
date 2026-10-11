-- +goose Up
CREATE TABLE IF NOT EXISTS verified_proxies (
    url TEXT PRIMARY KEY,
    valid_until INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS proxy_cache_state (
    id INTEGER PRIMARY KEY CHECK(id=1),
    full_valid_until INTEGER NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS proxy_cache_state;
DROP TABLE IF EXISTS verified_proxies;
