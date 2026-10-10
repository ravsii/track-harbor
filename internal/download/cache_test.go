package download

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"meshvault/internal/repository"
)

// Only the external music provider is replaced; SQLite, hashes and files are real.
type cacheProvider struct {
	calls    atomic.Int64
	body     string
	length   int64
	distinct bool
}

func (p *cacheProvider) Name() string { return "test" }
func (p *cacheProvider) Search(query string) (Track, error) {
	id := "track"
	if p.distinct {
		id = query
	}
	return Track{ID: id, Artist: "Artist", Title: "Title"}, nil
}
func (p *cacheProvider) Download(Track) (DownloadResponse, error) {
	p.calls.Add(1)
	return DownloadResponse{Body: io.NopCloser(strings.NewReader(p.body)), ContentLength: p.length}, nil
}

func cacheDatabase(t *testing.T) (*sqlx.DB, *repository.DownloadedTracksRepository) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store, err := repository.NewDownloadedTracksRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	return db, store
}

func runCached(t *testing.T, p Provider, dir string, store DownloadStore, queries ...string) *Display {
	t.Helper()
	display := NewDisplay()
	New(p, 10, dir).WithDownloadStore(store).WithDisplay(display).Run(queries)
	if len(display.active) != 0 {
		t.Fatal("completed jobs remained active")
	}
	return display
}

func TestDownloadPersistsHashAndSkipsVerifiedFileOnNextRun(t *testing.T) {
	db, store := cacheDatabase(t)
	dir := t.TempDir()
	p := &cacheProvider{body: "abcdef", length: 6}
	first := runCached(t, p, dir, store, "query")
	if len(first.done) != 1 || first.done[0].Kind != StatusDone {
		t.Fatalf("first download failed: %+v", first.done)
	}
	var hash, source, trackID, filePath string
	if err := db.QueryRow("SELECT sha256, source, track_id, file_path FROM downloaded_tracks").Scan(&hash, &source, &trackID, &filePath); err != nil {
		t.Fatal(err)
	}
	if hash != "bef57ec7f53a6d40beb640a780a639c83bc29ac8a9816f1fc6c5c6dcd93c4721" || source != "test" || trackID != "track" || filePath != filepath.Join(dir, "track.mp3") {
		t.Fatalf("incorrect downloaded-file record: hash=%s source=%s id=%s path=%s", hash, source, trackID, filePath)
	}
	// Reopen SQLite itself so an in-memory flag cannot satisfy this check.
	var seq int
	var dbName, dbPath string
	if err := db.QueryRow("PRAGMA database_list").Scan(&seq, &dbName, &dbPath); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopenedDB, err := sqlx.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopenedDB.Close()
	reopened, err := repository.NewDownloadedTracksRepository(reopenedDB)
	if err != nil {
		t.Fatal(err)
	}
	second := runCached(t, p, dir, reopened, "query")
	if p.calls.Load() != 1 || len(second.done) != 1 || second.done[0].Kind != StatusSkipped {
		t.Fatalf("verified file was downloaded again: calls=%d events=%+v", p.calls.Load(), second.done)
	}
}

func TestDownloadRedownloadsMissingChangedOrUntrackedFiles(t *testing.T) {
	for _, state := range []string{"missing", "same-size corruption", "untracked", "different directory"} {
		t.Run(state, func(t *testing.T) {
			db, store := cacheDatabase(t)
			dir := t.TempDir()
			p := &cacheProvider{body: "abcdef", length: 6}
			runCached(t, p, dir, store, "query")
			file := filepath.Join(dir, "track.mp3")
			switch state {
			case "missing":
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
			case "same-size corruption":
				if err := os.WriteFile(file, []byte("abcdeg"), 0600); err != nil {
					t.Fatal(err)
				}
			case "untracked":
				if _, err := db.Exec("DELETE FROM downloaded_tracks"); err != nil {
					t.Fatal(err)
				}
			case "different directory":
				dir = t.TempDir()
				file = filepath.Join(dir, "track.mp3")
			}
			p.body = "ghijkl"
			display := runCached(t, p, dir, store, "query")
			body, err := os.ReadFile(file)
			if err != nil || string(body) != "ghijkl" || p.calls.Load() != 2 || display.done[0].Kind != StatusDone {
				t.Fatalf("file was not refreshed: body=%q err=%v calls=%d events=%+v", body, err, p.calls.Load(), display.done)
			}
			var count int
			if err := db.Get(&count, "SELECT COUNT(*) FROM downloaded_tracks"); err != nil {
				t.Fatal(err)
			}
			want := 1
			if state == "different directory" {
				want = 2
			}
			if count != want {
				t.Fatalf("unexpected number of records: %d", count)
			}
			next := runCached(t, p, dir, store, "query")
			if p.calls.Load() != 2 || next.done[0].Kind != StatusSkipped {
				t.Fatal("new file hash was not persisted for subsequent runs")
			}
		})
	}
}

