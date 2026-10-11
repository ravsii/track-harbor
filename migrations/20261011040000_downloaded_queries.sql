-- +goose Up
CREATE TABLE IF NOT EXISTS downloaded_queries (
    source TEXT NOT NULL,
    query TEXT NOT NULL,
    track_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    PRIMARY KEY (source, query, file_path)
);

-- +goose Down
DROP TABLE IF EXISTS downloaded_queries;
