ALTER TABLE sessions RENAME TO user_sessions;
ALTER TABLE user_sessions ADD COLUMN renewed_at INTEGER NOT NULL DEFAULT 0;

-- Keep existing sessions without granting them more than one remaining day.
UPDATE user_sessions
SET expires_at = MIN(expires_at, CAST(strftime('%s','now') AS INTEGER) + 86400),
    renewed_at = CAST(strftime('%s','now') AS INTEGER);

DROP INDEX sessions_user;
CREATE INDEX user_sessions_user ON user_sessions(user_id);
