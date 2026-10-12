package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen        string   `yaml:"listen"`
	DownloadDir   string   `yaml:"download_dir"`
	ConfigPath    string   `yaml:"-"`
	Strategy      []string `yaml:"strategy"`
	PersonalProxy []string `yaml:"personal_proxy"`
	PublicProxy   []string `yaml:"public_proxy"`
	loadedYAML    []byte
}

const defaultYAML = "listen: 127.0.0.1:8787\n"

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
	cfg.ConfigPath = configPath
	cfg.loadedYAML = data
	if cfg.DownloadDir == "" {
		cfg.DownloadDir = filepath.Join(filepath.Dir(dbPath), "downloads")
	}
	if len(cfg.Strategy) == 0 {
		cfg.Strategy = []string{"direct"}
	}
	return cfg, dbPath, nil
}

// RemoveImportedSettings leaves startup-only settings in YAML after the
// download routes and proxies have been safely imported into the database.
func RemoveImportedSettings(cfg Config, strategy, personal, public []string) error {
	path, err := filepath.EvalSymlinks(cfg.ConfigPath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, cfg.loadedYAML) {
		return fmt.Errorf("server config changed during settings import; review config.yaml")
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("server config must be a YAML mapping")
	}
	root := doc.Content[0]
	content := make([]*yaml.Node, 0, len(root.Content))
	changed := false
	for i := 0; i < len(root.Content); i += 2 {
		switch root.Content[i].Value {
		case "strategy", "vk_strategy", "personal_proxy", "public_proxy":
			changed = true
		default:
			content = append(content, root.Content[i], root.Content[i+1])
		}
	}
	if !changed {
		return nil
	}
	if !slices.Equal(cfg.Strategy, strategy) || !slices.Equal(cfg.PersonalProxy, personal) || !slices.Equal(cfg.PublicProxy, public) {
		return fmt.Errorf("legacy download settings differ from the database; review config.yaml before removing them")
	}
	root.Content = content
	updated, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".config-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(updated); err != nil {
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
	return os.Rename(f.Name(), path)
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
