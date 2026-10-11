package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"trackharbor/internal/store"
)

const playerLease = 10 * time.Second

type playerTrack struct {
	Source string `json:"source"`
	Artist string `json:"artist"`
	Title  string `json:"title"`
}

type playerSnapshot struct {
	Track    *playerTrack `json:"track"`
	Position float64      `json:"position"`
	Playing  bool         `json:"playing"`
	Owner    string       `json:"owner"`
	Pending  string       `json:"pending"`
	Revision uint64       `json:"revision"`
	Shuffle  bool         `json:"shuffle"`
}

type playerHub struct {
	mu         sync.Mutex
	state      playerSnapshot
	lease      time.Time
	reportedAt time.Time
	pending    *playerTrack
	history    []playerTrack
	listeners  map[chan struct{}]struct{}
}

func (p *playerHub) notifyLocked() {
	for listener := range p.listeners {
		select {
		case listener <- struct{}{}:
		default:
		}
	}
}

func (p *playerHub) expireLocked() {
	if p.state.Owner == "" || time.Now().Before(p.lease) {
		return
	}
	p.state.Position = p.positionLocked()
	p.state.Owner = p.state.Pending
	if p.state.Owner != "" && p.pending != nil {
		p.selectLocked(*p.pending)
		p.state.Playing = true
	} else {
		p.state.Playing = false
	}
	p.state.Pending = ""
	p.pending = nil
	p.lease = time.Now().Add(playerLease)
	p.reportedAt = time.Now()
	p.state.Revision++
	p.notifyLocked()
}

func (p *playerHub) selectLocked(track playerTrack) {
	if p.state.Track == nil || *p.state.Track != track {
		if p.state.Track != nil {
			p.history = append(p.history, *p.state.Track)
			if len(p.history) > 100 {
				p.history = p.history[len(p.history)-100:]
			}
		}
		p.state.Track = &track
		p.state.Position = 0
		p.reportedAt = time.Now()
	}
}

func (p *playerHub) positionLocked() float64 {
	if p.state.Playing && p.state.Owner != "" && !p.reportedAt.IsZero() {
		return p.state.Position + max(0, time.Since(p.reportedAt).Seconds())
	}
	return p.state.Position
}

func (p *playerHub) snapshot() playerSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.expireLocked()
	state := p.state
	state.Position = p.positionLocked()
	if state.Track != nil {
		track := *state.Track
		state.Track = &track
	}
	return state
}

func (s *Server) playerState(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(s.player.snapshot()); err != nil {
		s.logger.Error("write player state failed")
	}
}

func (s *Server) playerEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "player updates unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	listener := make(chan struct{}, 1)
	s.player.mu.Lock()
	s.player.listeners[listener] = struct{}{}
	s.player.mu.Unlock()
	defer func() {
		s.player.mu.Lock()
		delete(s.player.listeners, listener)
		s.player.mu.Unlock()
	}()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		state := s.player.snapshot()
		data, err := json.Marshal(state)
		if err != nil {
			return
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-listener:
		case <-ticker.C:
		}
	}
}

type playerRequest struct {
	Client   string       `json:"client"`
	Action   string       `json:"action"`
	Track    *playerTrack `json:"track"`
	Position float64      `json:"position"`
	Revision uint64       `json:"revision"`
	Shuffle  bool         `json:"shuffle"`
}

