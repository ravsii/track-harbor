package web

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"trackharbor/server/internal/library"
	"trackharbor/server/internal/store"
	"trackharbor/vk"
)

//go:embed assets/*
var assets embed.FS

type Server struct {
	store  *store.Store
	mu     sync.Mutex
	status map[string]string
}

func New(db *store.Store) http.Handler {
	s := &Server{store: db, status: map[string]string{}}
	mux := http.NewServeMux()
	static, _ := fs.Sub(assets, "assets")
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServerFS(static)))
	mux.HandleFunc("GET /", s.page)
	mux.HandleFunc("GET /status", s.currentStatus)
	mux.HandleFunc("POST /connect", s.connect)
	mux.HandleFunc("POST /sync", s.syncSource)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; form-action 'self'; base-uri 'none'")
		mux.ServeHTTP(w, r)
	})
}

func sourceName(r *http.Request) string {
	if r.FormValue("source") == "vk" {
		return "vk"
	}
	return "spotify"
}

func selectedSource(r *http.Request) (string, bool) {
	source := r.PostForm.Get("source")
	return source, source == "spotify" || source == "vk"
}

type pageData struct {
	Source, Name, Status, AuthURL string
	Connected                     bool
	Tracks                        []store.Track
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	source := sourceName(r)
	connected, err := s.store.Connected(source)
	if err != nil {
		http.Error(w, "unable to read library", 500)
		return
	}
	tracks, err := s.store.List(source)
	if err != nil {
		http.Error(w, "unable to read library", 500)
		return
	}
	name := "Spotify"
	if source == "vk" {
		name = "VK Music"
	}
	s.mu.Lock()
	status := s.status[source]
	s.mu.Unlock()
	data := pageData{Source: source, Name: name, Connected: connected, Tracks: tracks, Status: status, AuthURL: vk.AuthorizationURL()}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pageTemplate.Execute(w, data); err != nil {
		log.Print("render web page failed")
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
		message := fmt.Sprintf("Synced %d tracks. Refresh this view to see them.", count)
		if err != nil {
			// Provider errors may include remote URLs or credentials: never echo them.
			message = "Sync failed. Previous tracks were kept. Check credentials and try again."
			if errors.Is(err, context.DeadlineExceeded) {
				message = "Sync timed out. Previous tracks were kept."
			}
			log.Printf("%s sync failed (details withheld to protect credentials)", source)
		}
		s.mu.Lock()
		s.status[source] = message
		s.mu.Unlock()
	}()
	http.Redirect(w, r, "/?source="+source, http.StatusSeeOther)
}
