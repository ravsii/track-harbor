package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"trackharbor/internal/repository"
)

// Previous downloads predate query-to-file persistence. Recover exact mappings
// from successful terminal log events rather than guessing from filenames.
func restoreDownloadQueries(store *repository.DownloadedTracksRepository) error {
	f, err := os.Open("logs/download.jsonl")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	seen := make(map[string]bool)
	for scanner.Scan() {
		var event struct {
			Message  string `json:"msg"`
			Source   string `json:"source"`
			Query    string `json:"query"`
			TrackID  string `json:"track_id"`
			FilePath string `json:"file_path"`
		}
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		if event.Message != "downloaded" && event.Message != "skipped" {
			continue
		}
		if event.Source == "" || event.Query == "" || event.TrackID == "" || !filepath.IsAbs(event.FilePath) {
			continue
		}
		key := event.Source + "\x00" + event.Query + "\x00" + event.FilePath
		if seen[key] {
			continue
		}
		seen[key] = true
		if err := store.SaveQuery(event.Source, event.Query, event.TrackID, event.FilePath); err != nil {
			return fmt.Errorf("restore search history: %w", err)
		}
	}
	return scanner.Err()
}
