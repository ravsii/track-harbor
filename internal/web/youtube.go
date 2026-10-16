package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ravsii/trackharbor/download"
	"github.com/ravsii/trackharbor/internal/store"
)

func (s *Server) addYouTubeVideo(w http.ResponseWriter, r *http.Request) {
	if s.downloads == nil {
		http.Error(w, "downloads unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid video form", http.StatusBadRequest)
		return
	}
	id, err := download.YouTubeID(r.PostForm.Get("video"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	userID := currentUser(r).ID
	track, err := s.store.UserYouTubeTrack(userID, id)
	added := false
	if errors.Is(err, sql.ErrNoRows) {
		settings, loadErr := s.store.DownloadSettings()
		if loadErr != nil {
			http.Error(w, "unable to read download proxies", http.StatusInternalServerError)
			return
		}
		metadata, lookupErr := download.YouTubeTrack(r.Context(), id, &download.Routes{Personal: settings.PersonalProxy, Public: settings.PublicProxy})
		if lookupErr != nil {
			// Never expose yt-dlp output or upstream URLs/credentials.
			http.Error(w, "could not load this video; check its availability, yt-dlp and proxy settings. Live or upcoming streams cannot be added", http.StatusBadGateway)
			return
		}
		track, added, err = s.store.AddYouTubeTrack(userID, metadata)
	}
	if err != nil {
		http.Error(w, "unable to add video to your collection", http.StatusInternalServerError)
		return
	}
	s.downloads.enqueue([]store.Track{track})
	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(struct {
			Added bool `json:"added"`
		}{added}); err != nil {
			s.logger.Error("write YouTube add result failed")
		}
	} else {
		http.Redirect(w, r, "/?source=youtube", http.StatusSeeOther)
	}
}
