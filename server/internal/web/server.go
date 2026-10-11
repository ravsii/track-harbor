package web

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"trackharbor/server/internal/config"
	"trackharbor/server/internal/library"
	"trackharbor/server/internal/store"
	"trackharbor/spotify"
	"trackharbor/vk"
)

//go:embed assets/*
var assets embed.FS

type Server struct {
	store      *store.Store
	logger     *zap.Logger
	mu         sync.Mutex
	status     map[string]string
	pingStatus map[string]string
	downloads  *downloadJob
}

func New(db *store.Store, logger *zap.Logger, configs ...config.Config) http.Handler {
	s := &Server{store: db, logger: logger, status: map[string]string{}, pingStatus: map[string]string{}}
	if len(configs) != 0 {
		s.downloads = newDownloadJob(db, configs[0], logger)
	}
	mux := http.NewServeMux()
	static, _ := fs.Sub(assets, "assets")
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServerFS(static)))
	mux.HandleFunc("GET /", s.page)
	mux.HandleFunc("GET /status", s.currentStatus)
	mux.HandleFunc("GET /ping-status", s.currentPingStatus)
	mux.HandleFunc("POST /connect", s.connect)
	mux.HandleFunc("POST /ping", s.pingSource)
	mux.HandleFunc("POST /sync", s.syncSource)
	mux.HandleFunc("GET /download/status", s.downloadStatus)
	mux.HandleFunc("GET /download/events", s.downloadEvents)
	mux.HandleFunc("GET /audio", s.playAudio)
	mux.HandleFunc("GET /audio/tracks", s.playableTracks)
	return logRequests(logger, mux, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if host != "localhost" && host != "127.0.0.1" && host != "[::1]" && host != "::1" {
			http.Error(w, "local access only", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost {
			origin := r.Header.Get("Origin")
			u, err := url.Parse(origin)
			if origin == "" || err != nil || u.Scheme != "http" || u.Host != r.Host {
				http.Error(w, "cross-origin request rejected", http.StatusForbidden)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; form-action 'self'; base-uri 'none'")
		mux.ServeHTTP(w, r)
	}))
}

func (s *Server) playableTracks(w http.ResponseWriter, r *http.Request) {
	if s.downloads == nil {
		http.NotFound(w, r)
		return
	}
	tracks, err := s.store.ListPlayable()
	if err != nil {
		http.Error(w, "unable to read saved tracks", http.StatusInternalServerError)
		return
	}
	type playableTrack struct {
		Source string `json:"source"`
		Artist string `json:"artist"`
		Title  string `json:"title"`
	}
	items := make([]playableTrack, 0, len(tracks))
	for _, t := range tracks {
		items = append(items, playableTrack{t.Source, t.Artist, t.Title})
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(items); err != nil {
		s.logger.Error("write saved tracks failed")
	}
}

func sourceName(r *http.Request) string {
	if r.FormValue("source") == "vk" {
		return "vk"
	}
	if r.FormValue("source") == "spotify" {
		return "spotify"
	}
	if r.FormValue("source") == "downloads" {
		return "downloads"
	}
	return "music"
}

func selectedSource(r *http.Request) (string, bool) {
	source := r.PostForm.Get("source")
	return source, source == "spotify" || source == "vk"
}

type pageData struct {
	Source, Name, Status, PingStatus, AuthURL string
	Connected                                 bool
	Tracks                                    []store.Track
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	source := sourceName(r)
	data := pageData{Source: source, Name: "Music"}
	var err error
	if source == "downloads" {
		data.Name = "Downloads"
	} else if source == "music" {
		data.Tracks, err = s.store.ListAll()
	} else {
		data.Connected, err = s.store.Connected(source)
		if err == nil {
			data.Tracks, err = s.store.List(source)
		}
		if source == "vk" {
			data.Name = "VK Music"
		} else {
			data.Name = "Spotify"
		}
		s.mu.Lock()
		data.Status = s.status[source]
		data.PingStatus = s.pingStatus[source]
		s.mu.Unlock()
		data.AuthURL = vk.AuthorizationURL()
	}
	if err != nil {
		http.Error(w, "unable to read library", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pageTemplate.Execute(w, data); err != nil {
		s.logger.Error("render web page failed")
	}
}

func (s *Server) currentStatus(w http.ResponseWriter, r *http.Request) {
	source := sourceName(r)
	s.mu.Lock()
	status := s.status[source]
	s.mu.Unlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if status == "" {
		status = "Ready to sync."
	}
	template.HTMLEscape(w, []byte(status))
}

func (s *Server) currentPingStatus(w http.ResponseWriter, r *http.Request) {
	source := sourceName(r)
	s.mu.Lock()
	status := s.pingStatus[source]
	s.mu.Unlock()
	if status == "" {
		status = "Not checked yet."
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	template.HTMLEscape(w, []byte(status))
}

func (s *Server) connect(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", 400)
		return
	}
	source, ok := selectedSource(r)
	if !ok {
		http.Error(w, "unknown source", 400)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status[source] == "Syncing…" {
		http.Error(w, "sync in progress", http.StatusConflict)
		return
	}
	var first, second string
	if source == "spotify" {
		first, second = spotifyHeaders(r.PostForm.Get("headers"))
		if !strings.HasPrefix(first, "Bearer ") || len(first) < len("Bearer ")+1 || second == "" {
			http.Error(w, "paste request headers containing authorization: Bearer … and client-token", 400)
			return
		}
	} else {
		var err error
		first, err = vk.ParseToken(r.PostForm.Get("token"))
		if err != nil {
			http.Error(w, "paste a valid VK token or redirect URL", 400)
			return
		}
	}
	if err := s.store.SaveCredentials(source, first, second); err != nil {
		http.Error(w, "could not save credentials", 500)
		return
	}
	s.status[source] = "Credentials saved. Click Sync library to import tracks."
	s.pingStatus[source] = "Not checked yet."
	s.logger.Info("source credentials saved", zap.String("source", source))
	http.Redirect(w, r, "/?source="+source, http.StatusSeeOther)
}

func (s *Server) pingSource(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", 400)
		return
	}
	source, ok := selectedSource(r)
	if !ok {
		http.Error(w, "unknown source", 400)
		return
	}
	first, second, err := s.store.Credentials(source)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "connect source first", 400)
		return
	}
	if err != nil {
		http.Error(w, "unable to read credentials", 500)
		return
	}
	s.mu.Lock()
	if s.pingStatus[source] == "Checking…" {
		s.mu.Unlock()
		http.Redirect(w, r, "/?source="+source, http.StatusSeeOther)
		return
	}
	s.pingStatus[source] = "Checking…"
	s.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		var pingErr error
		if source == "spotify" {
			pingErr = spotify.Ping(ctx, first, second)
		} else {
			client := vk.NewClient(first)
			defer client.Close()
			pingErr = client.Ping(ctx)
		}
		message := "Connection verified."
		switch {
		case errors.Is(pingErr, spotify.ErrUnauthorized), errors.Is(pingErr, vk.ErrUnauthorized):
			message = "Token rejected. Reconnect this source with fresh credentials."
		case pingErr != nil:
			message = "Could not verify connection. Try again later; the token may still be valid."
		}
		s.mu.Lock()
		if s.pingStatus[source] == "Checking…" {
			s.pingStatus[source] = message
		}
		s.mu.Unlock()
		result := "verified"
		if errors.Is(pingErr, spotify.ErrUnauthorized) || errors.Is(pingErr, vk.ErrUnauthorized) {
			result = "rejected"
		} else if pingErr != nil {
			result = "unavailable"
		}
		s.logger.Info("source connection checked", zap.String("source", source), zap.String("result", result))
	}()
	http.Redirect(w, r, "/?source="+source, http.StatusSeeOther)
}

// spotifyHeaders extracts only the two credentials needed for sync from a
// DevTools header dump. Other headers (including cookies) are never persisted.
func spotifyHeaders(input string) (authorization, clientToken string) {
	for _, line := range strings.Split(input, "\n") {
		name, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "authorization":
			authorization = value
		case "client-token":
			clientToken = value
		}
	}
	return authorization, clientToken
}

