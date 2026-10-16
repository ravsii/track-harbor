CREATE TABLE settings (
    name TEXT NOT NULL PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO settings(name, value)
SELECT 'downloads_stopped', CASE stopped WHEN 1 THEN 'true' ELSE 'false' END
FROM download_control WHERE id = 1;

INSERT INTO settings(name, value)
SELECT 'download_strategy', strategy FROM download_settings WHERE id = 1;

INSERT INTO settings(name, value)
SELECT 'public_proxy', public_proxy FROM download_settings WHERE id = 1;

DROP TABLE download_control;
DROP TABLE download_settings;
