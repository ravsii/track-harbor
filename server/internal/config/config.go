package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

type Config struct {
	Listen        string   `json:"listen"`
	DownloadDir   string   `json:"download_dir"`
	Strategy      []string `json:"strategy"`
	PersonalProxy []string `json:"personal_proxy"`
	PublicProxy   []string `json:"public_proxy"`
}

func Load() (Config, string, error) {
	dir := os.Getenv("TRACKHARBOR_SERVER_DIR")
	if dir == "" {
		executable, err := os.Executable()
		if err != nil {
			return Config{}, "", fmt.Errorf("locate server executable: %w", err)
		}
		dir = filepath.Dir(executable)
	}
	configPath := os.Getenv("TRACKHARBOR_SERVER_CONFIG")
	if configPath == "" {
		configPath = filepath.Join(dir, "config.json")
	}
	dbPath := os.Getenv("TRACKHARBOR_SERVER_DB")
	if dbPath == "" {
		dbPath = filepath.Join(dir, "library.db")
	}
	for _, path := range []string{configPath, dbPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return Config{}, "", err
		}
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		f, err := os.OpenFile(configPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil && !os.IsExist(err) {
			return Config{}, "", err
		}
		if err == nil {
			_, writeErr := f.WriteString("{\n  \"listen\": \"127.0.0.1:8787\"\n}\n")
			closeErr := f.Close()
			if writeErr != nil {
				return Config{}, "", writeErr
			}
			if closeErr != nil {
				return Config{}, "", closeErr
			}
		}
	} else if err != nil {
		return Config{}, "", err
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return Config{}, "", err
	}
	if err := os.Chmod(configPath, 0600); err != nil {
		return Config{}, "", err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, "", fmt.Errorf("server config: %w", err)
	}
	if cfg.DownloadDir == "" {
		cfg.DownloadDir = filepath.Join(filepath.Dir(dbPath), "downloads")
	}
	if len(cfg.Strategy) == 0 {
		cfg.Strategy = []string{"direct"}
	}
	for _, route := range cfg.Strategy {
		if route != "direct" && route != "personal_proxy" && route != "random_proxy" {
			return Config{}, "", fmt.Errorf("server config: unknown download strategy %q", route)
		}
	}
	host, _, err := net.SplitHostPort(cfg.Listen)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return Config{}, "", fmt.Errorf("server without login must listen on a loopback IP address")
	}
	return cfg, dbPath, nil
}
