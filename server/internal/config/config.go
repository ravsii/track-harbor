package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

type Config struct {
	Listen string `json:"listen"`
}

func Load() (Config, string, error) {
	dir := os.Getenv("TRACKHARBOR_SERVER_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Config{}, "", err
		}
		dir = filepath.Join(home, ".trackharbor-server")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Config{}, "", err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return Config{}, "", err
	}
	path := filepath.Join(dir, "config.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
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
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, "", err
	}
	if err := os.Chmod(path, 0600); err != nil {
		return Config{}, "", err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, "", fmt.Errorf("server config: %w", err)
	}
	host, _, err := net.SplitHostPort(cfg.Listen)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return Config{}, "", fmt.Errorf("server without login must listen on a loopback IP address")
	}
	return cfg, filepath.Join(dir, "library.db"), nil
}
