CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL
);
CREATE TABLE auth_meta (
    id INTEGER PRIMARY KEY CHECK (id=1),
    signing_key BLOB NOT NULL,
    bootstrapped INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at INTEGER NOT NULL
);
CREATE INDEX sessions_user ON sessions(user_id);
CREATE TABLE user_credentials (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source TEXT NOT NULL CHECK (source IN ('spotify','vk')),
    first TEXT NOT NULL,
    second TEXT NOT NULL,
    PRIMARY KEY (user_id, source)
);
CREATE TABLE catalog_tracks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL CHECK (source IN ('spotify','vk')),
    source_id TEXT NOT NULL CHECK (length(source_id)>0),
    artist TEXT NOT NULL,
    title TEXT NOT NULL,
    cover_url TEXT NOT NULL DEFAULT '',
    UNIQUE (source, source_id)
);
CREATE TABLE user_tracks (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_id INTEGER NOT NULL REFERENCES catalog_tracks(id),
    added_at INTEGER NOT NULL DEFAULT 0,
    audio_url TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (user_id, track_id)
);
CREATE INDEX user_tracks_track ON user_tracks(track_id);
CREATE TABLE shared_downloads (
    track_id INTEGER PRIMARY KEY REFERENCES catalog_tracks(id),
    file_path TEXT NOT NULL,
    sha256 TEXT NOT NULL
);
-- Only VK IDs can be recovered exactly. Keep the legacy tables/file index for
-- Spotify: its old rows contain no source ID and must not be guessed from names.
INSERT INTO catalog_tracks(source,source_id,artist,title,cover_url)
SELECT source, CAST(owner_id AS TEXT)||'_'||CAST(audio_id AS TEXT),artist,title,cover_url
FROM tracks WHERE source='vk' AND owner_id<>0 AND audio_id>0
ON CONFLICT(source,source_id) DO NOTHING;
INSERT INTO shared_downloads(track_id,file_path,sha256)
SELECT c.id,d.file_path,d.sha256 FROM tracks t JOIN downloads d
ON t.source=d.source AND t.artist=d.artist AND t.title=d.title
JOIN catalog_tracks c ON c.source=t.source
AND c.source_id=CAST(t.owner_id AS TEXT)||'_'||CAST(t.audio_id AS TEXT)
WHERE t.source='vk' AND t.owner_id<>0 AND t.audio_id>0
ON CONFLICT(track_id) DO NOTHING;
-- Users explicitly reconnect their sources after this upgrade.
DELETE FROM credentials;
UPDATE tracks SET audio_url='';
