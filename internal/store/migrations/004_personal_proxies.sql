CREATE TABLE personal_proxies (
    id INTEGER PRIMARY KEY,
    address TEXT NOT NULL
);

INSERT INTO personal_proxies(address)
SELECT value FROM download_settings, json_each(download_settings.personal_proxy)
WHERE download_settings.id = 1 AND json_each.type = 'text';

UPDATE download_settings SET personal_proxy = '[]' WHERE id = 1;
