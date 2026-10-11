package web

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

type personalProxyView struct {
	ID           int64
	Host, Masked string
}

func maskPersonalProxy(address string, parsed *url.URL) string {
	if parsed.User == nil {
		return address
	}
	start := strings.Index(address, "://")
	if start < 0 {
		return address
	}
	authority := address[start+3:]
	if end := strings.IndexAny(authority, "/?#"); end >= 0 {
		authority = authority[:end]
	}
	at := strings.LastIndex(authority, "@")
	if at < 0 {
		return address
	}
	masked := "********"
	if strings.Contains(authority[:at], ":") {
		masked += ":********"
	}
	return address[:start+3] + masked + address[start+3+at:]
}

func proxyID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if id <= 0 {
		return 0, fmt.Errorf("invalid proxy ID")
	}
	return id, err
}

func (s *Server) refreshPersonalRoutes(settings []string) {
	s.downloads.mu.Lock()
	s.downloads.config.PersonalProxy = append([]string(nil), settings...)
	s.downloads.mu.Unlock()
}

func (s *Server) addPersonalProxy(w http.ResponseWriter, r *http.Request) {
	if s.downloads == nil {
		http.Error(w, "downloads unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid proxy form", http.StatusBadRequest)
		return
	}
	address := strings.TrimSpace(r.PostForm.Get("address"))
	if proxies, err := settingsProxies(address); err != nil || len(proxies) != 1 {
		http.Error(w, "enter one valid proxy URL", http.StatusBadRequest)
		return
	}
	parsed, err := url.Parse(address)
	if err != nil || parsed.Port() == "" {
		http.Error(w, "enter a proxy URL with a host and port", http.StatusBadRequest)
		return
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		http.Error(w, "enter a valid proxy port", http.StatusBadRequest)
		return
	}
	if err := probePersonalProxy(r.Context(), address); err != nil {
		http.Error(w, "proxy could not reach YouTube; check its address, credentials and HTTPS access", http.StatusBadRequest)
		return
	}
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	proxies, err := s.store.PersonalProxies()
	if err != nil {
		http.Error(w, "unable to read personal proxies", http.StatusInternalServerError)
		return
	}
	if len(proxies) >= 100 {
		http.Error(w, "use no more than 100 personal proxies", http.StatusBadRequest)
		return
	}
	for _, proxy := range proxies {
		if proxy.Address == address {
			http.Error(w, "proxy already saved", http.StatusBadRequest)
			return
		}
	}
	if err := s.store.AddPersonalProxy(address); err != nil {
		http.Error(w, "unable to save personal proxy", http.StatusInternalServerError)
		return
	}
	settings, err := s.store.DownloadSettings()
	if err != nil {
		http.Error(w, "proxy saved, but unable to refresh download settings", http.StatusInternalServerError)
		return
	}
	s.refreshPersonalRoutes(settings.PersonalProxy)
	s.logger.Info("personal proxy added")
	settingsResponse(w, r)
}

func (s *Server) deletePersonalProxy(w http.ResponseWriter, r *http.Request) {
	if s.downloads == nil {
		http.Error(w, "downloads unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil || r.PostForm.Get("confirm") != "yes" {
		http.Error(w, "confirm proxy removal before submitting", http.StatusBadRequest)
		return
	}
	id, err := proxyID(r)
	if err != nil {
		http.Error(w, "invalid proxy ID", http.StatusBadRequest)
		return
	}
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	if _, err := s.store.PersonalProxy(id); err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, "unable to read personal proxy", http.StatusInternalServerError)
		return
	}
	settings, err := s.store.DownloadSettings()
	if err != nil {
		http.Error(w, "unable to read download settings", http.StatusInternalServerError)
		return
	}
	if len(settings.PersonalProxy) == 1 && !settingsUsable(settings.Strategy, nil, settings.PublicProxy) {
		http.Error(w, "add another proxy or enable a usable route before removing the last personal proxy", http.StatusBadRequest)
		return
	}
	deleted, err := s.store.DeletePersonalProxy(id)
	if err != nil {
		http.Error(w, "unable to remove personal proxy", http.StatusInternalServerError)
		return
	}
	if !deleted {
		http.Error(w, "proxy not found", http.StatusNotFound)
		return
	}
	settings, err = s.store.DownloadSettings()
	if err != nil {
		http.Error(w, "proxy removed, but unable to refresh download settings", http.StatusInternalServerError)
		return
	}
	s.refreshPersonalRoutes(settings.PersonalProxy)
	s.logger.Info("personal proxy removed")
	settingsResponse(w, r)
}

func (s *Server) personalProxyAddress(w http.ResponseWriter, r *http.Request) {
	id, err := proxyID(r)
	if err != nil {
		http.Error(w, "invalid proxy ID", http.StatusBadRequest)
		return
	}
	address, err := s.store.PersonalProxy(id)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "unable to read personal proxy", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(struct {
		Address string `json:"address"`
	}{Address: address}); err != nil {
		s.logger.Error("write personal proxy address failed")
	}
}

func settingsResponse(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Accept") == "application/json" {
		w.WriteHeader(http.StatusNoContent)
	} else {
		http.Redirect(w, r, "/?source=settings&saved=1", http.StatusSeeOther)
	}
}

func settingsUsable(routes, personal, public []string) bool {
	for _, route := range routes {
		if route == "direct" || (route == "personal_proxy" && len(personal) > 0) || (route == "random_proxy" && len(public) > 0) {
			return true
		}
	}
	return false
}

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
	clear := r.PostForm.Get("clear_public_proxy") == "on"
	replacement := strings.TrimSpace(r.PostForm.Get("public_proxy"))
	if clear && replacement != "" {
		http.Error(w, "choose either replacement or removal for public_proxy", http.StatusBadRequest)
		return
	}
	if clear {
		settings.PublicProxy = nil
	} else if replacement != "" {
		settings.PublicProxy, err = settingsProxies(replacement)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if !settingsUsable(settings.Strategy, settings.PersonalProxy, settings.PublicProxy) {
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
	settingsResponse(w, r)
}
