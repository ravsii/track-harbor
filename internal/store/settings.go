package store

import (
	"fmt"
	"strconv"
	"time"
)

const settingDownloadsStopped = "downloads_stopped"

func (s *Store) Setting(name string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE name=?`, name).Scan(&value)
	return value, err
}

func (s *Store) SetSetting(name, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings(name, value) VALUES(?, ?) ON CONFLICT(name) DO UPDATE SET value=excluded.value`, name, value)
	return err
}

func (s *Store) DownloadsStopped() (bool, error) {
	value, err := s.Setting(settingDownloadsStopped)
	if err != nil {
		return false, err
	}
	stopped, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("invalid downloads_stopped setting")
	}
	return stopped, nil
}

func (s *Store) SetDownloadsStopped(stopped bool) error {
	return s.SetSetting(settingDownloadsStopped, strconv.FormatBool(stopped))
}

type DownloadSettings struct {
	PersonalProxy []string
	PublicProxy   []string
}

type Proxy struct {
	ID            int64
	Address       string
	Type          string
	LastCheckedAt time.Time
	LastSuccessAt time.Time
}

func (s *Store) Proxies(proxyType string) ([]Proxy, error) {
	rows, err := s.db.Query(`SELECT id, address, type, last_checked_at, last_success_at FROM proxy WHERE type=? ORDER BY id`, proxyType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var proxies []Proxy
	for rows.Next() {
		var proxy Proxy
		var checkedAt, successAt int64
		if err := rows.Scan(&proxy.ID, &proxy.Address, &proxy.Type, &checkedAt, &successAt); err != nil {
			return nil, err
		}
		proxy.LastCheckedAt = time.Unix(checkedAt, 0).UTC()
		proxy.LastSuccessAt = time.Unix(successAt, 0).UTC()
		proxies = append(proxies, proxy)
	}
	return proxies, rows.Err()
}

func (s *Store) Proxy(id int64) (Proxy, error) {
	var proxy Proxy
	var checkedAt, successAt int64
	err := s.db.QueryRow(`SELECT id, address, type, last_checked_at, last_success_at FROM proxy WHERE id=?`, id).Scan(&proxy.ID, &proxy.Address, &proxy.Type, &checkedAt, &successAt)
	if err != nil {
		return Proxy{}, err
	}
	proxy.LastCheckedAt = time.Unix(checkedAt, 0).UTC()
	proxy.LastSuccessAt = time.Unix(successAt, 0).UTC()
	return proxy, nil
}

func (s *Store) ProxyCount(proxyType string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM proxy WHERE type=?`, proxyType).Scan(&count)
	return count, err
}

func (s *Store) AddProxy(address string, checkedAt time.Time) (bool, error) {
	result, err := s.db.Exec(`INSERT INTO proxy(address, type, last_checked_at, last_success_at) VALUES(?, 'personal', ?, ?) ON CONFLICT(address) DO NOTHING`, address, checkedAt.Unix(), checkedAt.Unix())
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n != 0, err
}

// AddPublicProxies persists candidates before probing. Zero means not checked/successful yet.
// Existing addresses, especially personal proxies, retain their type and history.
func (s *Store) AddPublicProxies(addresses []string) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO proxy(address, type, last_checked_at, last_success_at) VALUES(?, 'public', 0, 0) ON CONFLICT(address) DO NOTHING`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	added := 0
	for _, address := range addresses {
		result, err := stmt.Exec(address)
		if err != nil {
			return 0, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		added += int(n)
	}
	return added, tx.Commit()
}

func (s *Store) RecordProxyCheck(address string, checkedAt time.Time, success bool) error {
	_, err := s.db.Exec(`UPDATE proxy SET last_checked_at=MAX(last_checked_at, ?), last_success_at=CASE WHEN ? THEN MAX(last_success_at, ?) ELSE last_success_at END WHERE address=?`, checkedAt.Unix(), success, checkedAt.Unix(), address)
	return err
}

func (s *Store) DeleteProxy(id int64) (bool, error) {
	result, err := s.db.Exec(`DELETE FROM proxy WHERE id=?`, id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n != 0, err
}

// InitDownloadSettings safely imports any remaining legacy YAML proxy lists.
func (s *Store) InitDownloadSettings(defaults DownloadSettings) (DownloadSettings, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return DownloadSettings{}, err
	}
	defer tx.Rollback()
	for _, address := range defaults.PersonalProxy {
		if _, err := tx.Exec(`INSERT INTO proxy(address, type) VALUES(?, 'personal') ON CONFLICT(address) DO UPDATE SET type='personal'`, address); err != nil {
			return DownloadSettings{}, err
		}
	}
	for _, address := range defaults.PublicProxy {
		if _, err := tx.Exec(`INSERT INTO proxy(address, type) VALUES(?, 'public') ON CONFLICT(address) DO NOTHING`, address); err != nil {
			return DownloadSettings{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return DownloadSettings{}, err
	}
	return s.DownloadSettings()
}

func (s *Store) DownloadSettings() (DownloadSettings, error) {
	rows, err := s.db.Query(`SELECT address, type FROM proxy WHERE type='personal' OR (type='public' AND last_success_at>0) ORDER BY id`)
	if err != nil {
		return DownloadSettings{}, err
	}
	defer rows.Close()
	var settings DownloadSettings
	for rows.Next() {
		var address, proxyType string
		if err := rows.Scan(&address, &proxyType); err != nil {
			return DownloadSettings{}, err
		}
		if proxyType == "personal" {
			settings.PersonalProxy = append(settings.PersonalProxy, address)
		} else {
			settings.PublicProxy = append(settings.PublicProxy, address)
		}
	}
	return settings, rows.Err()
}
