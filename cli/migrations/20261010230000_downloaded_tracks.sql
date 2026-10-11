-- +goose Up
CREATE TABLE IF NOT EXISTS downloaded_tracks (
    source TEXT NOT NULL,
    track_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    sha256 TEXT NOT NULL CHECK (length(sha256) = 64),
    downloaded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (source, track_id, file_path)
);

-- +goose Down
DROP TABLE downloaded_tracks;
