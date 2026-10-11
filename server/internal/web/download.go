package web

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"trackharbor/download"
	"trackharbor/server/internal/config"
	"trackharbor/server/internal/store"
)

type downloadState struct {
	Running    bool   `json:"running"`
	Total      int    `json:"total"`
	Finished   int    `json:"finished"`
	Message    string `json:"message"`
	Source     string `json:"source,omitempty"`
	Artist     string `json:"artist,omitempty"`
	Title      string `json:"title,omitempty"`
	Stage      string `json:"stage,omitempty"`
	Bytes      int64  `json:"bytes,omitempty"`
	TotalBytes int64  `json:"total_bytes,omitempty"`
}

type downloadJob struct {
	store     *store.Store
	config    config.Config
	logger    *zap.Logger
	mu        sync.Mutex
	state     downloadState
	listeners map[chan []byte]struct{}
}

func newDownloadJob(db *store.Store, cfg config.Config, logger *zap.Logger) *downloadJob {
	return &downloadJob{store: db, config: cfg, logger: logger, listeners: make(map[chan []byte]struct{}), state: downloadState{Message: "Ready to download."}}
}

func (d *downloadJob) publish(update downloadState) {
	d.mu.Lock()
	if update.Running && update.Finished < d.state.Finished {
		update.Finished = d.state.Finished
	}
	d.state = update
	data, _ := json.Marshal(update)
	for ch := range d.listeners {
		select {
		case ch <- data:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- data:
			default:
			}
		}
	}
	d.mu.Unlock()
}

func (s *Server) startDownload(w http.ResponseWriter, r *http.Request) {
	if s.downloads == nil {
		http.Error(w, "downloads unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", 400)
		return
	}
	mode := r.PostForm.Get("mode")
	var tracks []store.Track
	var err error
	switch mode {
	case "all":
		tracks, err = s.store.ListAll()
	case "one":
		track, e := s.store.Track(r.PostForm.Get("source"), r.PostForm.Get("artist"), r.PostForm.Get("title"))
		err = e
		if e == nil {
			tracks = []store.Track{track}
		}
	default:
		http.Error(w, "invalid download selection", 400)
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "track not found", 404)
		return
	}
	if err != nil {
		http.Error(w, "unable to read library", 500)
		return
	}
	d := s.downloads
	d.mu.Lock()
	if d.state.Running {
		d.mu.Unlock()
		http.Error(w, "download already running", 409)
		return
	}
	d.state = downloadState{Running: true, Total: len(tracks), Message: "Download started."}
	d.mu.Unlock()
	d.publish(downloadState{Running: true, Total: len(tracks), Message: "Download started."})
	d.logger.Info("download started", zap.String("selection", mode), zap.Int("tracks", len(tracks)))
	go d.run(tracks)
	if r.Header.Get("Accept") == "application/json" {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	view := r.PostForm.Get("view")
	if view != "spotify" && view != "vk" && view != "music" {
		view = sourceName(r)
	}
	http.Redirect(w, r, "/?source="+view, http.StatusSeeOther)
}

