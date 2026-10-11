package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

type DownloadSettings struct {
	Strategy      []string
	PersonalProxy []string
	PublicProxy   []string
}

type PersonalProxy struct {
	ID      int64
	Address string
}

func (s *Store) PersonalProxies() ([]PersonalProxy, error) {
	rows, err := s.db.Query(`SELECT id, address FROM personal_proxies ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var proxies []PersonalProxy
	for rows.Next() {
		var proxy PersonalProxy
		if err := rows.Scan(&proxy.ID, &proxy.Address); err != nil {
			return nil, err
		}
		proxies = append(proxies, proxy)
	}
	return proxies, rows.Err()
}

func (s *Store) PersonalProxy(id int64) (string, error) {
	var address string
	err := s.db.QueryRow(`SELECT address FROM personal_proxies WHERE id=?`, id).Scan(&address)
	return address, err
}

func (s *Store) AddPersonalProxy(address string) error {
	_, err := s.db.Exec(`INSERT INTO personal_proxies(address) VALUES(?)`, address)
	return err
}

func (s *Store) DeletePersonalProxy(id int64) (bool, error) {
	result, err := s.db.Exec(`DELETE FROM personal_proxies WHERE id=?`, id)
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
	var id int
	err := s.db.QueryRow(`SELECT id FROM download_settings WHERE id=1`).Scan(&id)
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
	result, err := tx.Exec(`INSERT OR IGNORE INTO download_settings(id, strategy, personal_proxy, public_proxy) VALUES(1, ?, '[]', ?)`, strategy, public)
	if err != nil {
		return DownloadSettings{}, err
	}
	if inserted, _ := result.RowsAffected(); inserted != 0 {
		for _, address := range defaults.PersonalProxy {
			if _, err := tx.Exec(`INSERT INTO personal_proxies(address) VALUES(?)`, address); err != nil {
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
	err := s.db.QueryRow(`SELECT strategy, public_proxy FROM download_settings WHERE id=1`).Scan(&strategy, &public)
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
	proxies, err := s.PersonalProxies()
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
	_, err = s.db.Exec(`UPDATE download_settings SET strategy=?, public_proxy=? WHERE id=1`, strategy, public)
	return err
}
