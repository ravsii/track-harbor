package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

const (
	settingDownloadsStopped = "downloads_stopped"
	settingDownloadStrategy = "download_strategy"
	settingPublicProxy      = "public_proxy"
)

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
	Strategy      []string
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

func (s *Store) Proxies() ([]Proxy, error) {
	rows, err := s.db.Query(`SELECT id, address, type, last_checked_at, last_success_at FROM proxy ORDER BY id`)
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

func (s *Store) Proxy(id int64) (string, error) {
	var address string
	err := s.db.QueryRow(`SELECT address FROM proxy WHERE id=?`, id).Scan(&address)
	return address, err
}

func (s *Store) AddProxy(address string, checkedAt time.Time) error {
	_, err := s.db.Exec(`INSERT INTO proxy(address, last_checked_at, last_success_at) VALUES(?, ?, ?)`, address, checkedAt.Unix(), checkedAt.Unix())
	return err
}

func (s *Store) RecordProxyCheck(address string, checkedAt time.Time, success bool) error {
	_, err := s.db.Exec(`UPDATE proxy SET last_checked_at=?, last_success_at=CASE WHEN ? THEN ? ELSE last_success_at END WHERE address=?`, checkedAt.Unix(), success, checkedAt.Unix(), address)
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

func validateStrategy(strategy []string) error {
	if len(strategy) == 0 {
		return fmt.Errorf("choose at least one download route")
	}
	for _, route := range strategy {
		if route != "direct" && route != "personal_proxy" && route != "random_proxy" {
			return fmt.Errorf("unknown download route")
		}
	}
	return nil
}

// InitDownloadSettings imports legacy YAML values only when no settings have been saved.
func (s *Store) InitDownloadSettings(defaults DownloadSettings) (DownloadSettings, error) {
	_, err := s.Setting(settingDownloadStrategy)
	if err == nil {
		return s.DownloadSettings()
	}
	if err != sql.ErrNoRows {
		return DownloadSettings{}, err
	}
	if err := validateStrategy(defaults.Strategy); err != nil {
		return DownloadSettings{}, err
	}
	strategy, err := json.Marshal(defaults.Strategy)
	if err != nil {
		return DownloadSettings{}, err
	}
	public, err := json.Marshal(defaults.PublicProxy)
	if err != nil {
		return DownloadSettings{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return DownloadSettings{}, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`INSERT INTO settings(name, value) VALUES(?, ?) ON CONFLICT(name) DO NOTHING`, settingDownloadStrategy, string(strategy))
	if err != nil {
		return DownloadSettings{}, err
	}
	if inserted, _ := result.RowsAffected(); inserted != 0 {
		if _, err := tx.Exec(`INSERT INTO settings(name, value) VALUES(?, ?) ON CONFLICT(name) DO NOTHING`, settingPublicProxy, string(public)); err != nil {
			return DownloadSettings{}, err
		}
		for _, address := range defaults.PersonalProxy {
			if _, err := tx.Exec(`INSERT INTO proxy(address) VALUES(?)`, address); err != nil {
				return DownloadSettings{}, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return DownloadSettings{}, err
	}
	return s.DownloadSettings()
}

func (s *Store) DownloadSettings() (DownloadSettings, error) {
	var strategy, public string
	err := s.db.QueryRow(`SELECT strategy.value, public_proxy.value
		FROM settings AS strategy CROSS JOIN settings AS public_proxy
		WHERE strategy.name=? AND public_proxy.name=?`, settingDownloadStrategy, settingPublicProxy).Scan(&strategy, &public)
	if err != nil {
		return DownloadSettings{}, err
	}
	var settings DownloadSettings
	if err := json.Unmarshal([]byte(strategy), &settings.Strategy); err != nil {
		return DownloadSettings{}, err
	}
	if err := json.Unmarshal([]byte(public), &settings.PublicProxy); err != nil {
		return DownloadSettings{}, err
	}
	proxies, err := s.Proxies()
	if err != nil {
		return DownloadSettings{}, err
	}
	for _, proxy := range proxies {
		settings.PersonalProxy = append(settings.PersonalProxy, proxy.Address)
	}
	return settings, validateStrategy(settings.Strategy)
}

func (s *Store) SaveDownloadSettings(settings DownloadSettings) error {
	if err := validateStrategy(settings.Strategy); err != nil {
		return err
	}
	strategy, err := json.Marshal(settings.Strategy)
	if err != nil {
		return err
	}
	public, err := json.Marshal(settings.PublicProxy)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO settings(name, value) VALUES(?, ?), (?, ?)
		ON CONFLICT(name) DO UPDATE SET value=excluded.value`, settingDownloadStrategy, string(strategy), settingPublicProxy, string(public))
	return err
}
