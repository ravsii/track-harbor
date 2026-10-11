package config

import (
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

const defaultYAML = "version: 1\nstrategy:\n  - direct\nmy_proxy: []\n"

type Config struct {
	Version  int      `yaml:"version"`
	Strategy []string `yaml:"strategy"`
	MyProxy  []string `yaml:"my_proxy"`
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

// Load creates the default configuration only when no user file exists.
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
	f, err = os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	decoder := yaml.NewDecoder(f)
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
		return Config{}, fmt.Errorf("%s: strategy must be a non-empty list of direct/config_proxy/random_proxy", path)
	}
	seen := make(map[string]bool)
	for i, name := range cfg.Strategy {
		// Preserve old configurations without rewriting the user's file.
		if name == "proxy" {
			name = "random_proxy"
			cfg.Strategy[i] = name
		}
		if name != "direct" && name != "config_proxy" && name != "random_proxy" {
			return Config{}, fmt.Errorf("%s: unknown strategy %q; expected direct, config_proxy or random_proxy", path, name)
		}
		if seen[name] {
			return Config{}, fmt.Errorf("%s: repeated strategy %q", path, name)
		}
		seen[name] = true
	}
	addresses := make([]string, 0, len(cfg.MyProxy))
	unique := make(map[string]bool)
	for i, address := range cfg.MyProxy {
		address = strings.TrimSpace(address)
		if !strings.Contains(address, "://") {
			address = "http://" + address
		}
		u, err := url.Parse(address)
		// Do not include the supplied URL or parser error: it can contain a password.
		if err != nil || u.Hostname() == "" || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
			return Config{}, fmt.Errorf("%s: my_proxy[%d]: expected proxy URL with host and port", path, i+1)
		}
		switch u.Scheme {
		case "http", "https", "socks4", "socks5":
		default:
			return Config{}, fmt.Errorf("%s: my_proxy[%d]: supported protocols are http, https, socks4, socks5", path, i+1)
		}
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return Config{}, fmt.Errorf("%s: my_proxy[%d]: port must be between 1 and 65535", path, i+1)
		}
		u.Path = ""
		address = u.String()
		if !unique[address] {
			unique[address] = true
			addresses = append(addresses, address)
		}
	}
	cfg.MyProxy = addresses
	if seen["config_proxy"] && len(cfg.MyProxy) == 0 {
		return Config{}, fmt.Errorf("%s: config_proxy requires a non-empty my_proxy list", path)
	}
	return cfg, nil
}
