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
	rawSources, err := s.store.Setting("proxy_sources")
	if err != nil {
		http.Error(w, "unable to read proxy sources", http.StatusInternalServerError)
		return
	}
	if len(settingsLines(rawSources)) == 0 {
		http.Error(w, "save at least one source URL before importing", http.StatusBadRequest)
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
	count, err := s.store.ProxyCount("public")
	if err != nil {
		http.Error(w, "unable to count public proxies", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(struct {
		Running bool   `json:"running"`
		Message string `json:"message"`
		Count   int    `json:"count"`
	}{state.Running, state.Message, count}); err != nil {
		s.logger.Error("write public proxy import status failed")
	}
}

type proxyProbeResult struct {
	address   string
	checkedAt time.Time
	success   bool
}

func (s *Server) runPublicProxyImport() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	rawSources, err := s.store.Setting("proxy_sources")
	if err != nil {
		s.setProxyImport("Could not read proxy sources. Try again later.", false)
		s.logger.Error("read proxy sources failed")
		return
	}
	addresses, sources, err := fetchPublicProxies(ctx, settingsLines(rawSources))
	if err != nil {
		s.setProxyImport("Could not load public proxy lists. Saved proxies were kept; try again later.", false)
		s.logger.Warn("public proxy sources unavailable")
		return
	}
	added, err := s.store.AddPublicProxies(addresses)
	if err != nil {
		s.setProxyImport("Could not save discovered proxies. Try again later.", false)
		s.logger.Error("save discovered public proxies failed")
		return
	}
	s.setProxyImport(fmt.Sprintf("Saved proxies. Checking 0/%d against YouTube…", len(addresses)), true)
	jobs := make(chan string)
	results := make(chan proxyProbeResult, publicProbeWorkers)
	var workers sync.WaitGroup
	for range min(publicProbeWorkers, len(addresses)) {
		workers.Go(func() {
			for address := range jobs {
				if ctx.Err() != nil {
					return
				}
				success := probeYouTube(ctx, address, 5*time.Second) == nil
				results <- proxyProbeResult{address: address, checkedAt: time.Now(), success: success}
			}
		})
	}
	go func() {
		defer close(jobs)
		for _, address := range addresses {
			select {
			case jobs <- address:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { workers.Wait(); close(results) }()
	checked, successful := 0, 0
	saveFailed := false
	for result := range results {
		if saveFailed {
			continue
		}
		if err := s.store.RecordProxyCheck(result.address, result.checkedAt, result.success); err != nil {
			saveFailed = true
			cancel()
			continue
		}
		checked++
		if result.success {
			successful++
		}
		if checked%250 == 0 || checked == len(addresses) {
			s.setProxyImport(fmt.Sprintf("Checking %d/%d public proxies against YouTube…", checked, len(addresses)), true)
		}
	}
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	current, err := s.store.DownloadSettings()
	if err != nil {
		s.setProxyImport("Proxies saved, but could not refresh download routes. Restart the server to reload them.", false)
		s.logger.Error("refresh imported public proxies failed")
		return
	}
	s.downloads.mu.Lock()
	s.downloads.config.PublicProxy = append([]string(nil), current.PublicProxy...)
	s.downloads.mu.Unlock()
	if saveFailed {
		s.setProxyImport("Proxies saved, but some check results could not be saved. Try importing again.", false)
		s.logger.Error("save public proxy check result failed")
		return
	}
	if ctx.Err() != nil {
		s.setProxyImport(fmt.Sprintf("Check timed out after %d/%d proxies. All discovered proxies stay saved.", checked, len(addresses)), false)
		s.logger.Warn("public proxy import timed out")
		return
	}
	s.setProxyImport(fmt.Sprintf("Import finished: %d discovered, %d added; %d/%d checks succeeded from %d sources. Failed proxies stay saved.", len(addresses), added, successful, checked, sources), false)
	s.logger.Info("public proxy import finished", zap.Int("sources", sources), zap.Int("checked", checked), zap.Int("successful", successful), zap.Int("added", added))
}
