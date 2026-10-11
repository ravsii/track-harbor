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
	personal, err := json.Marshal(defaults.PersonalProxy)
	if err != nil {
		return DownloadSettings{}, err
	}
	public, err := json.Marshal(defaults.PublicProxy)
	if err != nil {
		return DownloadSettings{}, err
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO download_settings(id, strategy, personal_proxy, public_proxy) VALUES(1, ?, ?, ?)`, strategy, personal, public); err != nil {
		return DownloadSettings{}, err
	}
	return s.DownloadSettings()
}

func (s *Store) DownloadSettings() (DownloadSettings, error) {
	var strategy, personal, public string
	err := s.db.QueryRow(`SELECT strategy, personal_proxy, public_proxy FROM download_settings WHERE id=1`).Scan(&strategy, &personal, &public)
	if err != nil {
		return DownloadSettings{}, err
	}
	var settings DownloadSettings
	if err := json.Unmarshal([]byte(strategy), &settings.Strategy); err != nil {
		return DownloadSettings{}, err
	}
	if err := json.Unmarshal([]byte(personal), &settings.PersonalProxy); err != nil {
		return DownloadSettings{}, err
	}
	if err := json.Unmarshal([]byte(public), &settings.PublicProxy); err != nil {
		return DownloadSettings{}, err
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
	personal, err := json.Marshal(settings.PersonalProxy)
	if err != nil {
		return err
	}
	public, err := json.Marshal(settings.PublicProxy)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE download_settings SET strategy=?, personal_proxy=?, public_proxy=? WHERE id=1`, strategy, personal, public)
	return err
}