func TestIncompleteDownloadDoesNotReplaceFileOrSaveHash(t *testing.T) {
	db, store := cacheDatabase(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "track.mp3")
	if err := os.WriteFile(file, []byte("old file"), 0600); err != nil {
		t.Fatal(err)
	}
	p := &cacheProvider{body: "partial", length: 100}
	display := runCached(t, p, dir, store, "query")
	body, err := os.ReadFile(file)
	if err != nil || string(body) != "old file" || display.done[0].Kind != StatusFailed {
		t.Fatalf("incomplete transfer replaced the file: %q %v events=%+v", body, err, display.done)
	}
	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM downloaded_tracks"); err != nil || count != 0 {
		t.Fatalf("partial file acquired a hash: count=%d err=%v", count, err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("temporary files were not removed: %v %v", files, err)
	}
}

func TestParallelDownloadsPersistHashesWithoutDuplicateTransfers(t *testing.T) {
	for _, distinct := range []bool{false, true} {
		t.Run(fmt.Sprintf("distinct=%t", distinct), func(t *testing.T) {
			db, store := cacheDatabase(t)
			dir := t.TempDir()
			p := &cacheProvider{body: "abcdef", length: 6, distinct: distinct}
			var queries []string
			for i := 0; i < 10; i++ {
				queries = append(queries, fmt.Sprintf("track%d", i))
			}
			display := runCached(t, p, dir, store, queries...)
			var count int
			if err := db.Get(&count, "SELECT COUNT(*) FROM downloaded_tracks"); err != nil {
				t.Fatal(err)
			}
			want := 1
			if distinct {
				want = 10
			}
			if p.calls.Load() != int64(want) || count != want || len(display.done) != 10 {
				t.Fatalf("parallel cache failed: calls=%d rows=%d events=%+v", p.calls.Load(), count, display.done)
			}
			for _, event := range display.done {
				if event.Kind == StatusFailed {
					t.Fatalf("parallel job failed: %v", event.Err)
				}
			}
		})
	}
}

func TestCacheWriteFailureIsNotReportedAsSuccess(t *testing.T) {
	db, store := cacheDatabase(t)
	if _, err := db.Exec(`CREATE TRIGGER reject_hash BEFORE INSERT ON downloaded_tracks BEGIN SELECT RAISE(ABORT, 'write failed'); END`); err != nil {
		t.Fatal(err)
	}
	display := runCached(t, &cacheProvider{body: "abcdef", length: 6}, t.TempDir(), store, "query")
	if display.done[0].Kind != StatusFailed {
		t.Fatalf("failed hash save reported success: %+v", display.done)
	}
}

func TestCacheReadFailureDoesNotOverwriteFile(t *testing.T) {
	db, store := cacheDatabase(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "track.mp3")
	if err := os.WriteFile(file, []byte("old file"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DROP TABLE downloaded_tracks"); err != nil {
		t.Fatal(err)
	}
	p := &cacheProvider{body: "abcdef", length: 6}
	display := runCached(t, p, dir, store, "query")
	body, err := os.ReadFile(file)
	if display.done[0].Kind != StatusFailed || p.calls.Load() != 0 || err != nil || string(body) != "old file" {
		t.Fatalf("cache failure replaced a file: calls=%d body=%q err=%v events=%+v", p.calls.Load(), body, err, display.done)
	}
}

func TestSyncingSongsKeepsDownloadHistory(t *testing.T) {
	db, store := cacheDatabase(t)
	if _, err := db.Exec(`CREATE TABLE songs (id INTEGER PRIMARY KEY, artists TEXT NOT NULL, title TEXT NOT NULL, created_at DATETIME NOT NULL, UNIQUE (artists, title))`); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	p := &cacheProvider{body: "abcdef", length: 6}
	runCached(t, p, dir, store, "query")
	if _, err := repository.NewSongRepository(db).ReplaceAll(nil); err != nil {
		t.Fatal(err)
	}
	display := runCached(t, p, dir, store, "query")
	if p.calls.Load() != 1 || display.done[0].Kind != StatusSkipped {
		t.Fatal("sync erased download history")
	}
}

func TestFileHashMatchesEntireContents(t *testing.T) {
	// The byte fixture intentionally spans multiple io.Copy buffers.
	db, store := cacheDatabase(t)
	body := strings.Repeat("abcdef", 20000)
	dir := t.TempDir()
	display := runCached(t, &cacheProvider{body: body, length: int64(len(body))}, dir, store, "query")
	if display.done[0].Kind != StatusDone {
		t.Fatal(display.done[0].Err)
	}
	var hash string
	if err := db.Get(&hash, "SELECT sha256 FROM downloaded_tracks"); err != nil {
		t.Fatal(err)
	}
	if hash != fmt.Sprintf("%x", sha256.Sum256([]byte(body))) {
		t.Fatal("hash covers only part of file")
	}
}
