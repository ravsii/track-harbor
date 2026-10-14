CREATE TABLE tracks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    artist TEXT NOT NULL,
    name TEXT NOT NULL,
    source TEXT NOT NULL CHECK (length(source) > 0),
    source_key TEXT NOT NULL CHECK (length(source_key) > 0),
    source_url TEXT NOT NULL DEFAULT '',
    cover_url TEXT NOT NULL DEFAULT '',
    UNIQUE (source, source_key)
);

INSERT INTO tracks(id,artist,name,source,source_key,source_url,cover_url)
SELECT t.id,t.artist,t.title,t.source,t.source_id,
    COALESCE((SELECT audio_url FROM user_tracks p
        WHERE p.track_id=t.id AND p.audio_url<>''
        ORDER BY p.synced_at DESC,p.user_id LIMIT 1),''),t.cover_url
FROM catalog_tracks t;

CREATE TABLE user_tracks_new (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_id INTEGER NOT NULL REFERENCES tracks(id),
    added_at INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, track_id)
);
INSERT INTO user_tracks_new(user_id,track_id,added_at)
SELECT user_id,track_id,added_at FROM user_tracks;

DROP TABLE user_tracks;
ALTER TABLE user_tracks_new RENAME TO user_tracks;
CREATE INDEX user_tracks_track ON user_tracks(track_id);
DROP TABLE shared_downloads;
DROP TABLE catalog_tracks;
