package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"

	"trackharbor/download"
	"trackharbor/internal/config"
	"trackharbor/internal/store"
)

type downloadState struct {
	Running    bool   `json:"running"`
	Stopped    bool   `json:"stopped"`
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
	queue     []store.Track
	active    map[string]bool
	tracks    map[string]downloadState
	finished  int
	failed    int
	stopped   bool
	cancel    context.CancelFunc
}

func newDownloadJob(db *store.Store, cfg config.Config, logger *zap.Logger) *downloadJob {
	return &downloadJob{store: db, config: cfg, logger: logger, listeners: make(map[chan []byte]struct{}), active: make(map[string]bool), tracks: make(map[string]downloadState), state: downloadState{Message: "Waiting for a library sync."}}
}

func (d *downloadJob) publish(update downloadState) {
	d.mu.Lock()
	d.publishLocked(update)
	d.mu.Unlock()
}

func (d *downloadJob) publishLocked(update downloadState) {
	update.Stopped = d.stopped
	if update.Running && update.Total < d.state.Total {
		update.Total = d.state.Total
	}
	if update.Running && update.Finished < d.state.Finished {
		update.Finished = d.state.Finished
	}
	d.state = update
	if update.Title != "" {
		d.tracks[trackKey(update.Source, update.Artist, update.Title)] = update
	}
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
}

func trackKey(source, artist, title string) string {
	return source + "\x00" + artist + "\x00" + title
}

func (d *downloadJob) enqueue(tracks []store.Track) {
	d.mu.Lock()
	if d.stopped {
		d.mu.Unlock()
		return
	}
	start := !d.state.Running
	var ctx context.Context
	if start {
		d.finished, d.failed = 0, 0
		d.state = downloadState{Message: "Checking downloaded files."}
		ctx, d.cancel = context.WithCancel(context.Background())
	}
	added := 0
	for _, t := range tracks {
		key := trackKey(t.Source, t.Artist, t.Title)
		if d.active[key] {
			continue
		}
		d.active[key] = true
		d.queue = append(d.queue, t)
		d.tracks[key] = downloadState{Source: t.Source, Artist: t.Artist, Title: t.Title, Stage: "queued"}
		added++
	}
	if added == 0 && !d.state.Running {
		d.cancel()
		d.cancel = nil
		d.publishLocked(downloadState{Message: "No tracks to download."})
		d.mu.Unlock()
		return
	}
	d.publishLocked(downloadState{Running: true, Total: d.state.Total + added, Finished: d.finished, Stage: "queued", Message: "Checking downloaded files."})
	d.mu.Unlock()
	if start {
		d.logger.Info("download check started", zap.Int("tracks", added))
		go d.runQueue(ctx)
	}
}

func (d *downloadJob) runQueue(ctx context.Context) {
	for {
		d.mu.Lock()
		if d.stopped {
			for _, t := range d.queue {
				key := trackKey(t.Source, t.Artist, t.Title)
				d.tracks[key] = downloadState{Source: t.Source, Artist: t.Artist, Title: t.Title, Stage: "stopped", Message: "Stopped"}
			}
			d.queue = nil
			clear(d.active)
			d.cancel = nil
			d.publishLocked(downloadState{Total: d.state.Total, Finished: d.finished, Stage: "stopped", Message: "Downloads stopped."})
			d.mu.Unlock()
			d.logger.Info("downloads stopped")
			return
		}
		if len(d.queue) == 0 {
			d.cancel()
			d.cancel = nil
			message := fmt.Sprintf("Checked %d tracks; %d failed.", d.finished, d.failed)
			d.publishLocked(downloadState{Total: d.finished, Finished: d.finished, Stage: "finished", Message: message})
			failed, total := d.failed, d.finished
			d.mu.Unlock()
			if failed > 0 {
				d.logger.Warn("download check finished with failures", zap.Int("total", total), zap.Int("failed", failed))
			} else {
				d.logger.Info("download check finished", zap.Int("total", total))
			}
			return
		}
		tracks := d.queue
		d.queue = nil
		d.mu.Unlock()
		d.runBatch(ctx, tracks)
	}
}

