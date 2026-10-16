-- Preserve one row per address, preferring personal ownership and the latest checks.
UPDATE proxy
SET type = CASE WHEN EXISTS (
        SELECT 1 FROM proxy AS duplicate
        WHERE duplicate.address = proxy.address AND duplicate.type = 'personal'
    ) THEN 'personal' ELSE type END,
    last_checked_at = (SELECT MAX(duplicate.last_checked_at) FROM proxy AS duplicate WHERE duplicate.address = proxy.address),
    last_success_at = (SELECT MAX(duplicate.last_success_at) FROM proxy AS duplicate WHERE duplicate.address = proxy.address)
WHERE id IN (SELECT MIN(id) FROM proxy GROUP BY address);

DELETE FROM proxy WHERE id NOT IN (SELECT MIN(id) FROM proxy GROUP BY address);
CREATE UNIQUE INDEX proxy_address ON proxy(address);

INSERT INTO proxy(address, type)
SELECT DISTINCT entry.value, 'public'
FROM settings, json_each(settings.value) AS entry
WHERE settings.name = 'public_proxy' AND entry.type = 'text' AND trim(entry.value) <> ''
ON CONFLICT(address) DO NOTHING;

DELETE FROM settings WHERE name IN ('public_proxy', 'download_strategy');

INSERT INTO settings(name, value) VALUES ('proxy_sources',
'https://api.proxyscrape.com/v4/free-proxy-list/get?request=getproxies&protocol=http&ssl=all&timeout=10000&limit=500000&skip=0
https://api.proxyscrape.com/v4/free-proxy-list/get?request=getproxies&protocol=socks4&ssl=all&timeout=10000&limit=500000&skip=0
https://api.proxyscrape.com/v4/free-proxy-list/get?request=getproxies&protocol=socks5&ssl=all&timeout=10000&limit=500000&skip=0
https://raw.githubusercontent.com/proxifly/free-proxy-list/main/proxies/all/data.txt
https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/http.txt
https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/socks4.txt
https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/socks5.txt
https://raw.githubusercontent.com/stormsia/proxy-list/refs/heads/main/socks5.txt')
ON CONFLICT(name) DO NOTHING;
