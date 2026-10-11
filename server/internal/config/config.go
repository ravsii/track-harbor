package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen        string   `yaml:"listen"`
	DownloadDir   string   `yaml:"download_dir"`
	Strategy      []string `yaml:"strategy"`
	VKStrategy    []string `yaml:"vk_strategy"`
	PersonalProxy []string `yaml:"personal_proxy"`
	PublicProxy   []string `yaml:"public_proxy"`
}

const defaultYAML = "listen: 127.0.0.1:8787\nstrategy:\n  - direct\nvk_strategy:\n  - direct\n"

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
		configPath = filepath.Join(dir, "config.yaml")
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
	if os.Getenv("TRACKHARBOR_SERVER_CONFIG") == "" {
		if err := migrateJSON(filepath.Join(dir, "config.json"), configPath); err != nil {
			return Config{}, "", err
		}
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		f, err := os.OpenFile(configPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil && !os.IsExist(err) {
			return Config{}, "", err
		}
		if err == nil {
			_, writeErr := f.WriteString(defaultYAML)
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
	if err := yaml.Unmarshal(data, &cfg); err != nil {
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
	if len(cfg.VKStrategy) == 0 {
		cfg.VKStrategy = []string{"direct"}
	}
	if len(cfg.VKStrategy) != 1 || cfg.VKStrategy[0] != "direct" {
		return Config{}, "", fmt.Errorf("server config: vk_strategy currently supports only direct")
	}
	host, _, err := net.SplitHostPort(cfg.Listen)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return Config{}, "", fmt.Errorf("server without login must listen on a loopback IP address")
	}
	return cfg, dbPath, nil
}

// migrateJSON preserves existing server settings (including private proxy URLs)
// when upgrading the default config path. An explicit config override is never
// migrated, and an existing YAML file always takes precedence.
func migrateJSON(oldPath, newPath string) error {
	if _, err := os.Stat(newPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := os.ReadFile(oldPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var values map[string]any
	if err := json.Unmarshal(data, &values); err != nil || values == nil {
		return fmt.Errorf("server config: existing JSON is invalid; previous file was kept")
	}
	converted, err := yaml.Marshal(values)
	if err != nil {
		return fmt.Errorf("server config: could not convert existing JSON: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(newPath), ".config-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(converted); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Link never replaces a YAML config created by another server process.
	if err := os.Link(f.Name(), newPath); errors.Is(err, os.ErrExist) {
		return nil
	} else if err != nil {
		return err
	}
	current, err := os.ReadFile(oldPath)
	if err != nil || string(current) != string(data) {
		return fmt.Errorf("server config: JSON changed during migration; keep the original and review config.yaml")
	}
	if err := os.Remove(oldPath); err != nil {
		return fmt.Errorf("server config: YAML created but old JSON could not be removed: %w", err)
	}
	return nil
}