func (d *downloadJob) run(tracks []store.Track) {
	ctx := context.Background()
	routes := &download.Routes{Order: d.config.Strategy, Personal: d.config.PersonalProxy, Public: d.config.PublicProxy}
	var finished, failed atomic.Int32
	process := func(t store.Track) {
		state := downloadState{Running: true, Total: len(tracks)}
		state.Source, state.Artist, state.Title = t.Source, t.Artist, t.Title
		state.Bytes, state.TotalBytes = 0, 0
		state.Stage = "checking"
		state.Message = "Checking saved file"
		d.publish(state)
		valid, err := d.saved(t)
		if err == nil && valid {
			state.Stage = "skipped"
			state.Message = "Already downloaded"
		} else {
			name := downloadFilename(t)
			path := filepath.Join(d.config.DownloadDir, t.Source, name)
			hash, saveErr := download.Save(ctx, download.Track{Source: t.Source, Artist: t.Artist, Title: t.Title, AudioURL: t.AudioURL, CoverURL: t.CoverURL}, path, routes, func(e download.Event) {
				state.Stage, state.Bytes, state.TotalBytes = e.Stage, e.Bytes, e.Total
				state.Message = "Downloading " + t.Source
				d.publish(state)
			})
			if saveErr == nil {
				saveErr = d.store.SaveDownload(t, path, hash)
			}
			if saveErr != nil {
				state.Stage = "failed"
				failed.Add(1)
				state.Message = "YouTube download failed: check yt-dlp, FFmpeg and server proxy settings."
				if t.Source == "vk" {
					state.Message = "VK download failed: refresh VK sync to renew audio URLs; check FFmpeg."
				}
				// Signed VK URLs and proxy passwords must never reach logs or browser.
			} else {
				state.Stage = "done"
				state.Message = "Saved MP3"
			}
		}
		state.Finished = int(finished.Add(1))
		d.publish(state)
	}
	var wg sync.WaitGroup
	for _, source := range []string{"vk", "spotify"} {
		queue := make(chan store.Track)
		workers := 10
		if source == "spotify" {
			workers = 50
		}
		for range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for t := range queue {
					process(t)
				}
			}()
		}
		go func(source string, jobs chan store.Track) {
			defer close(jobs)
			for _, t := range tracks {
				if t.Source == source {
					jobs <- t
				}
			}
		}(source, queue)
	}
	wg.Wait()
	d.publish(downloadState{Running: false, Total: len(tracks), Finished: int(finished.Load()), Stage: "finished", Message: fmt.Sprintf("Finished %d tracks; %d failed.", finished.Load(), failed.Load())})
	if failed.Load() > 0 {
		d.logger.Warn("download finished with failures", zap.Int("total", len(tracks)), zap.Int32("failed", failed.Load()))
	} else {
		d.logger.Info("download finished", zap.Int("total", len(tracks)))
	}
}

func downloadFilename(t store.Track) string {
	sum := sha256.Sum256([]byte(t.Source + "\x00" + t.Artist + "\x00" + t.Title))
	label := strings.Map(func(r rune) rune {
		if r < ' ' || strings.ContainsRune("/\\:", r) {
			return '-'
		}
		return r
	}, t.Artist+" - "+t.Title)
	if len(label) > 110 {
		var short strings.Builder
		for _, char := range label {
			if short.Len()+len(string(char)) > 110 {
				break
			}
			short.WriteRune(char)
		}
		label = short.String()
	}
	return label + "-" + hex.EncodeToString(sum[:8]) + ".mp3"
}

func (d *downloadJob) saved(t store.Track) (bool, error) {
	if t.FilePath == "" {
		return false, nil
	}
	stored, err := d.store.DownloadHash(t)
	if err != nil {
		return false, err
	}
	f, err := os.Open(t.FilePath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return false, nil
	}
	h := sha256.New()
	_, err = io.Copy(h, f)
	return err == nil && hex.EncodeToString(h.Sum(nil)) == stored, err
}

func (s *Server) downloadEvents(w http.ResponseWriter, r *http.Request) {
	if s.downloads == nil {
		http.Error(w, "downloads unavailable", 503)
		return
	}
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unavailable", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	d := s.downloads
	ch := make(chan []byte, 1)
	d.mu.Lock()
	d.listeners[ch] = struct{}{}
	snapshot, _ := json.Marshal(d.state)
	d.mu.Unlock()
	defer func() { d.mu.Lock(); delete(d.listeners, ch); d.mu.Unlock() }()
	if _, err := fmt.Fprintf(w, "data: %s\n\n", snapshot); err != nil {
		return
	}
	f.Flush()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case data := <-ch:
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return
			}
			f.Flush()
		case <-ticker.C:
			if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil {
				return
			}
			f.Flush()
		}
	}
}
