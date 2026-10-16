CREATE TABLE proxy (
    id INTEGER PRIMARY KEY,
    address TEXT NOT NULL,
    last_checked_at INTEGER NOT NULL DEFAULT (CAST(strftime('%s','now') AS INTEGER)),
    last_success_at INTEGER NOT NULL DEFAULT (CAST(strftime('%s','now') AS INTEGER))
);

INSERT INTO proxy(id, address)
SELECT id, address FROM personal_proxies;

DROP TABLE personal_proxies;