func (d *downloadJob) runBatch(ctx context.Context, tracks []store.Track) {
	routes := &download.Routes{Order: d.config.Strategy, Personal: d.config.PersonalProxy, Public: d.config.PublicProxy}
	process := func(t store.Track) {
		state := downloadState{Running: true}
		state.Source, state.Artist, state.Title = t.Source, t.Artist, t.Title
		if ctx.Err() != nil {
			state.Stage, state.Message = "stopped", "Stopped"
			d.mu.Lock()
			d.publishLocked(state)
			delete(d.active, trackKey(t.Source, t.Artist, t.Title))
			d.mu.Unlock()
			return
		}
		state.Bytes, state.TotalBytes = 0, 0
		state.Stage = "checking"
		state.Message = "Checking saved file"
		d.publish(state)
		valid, err := d.saved(t)
		if ctx.Err() != nil {
			state.Stage, state.Message = "stopped", "Stopped"
		} else if err == nil && valid {
			state.Stage = "skipped"
			state.Message = "Already downloaded"
		} else {
			directory := filepath.Join(d.config.DownloadDir, "youtube")
			id := ""
			if t.Source == "vk" {
				directory = filepath.Join(d.config.DownloadDir, "vk")
				id = fmt.Sprintf("%d_%d", t.OwnerID, t.AudioID)
			}
			fields := []zap.Field{zap.String("source", t.Source), zap.String("artist", t.Artist), zap.String("title", t.Title)}
			d.logger.Info("track download started", fields...)
			path, hash, saveErr := download.Save(ctx, download.Track{Source: t.Source, SourceID: id, Artist: t.Artist, Title: t.Title, AudioURL: t.AudioURL, CoverURL: t.CoverURL}, directory, t.FilePath, routes, func(e download.Event) {
				state.Stage, state.Bytes, state.TotalBytes = e.Stage, e.Bytes, e.Total
				state.Message = "Downloading " + t.Source
				d.publish(state)
			})
			if saveErr == nil {
				saveErr = d.store.SaveDownload(t, path, hash)
			}
			if ctx.Err() != nil && saveErr != nil {
				state.Stage = "stopped"
				state.Message = "Stopped"
			} else if saveErr != nil {
				state.Stage = "failed"
				state.Message = "YouTube download failed: check yt-dlp, FFmpeg and server proxy settings."
				if t.Source == "vk" {
					state.Message = "VK download failed: refresh VK sync to renew audio URLs; check FFmpeg."
				}
				// Signed VK URLs and proxy passwords must never reach logs or browser.
			} else {
				state.Stage = "done"
				state.Message = "Saved MP3"
			}
			switch state.Stage {
			case "done":
				d.logger.Info("track download finished", fields...)
			case "stopped":
				d.logger.Info("track download stopped", fields...)
			case "failed":
				d.logger.Warn("track download failed", fields...)
			}
		}
		d.mu.Lock()
		if state.Stage == "failed" {
			d.failed++
		}
		if state.Stage != "stopped" {
			d.finished++
		}
		state.Finished = d.finished
		state.Total = d.state.Total
		d.publishLocked(state)
		delete(d.active, trackKey(t.Source, t.Artist, t.Title))
		d.mu.Unlock()
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
}

func (s *Server) startDownloads(w http.ResponseWriter, r *http.Request) {
	if s.downloads == nil {
		http.Error(w, "downloads unavailable", http.StatusServiceUnavailable)
		return
	}
	tracks, err := s.store.ListAll()
	if err != nil {
		http.Error(w, "unable to read library", http.StatusInternalServerError)
		return
	}
	d := s.downloads
	d.mu.Lock()
	if d.state.Running {
		d.mu.Unlock()
		http.Error(w, "downloads are already running or stopping", http.StatusConflict)
		return
	}
	d.stopped = false
	d.mu.Unlock()
	d.enqueue(tracks)
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) stopDownloads(w http.ResponseWriter, r *http.Request) {
	if s.downloads == nil {
		http.Error(w, "downloads unavailable", http.StatusServiceUnavailable)
		return
	}
	d := s.downloads
	d.mu.Lock()
	d.stopped = true
	if d.cancel != nil {
		d.cancel()
	}
	stage, message := "stopped", "Downloads stopped."
	if d.state.Running {
		stage, message = "stopping", "Stopping downloads…"
	}
	d.publishLocked(downloadState{Running: d.state.Running, Total: d.state.Total, Finished: d.finished, Stage: stage, Message: message})
	d.mu.Unlock()
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) downloadStatus(w http.ResponseWriter, r *http.Request) {
	if s.downloads == nil {
		http.Error(w, "downloads unavailable", http.StatusServiceUnavailable)
		return
	}
	tracks, err := s.store.ListAll()
	if err != nil {
		http.Error(w, "unable to read library", http.StatusInternalServerError)
		return
	}
	type item struct {
		Source     string `json:"source"`
		Artist     string `json:"artist"`
		Title      string `json:"title"`
		Stage      string `json:"stage"`
		Message    string `json:"message,omitempty"`
		Bytes      int64  `json:"bytes,omitempty"`
		TotalBytes int64  `json:"total_bytes,omitempty"`
	}
	d := s.downloads
	d.mu.Lock()
	items := make([]item, 0, len(tracks))
	for _, t := range tracks {
		stage := "waiting"
		if t.FilePath != "" {
			stage = "saved"
		}
		status := d.tracks[trackKey(t.Source, t.Artist, t.Title)]
		if status.Stage != "" {
			stage = status.Stage
		}
		items = append(items, item{Source: t.Source, Artist: t.Artist, Title: t.Title, Stage: stage, Message: status.Message, Bytes: status.Bytes, TotalBytes: status.TotalBytes})
	}
	state := d.state
	d.mu.Unlock()
	s.mu.Lock()
	syncing := s.status["spotify"] == "Syncing…" || s.status["vk"] == "Syncing…"
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(struct {
		State   downloadState `json:"state"`
		Tracks  []item        `json:"tracks"`
		Syncing bool          `json:"syncing"`
	}{state, items, syncing}); err != nil {
		s.logger.Error("write download status failed")
	}
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
