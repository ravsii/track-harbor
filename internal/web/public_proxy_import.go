package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	publicProbeWorkers = 500
)

type proxyImportState struct {
	Running bool   `json:"running"`
	Message string `json:"message"`
}

func (s *Server) setProxyImport(message string, running bool) {
	s.importMu.Lock()
	s.proxyImport = proxyImportState{Running: running, Message: message}
	s.importMu.Unlock()
}

func (s *Server) importPublicProxies(w http.ResponseWriter, r *http.Request) {
	if s.downloads == nil {
		http.Error(w, "downloads unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil || r.PostForm.Get("confirm") != "yes" {
		http.Error(w, "confirm public proxy import before submitting", http.StatusBadRequest)
		return
	}
	s.importMu.Lock()
	if s.proxyImport.Running {
		s.importMu.Unlock()
		http.Error(w, "public proxy import is already running", http.StatusConflict)
		return
	}
	s.proxyImport = proxyImportState{Running: true, Message: "Fetching public proxy lists…"}
	s.importMu.Unlock()
	s.logger.Info("public proxy import started")
	go s.runPublicProxyImport()
	if r.Header.Get("Accept") == "application/json" {
		w.WriteHeader(http.StatusAccepted)
	} else {
		http.Redirect(w, r, "/?source=settings", http.StatusSeeOther)
	}
}

func (s *Server) publicProxyImportStatus(w http.ResponseWriter, r *http.Request) {
	s.importMu.Lock()
	state := s.proxyImport
	s.importMu.Unlock()
	settings, err := s.store.DownloadSettings()
	if err != nil {
		http.Error(w, "unable to read download settings", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(struct {
		Running bool   `json:"running"`
		Message string `json:"message"`
		Count   int    `json:"count"`
	}{state.Running, state.Message, len(settings.PublicProxy)}); err != nil {
		s.logger.Error("write public proxy import status failed")
	}
}

type proxyProbeResult struct {
	key     string
	address string // Empty when every protocol for this host:port failed.
}

func (s *Server) runPublicProxyImport() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	groups, sources, err := fetchPublicProxies(ctx)
	if err != nil {
		s.setProxyImport("Could not load public proxy lists. Saved proxies were kept; try again later.", false)
		s.logger.Warn("public proxy sources unavailable")
		return
	}
	s.setProxyImport(fmt.Sprintf("Checking 0/%d public proxies against YouTube…", len(groups)), true)
	jobs := make(chan publicProxyGroup)
	results := make(chan proxyProbeResult, publicProbeWorkers)
	var workers sync.WaitGroup
	for range min(publicProbeWorkers, len(groups)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for group := range jobs {
				result := proxyProbeResult{key: group.key}
				for _, address := range group.candidates {
					if ctx.Err() != nil {
						return
					}
					if probeYouTube(ctx, address, time.Second) == nil {
						result.address = address
						break
					}
				}
				select {
				case results <- result:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, group := range groups {
			select {
			case jobs <- group:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { workers.Wait(); close(results) }()
	checked := make(map[string]string, len(groups))
	for result := range results {
		checked[result.key] = result.address
		if len(checked)%250 == 0 || len(checked) == len(groups) {
			s.setProxyImport(fmt.Sprintf("Checking %d/%d public proxies against YouTube…", len(checked), len(groups)), true)
		}
	}
	if ctx.Err() != nil {
		s.setProxyImport("Public proxy check timed out. Saved proxies were kept; try again later.", false)
		s.logger.Warn("public proxy import timed out")
		return
	}
	if len(checked) == 0 {
		s.setProxyImport("No public proxies could be checked. Saved proxies were kept.", false)
		return
	}
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	current, err := s.store.DownloadSettings()
	if err != nil {
		s.setProxyImport("Could not read saved public proxies. Try again later.", false)
		s.logger.Error("read public proxies for import failed")
		return
	}
	updated := make([]string, 0, len(current.PublicProxy)+len(groups))
	seen := make(map[string]bool)
	replaced, removed, added := 0, 0, 0
	for _, address := range current.PublicProxy {
		key := proxyHostPort(address)
		if key == "" {
			key = address
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		if replacement, found := checked[key]; found {
			if replacement == "" {
				removed++
				continue
			}
			updated = append(updated, replacement)
			replaced++
		} else {
			updated = append(updated, address)
		}
	}
	for _, group := range groups {
		if seen[group.key] || checked[group.key] == "" {
			continue
		}
		seen[group.key] = true
		updated = append(updated, checked[group.key])
		added++
	}
	if !settingsUsable(current.Strategy, current.PersonalProxy, updated) {
		s.setProxyImport("Import would leave no usable download route. Saved proxies were kept.", false)
		return
	}
	current.PublicProxy = updated
	if err := s.store.SaveDownloadSettings(current); err != nil {
		s.setProxyImport("Could not save imported proxies. Previous settings were kept.", false)
		s.logger.Error("save imported public proxies failed")
		return
	}
	s.downloads.mu.Lock()
	s.downloads.config.PublicProxy = append([]string(nil), updated...)
	s.downloads.mu.Unlock()
	s.setProxyImport(fmt.Sprintf("Import finished: %d added, %d replaced, %d removed; %d saved.", added, replaced, removed, len(updated)), false)
	s.logger.Info("public proxy import finished", zap.Int("sources", sources), zap.Int("checked", len(checked)), zap.Int("added", added), zap.Int("replaced", replaced), zap.Int("removed", removed))
}