func (s *Server) playerCommand(w http.ResponseWriter, r *http.Request) {
	if s.downloads == nil {
		http.Error(w, "player unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var cmd playerRequest
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil || len(cmd.Client) < 16 || len(cmd.Client) > 128 || math.IsNaN(cmd.Position) || math.IsInf(cmd.Position, 0) || cmd.Position < 0 || cmd.Position > 86400 {
		http.Error(w, "invalid player command", http.StatusBadRequest)
		return
	}
	var playable []store.Track
	if cmd.Action == "next" || cmd.Action == "previous" {
		var err error
		playable, err = s.store.ListPlayable()
		if err != nil {
			http.Error(w, "unable to read playable tracks", http.StatusInternalServerError)
			return
		}
	}
	if cmd.Action == "play" && cmd.Track != nil {
		if cmd.Track.Source != "spotify" && cmd.Track.Source != "vk" {
			http.Error(w, "unknown track", http.StatusBadRequest)
			return
		}
		track, err := s.store.Track(cmd.Track.Source, cmd.Track.Artist, cmd.Track.Title)
		if errors.Is(err, sql.ErrNoRows) || err == nil && track.FilePath == "" {
			http.Error(w, "track not downloaded", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "unable to read track", http.StatusInternalServerError)
			return
		}
		if info, err := os.Stat(track.FilePath); err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			http.Error(w, "track file unavailable", http.StatusBadRequest)
			return
		}
	}
	p := &s.player
	p.mu.Lock()
	defer p.mu.Unlock()
	p.expireLocked()
	state := &p.state
	if state.Pending != "" && cmd.Action != "play" && cmd.Action != "release" && cmd.Action != "report" && cmd.Action != "pause" {
		http.Error(w, "player handoff in progress", http.StatusConflict)
		return
	}
	switch cmd.Action {
	case "play":
		track := cmd.Track
		if track == nil {
			track = state.Track
		}
		if track == nil {
			http.Error(w, "select a track first", http.StatusBadRequest)
			return
		}
		if state.Owner != "" && state.Owner != cmd.Client {
			state.Pending = cmd.Client
			p.pending = track
			state.Revision++
			p.notifyLocked()
			w.WriteHeader(http.StatusAccepted)
			return
		}
		p.selectLocked(*track)
		state.Owner = cmd.Client
		state.Playing = true
		state.Pending = ""
		p.pending = nil
		p.lease = time.Now().Add(playerLease)
		p.reportedAt = time.Now()
	case "release":
		if state.Owner != cmd.Client {
			w.WriteHeader(http.StatusConflict)
			return
		}
		state.Position = cmd.Position
		p.reportedAt = time.Now()
		state.Owner = state.Pending
		if state.Owner != "" && p.pending != nil {
			p.selectLocked(*p.pending)
			state.Playing = true
		} else {
			state.Playing = false
		}
		state.Pending = ""
		p.pending = nil
		p.lease = time.Now().Add(playerLease)
	case "report":
		if state.Owner != cmd.Client || state.Revision != cmd.Revision || state.Pending != "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		state.Position = cmd.Position
		p.reportedAt = time.Now()
		p.lease = time.Now().Add(playerLease)
		w.WriteHeader(http.StatusNoContent)
		return
	case "pause":
		position := p.positionLocked()
		state.Playing = false
		state.Pending = ""
		p.pending = nil
		if state.Owner == cmd.Client {
			position = cmd.Position
		}
		state.Position = position
		p.reportedAt = time.Time{}
	case "seek":
		if state.Track == nil {
			http.Error(w, "select a track first", http.StatusBadRequest)
			return
		}
		state.Position = cmd.Position
		p.reportedAt = time.Now()
	case "shuffle":
		state.Shuffle = cmd.Shuffle
	case "next", "previous":
		if len(playable) == 0 {
			http.Error(w, "no downloaded tracks", http.StatusBadRequest)
			return
		}
		index := -1
		for i, track := range playable {
			if state.Track != nil && *state.Track == (playerTrack{track.Source, track.Artist, track.Title}) {
				index = i
				break
			}
		}
		choice := playerTrack{}
		if cmd.Action == "previous" && len(p.history) > 0 {
			choice = p.history[len(p.history)-1]
			p.history = p.history[:len(p.history)-1]
		} else if cmd.Action == "next" && state.Shuffle && len(playable) > 1 {
			count := len(playable)
			if index >= 0 {
				count--
			}
			i := rand.Intn(count)
			if index >= 0 && i >= index {
				i++
			}
			track := playable[i]
			choice = playerTrack{track.Source, track.Artist, track.Title}
		} else {
			i := (index + 1) % len(playable)
			if cmd.Action == "previous" {
				i = (index - 1 + len(playable)) % len(playable)
			}
			track := playable[i]
			choice = playerTrack{track.Source, track.Artist, track.Title}
		}
		if cmd.Action == "previous" {
			state.Track = &choice
		} else {
			p.selectLocked(choice)
		}
		state.Position = 0
		state.Playing = state.Owner != ""
		p.reportedAt = time.Now()
	default:
		http.Error(w, "unknown player command", http.StatusBadRequest)
		return
	}
	state.Revision++
	p.notifyLocked()
	w.WriteHeader(http.StatusNoContent)
}
