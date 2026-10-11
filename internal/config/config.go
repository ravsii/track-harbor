package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultYAML = "version: 1\nstrategy:\n  - direct\npersonal_proxy: []\ndownload_dir: ~/.meshvault/downloaded\n"

type Config struct {
	Version       int      `yaml:"version"`
	Strategy      []string `yaml:"strategy"`
	PersonalProxy []string `yaml:"personal_proxy"`
	DownloadDir   string   `yaml:"download_dir"`
}

func Directory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".meshvault"), nil
}

func DatabasePath() (string, error) {
	dir, err := Directory()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "vault.db"), nil
}

// Load creates the configuration and persists missing default fields without
// replacing explicitly configured values.
func Load() (Config, error) {
	dir, err := Directory()
	if err != nil {
		return Config{}, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Config{}, err
	}
	path := filepath.Join(dir, "config.yaml")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		_, writeErr := io.WriteString(f, defaultYAML)
		closeErr := f.Close()
		if err := errors.Join(writeErr, closeErr); err != nil {
			return Config{}, fmt.Errorf("create %s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrExist) {
		return Config{}, fmt.Errorf("create %s: %w", path, err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	data, changed, err := completeDefaults(original)
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("%s: expected one YAML document", path)
	}
	if cfg.Version != 1 {
		return Config{}, fmt.Errorf("%s: version must be 1, got %d", path, cfg.Version)
	}
	if len(cfg.Strategy) == 0 {
		return Config{}, fmt.Errorf("%s: strategy must be a non-empty list of direct/personal_proxy/random_proxy", path)
	}
	seen := make(map[string]bool)
	for i, name := range cfg.Strategy {
		// Preserve old configurations without rewriting the user's file.
		if name == "proxy" {
			name = "random_proxy"
			cfg.Strategy[i] = name
		}
		if name != "direct" && name != "personal_proxy" && name != "random_proxy" {
			return Config{}, fmt.Errorf("%s: unknown strategy %q; expected direct, personal_proxy or random_proxy", path, name)
		}
		if seen[name] {
			return Config{}, fmt.Errorf("%s: repeated strategy %q", path, name)
		}
		seen[name] = true
	}
	addresses := make([]string, 0, len(cfg.PersonalProxy))
	unique := make(map[string]bool)
	for i, address := range cfg.PersonalProxy {
		address = strings.TrimSpace(address)
		if !strings.Contains(address, "://") {
			address = "http://" + address
		}
		u, err := url.Parse(address)
		// Do not include the supplied URL or parser error: it can contain a password.
		if err != nil || u.Hostname() == "" || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
			return Config{}, fmt.Errorf("%s: personal_proxy[%d]: expected proxy URL with host and port", path, i+1)
		}
		switch u.Scheme {
		case "http", "https", "socks4", "socks5":
		default:
			return Config{}, fmt.Errorf("%s: personal_proxy[%d]: supported protocols are http, https, socks4, socks5", path, i+1)
		}
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return Config{}, fmt.Errorf("%s: personal_proxy[%d]: port must be between 1 and 65535", path, i+1)
		}
		u.Path = ""
		address = u.String()
		if !unique[address] {
			unique[address] = true
			addresses = append(addresses, address)
		}
	}
	cfg.PersonalProxy = addresses
	if seen["personal_proxy"] && len(cfg.PersonalProxy) == 0 {
		return Config{}, fmt.Errorf("%s: personal_proxy requires a non-empty personal_proxy list", path)
	}
	cfg.DownloadDir = strings.TrimSpace(cfg.DownloadDir)
	if cfg.DownloadDir == "" {
		cfg.DownloadDir = filepath.Join(dir, "downloaded")
	} else if cfg.DownloadDir == "~" {
		cfg.DownloadDir = filepath.Dir(dir)
	} else if strings.HasPrefix(cfg.DownloadDir, "~/") || strings.HasPrefix(cfg.DownloadDir, `~\`) {
		cfg.DownloadDir = filepath.Join(filepath.Dir(dir), filepath.FromSlash(cfg.DownloadDir[2:]))
	} else if strings.HasPrefix(cfg.DownloadDir, "~") {
		return Config{}, fmt.Errorf("%s: download_dir supports ~ or ~/path, not another user's home", path)
	} else if !filepath.IsAbs(cfg.DownloadDir) {
		// Resolve relative paths against the configuration directory, not cwd.
		cfg.DownloadDir = filepath.Join(dir, cfg.DownloadDir)
	}
	cfg.DownloadDir = filepath.Clean(cfg.DownloadDir)
	if changed {
		if err := saveCompletedConfig(path, original, data); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}
