package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"go.uber.org/zap"
)

func settingsLines(raw string) []string {
	var lines []string
	for _, line := range strings.Split(raw, "\n") {
		if value := strings.TrimSpace(line); value != "" {
			lines = append(lines, value)
		}
	}
	return lines
}

func settingsRoutes(raw string) ([]string, error) {
	routes := settingsLines(raw)
	if len(routes) == 0 || len(routes) > 100 {
		return nil, fmt.Errorf("choose one to 100 routes, one per line")
	}
	for _, route := range routes {
		if route != "direct" && route != "personal_proxy" && route != "random_proxy" {
			return nil, fmt.Errorf("routes must be direct, personal_proxy or random_proxy")
		}
	}
	return routes, nil
}

func settingsProxies(raw string) ([]string, error) {
	proxies := settingsLines(raw)
	if len(proxies) > 100 {
		return nil, fmt.Errorf("use no more than 100 proxy addresses")
	}
	for _, address := range proxies {
		parsed, err := url.Parse(address)
		if len(address) > 2048 || err != nil || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("proxy addresses must be http(s) or SOCKS URLs with a host (one per line)")
		}
		switch parsed.Scheme {
		case "http", "https", "socks4", "socks4a", "socks5", "socks5h":
		default:
			return nil, fmt.Errorf("proxy addresses must be http(s) or SOCKS URLs with a host (one per line)")
		}
	}
	return proxies, nil
}

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	if s.downloads == nil {
		http.Error(w, "downloads unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid settings form", http.StatusBadRequest)
		return
	}
	routes, err := settingsRoutes(r.PostForm.Get("strategy"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	settings, err := s.store.DownloadSettings()
	if err != nil {
		http.Error(w, "unable to read download settings", http.StatusInternalServerError)
		return
	}
	settings.Strategy = routes
	for _, field := range []struct {
		name   string
		values *[]string
	}{
		{"personal_proxy", &settings.PersonalProxy},
		{"public_proxy", &settings.PublicProxy},
	} {
		clear := r.PostForm.Get("clear_"+field.name) == "on"
		replacement := strings.TrimSpace(r.PostForm.Get(field.name))
		if clear && replacement != "" {
			http.Error(w, "choose either replacement or removal for "+field.name, http.StatusBadRequest)
			return
		}
		if clear {
			*field.values = nil
		} else if replacement != "" {
			*field.values, err = settingsProxies(replacement)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
	}
	usable := false
	for _, route := range settings.Strategy {
		if route == "direct" || (route == "personal_proxy" && len(settings.PersonalProxy) > 0) || (route == "random_proxy" && len(settings.PublicProxy) > 0) {
			usable = true
		}
	}
	if !usable {
		http.Error(w, "configure proxies for the selected routes or include direct", http.StatusBadRequest)
		return
	}
	if err := s.store.SaveDownloadSettings(settings); err != nil {
		http.Error(w, "unable to save download settings", http.StatusInternalServerError)
		return
	}
	s.downloads.mu.Lock()
	s.downloads.config.Strategy = settings.Strategy
	s.downloads.config.PersonalProxy = settings.PersonalProxy
	s.downloads.config.PublicProxy = settings.PublicProxy
	s.downloads.mu.Unlock()
	s.logger.Info("download settings saved", zap.Int("routes", len(routes)), zap.Int("personal_proxies", len(settings.PersonalProxy)), zap.Int("public_proxies", len(settings.PublicProxy)))
	if r.Header.Get("Accept") == "application/json" {
		w.WriteHeader(http.StatusNoContent)
	} else {
		http.Redirect(w, r, "/?source=settings&saved=1", http.StatusSeeOther)
	}
}
