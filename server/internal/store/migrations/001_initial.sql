CREATE TABLE IF NOT EXISTS credentials (
    source TEXT PRIMARY KEY,
    first TEXT NOT NULL,
    second TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tracks (
    source TEXT NOT NULL,
    artist TEXT NOT NULL,
    title TEXT NOT NULL,
    added_at INTEGER NOT NULL DEFAULT 0,
    owner_id INTEGER NOT NULL DEFAULT 0,
    audio_id INTEGER NOT NULL DEFAULT 0,
    audio_url TEXT NOT NULL DEFAULT '',
    cover_url TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (source, artist, title)
);

CREATE TABLE IF NOT EXISTS downloads (
    source TEXT NOT NULL,
    artist TEXT NOT NULL,
    title TEXT NOT NULL,
    file_path TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    PRIMARY KEY (source, artist, title)
);
