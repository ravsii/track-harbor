-- +goose Up
CREATE TABLE IF NOT EXISTS proxy_cache_policy (
    id INTEGER PRIMARY KEY CHECK(id=1),
    policy TEXT NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS proxy_cache_policy;
