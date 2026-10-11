package web

import (
	"database/sql"
	"errors"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// playAudio serves only tracks registered in the server's own download history.
// ServeContent implements HTTP byte ranges for seeking without exposing paths.
func (s *Server) playAudio(w http.ResponseWriter, r *http.Request) {
	if s.downloads == nil {
		http.NotFound(w, r)
		return
	}
	source := r.URL.Query().Get("source")
	if source != "spotify" && source != "vk" {
		http.NotFound(w, r)
		return
	}
	t, err := s.store.Track(source, r.URL.Query().Get("artist"), r.URL.Query().Get("title"))
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "unable to read track", http.StatusInternalServerError)
		return
	}
	if t.FilePath == "" {
		http.NotFound(w, r)
		return
	}
	root, err := filepath.EvalSymlinks(s.downloads.config.DownloadDir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	path, err := filepath.EvalSymlinks(t.FilePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.Ext(path) != ".mp3" {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	if r.URL.Query().Has("download") {
		name := strings.ReplaceAll(t.Title, "\\", "_")
		name = strings.ReplaceAll(name, "/", "_")
		if name == "" {
			name = "track"
		}
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name + ".mp3"}))
	} else {
		w.Header().Set("Content-Disposition", "inline")
	}
	http.ServeContent(w, r, "", info.ModTime(), f)
}