func (s *Server) syncSource(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", 400)
		return
	}
	source, ok := selectedSource(r)
	if !ok {
		http.Error(w, "unknown source", 400)
		return
	}
	connected, err := s.store.Connected(source)
	if err != nil {
		http.Error(w, "unable to read credentials", 500)
		return
	}
	if !connected {
		http.Error(w, "connect source first", 400)
		return
	}
	s.mu.Lock()
	if s.status[source] == "Syncing…" {
		s.mu.Unlock()
		http.Redirect(w, r, "/?source="+source, http.StatusSeeOther)
		return
	}
	s.status[source] = "Syncing…"
	s.mu.Unlock()
	s.logger.Info("library sync started", zap.String("source", source))
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		var count int
		var err error
		if source == "spotify" {
			count, err = library.SyncSpotify(ctx, s.store)
		} else {
			count, err = library.SyncVK(ctx, s.store)
		}
		message := fmt.Sprintf("Synced %d tracks. Checking downloads in the background.", count)
		if err != nil {
			// Provider errors may include remote URLs or credentials: never echo them.
			message = "Sync failed. Previous tracks were kept. Check credentials and try again."
			if errors.Is(err, context.DeadlineExceeded) {
				message = "Sync timed out. Previous tracks were kept."
			}
			s.logger.Error("library sync failed", zap.String("source", source), zap.Bool("timed_out", errors.Is(err, context.DeadlineExceeded)))
		} else {
			s.logger.Info("library sync finished", zap.String("source", source), zap.Int("tracks", count))
			if s.downloads != nil {
				tracks, listErr := s.store.List(source)
				if listErr != nil {
					message = "Library synced, but downloads could not be queued. Try syncing again."
					s.logger.Error("queue synced tracks failed", zap.String("source", source))
				} else {
					s.downloads.enqueue(tracks)
				}
			}
		}
		s.mu.Lock()
		s.status[source] = message
		s.mu.Unlock()
	}()
	http.Redirect(w, r, "/?source="+source, http.StatusSeeOther)
}
