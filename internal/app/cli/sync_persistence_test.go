package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"meshvault/internal/source/spotify"
)

func syncResponse(t *testing.T, response spotify.PlaylistResponse) *http.Response {
	t.Helper()
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	compressed := encoder.EncodeAll(data, nil)
	encoder.Close()
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(compressed))}
}

func TestSyncPersistsUniqueSongsWhenPlaylistContainsRepeats(t *testing.T) {
	var response spotify.PlaylistResponse
	if err := json.Unmarshal([]byte(syncFixture), &response); err != nil {
		t.Fatal(err)
	}
	content := &response.Data.PlaylistV2.Content
	content.Items = append(content.Items, content.Items[0])
	content.TotalCount = 3
	output, err := runSyncTest(t, "authorization: Bearer test\nclient-token: test\n\n", syncTransport(func(*http.Request) (*http.Response, error) {
		return syncResponse(t, response), nil
	}), func(database *sql.DB) {
		if _, err := database.Exec(`INSERT INTO songs (artists, title, created_at) VALUES ('["Old artist"]', 'Old song', CURRENT_TIMESTAMP)`); err != nil {
			t.Fatal(err)
		}
	})
	if err != nil {
		t.Fatalf("repeated playlist track prevented persistence: %v", err)
	}
	database, err := sql.Open("sqlite3", "meshtune.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var count, old int
	if err := database.QueryRow("SELECT COUNT(*) FROM songs").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow("SELECT COUNT(*) FROM songs WHERE title = 'Old song'").Scan(&old); err != nil {
		t.Fatal(err)
	}
	if count != 2 || old != 0 {
		t.Fatalf("replacement saved the wrong list: count=%d old=%d", count, old)
	}
	if !strings.Contains(output, "Saved 2 tracks") || !strings.Contains(output, "Duplicates: 1") {
		t.Fatalf("summary did not report actual persisted count and repeats: %s", output)
	}
}

func TestSyncReadsLegacyMetadataAndSkipsUnavailableSongs(t *testing.T) {
	var response spotify.PlaylistResponse
	content := &response.Data.PlaylistV2.Content
	content.TotalCount = 2
	content.Items = make([]spotify.Item, 2)
	content.Items[0].ItemV2.Data.Name = "Legacy title"
	content.Items[0].ItemV2.Data.Artists.Items = []spotify.ArtistRef{{Profile: spotify.Profile{Name: "Legacy artist"}}}
	output, err := runSyncTest(t, "authorization: Bearer test\nclient-token: test\n\n", syncTransport(func(*http.Request) (*http.Response, error) {
		return syncResponse(t, response), nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite3", "meshtune.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var title, artists string
	if err := database.QueryRow("SELECT title, artists FROM songs").Scan(&title, &artists); err != nil {
		t.Fatal(err)
	}
	if title != "Legacy title" || strings.TrimSpace(artists) != `["Legacy artist"]` {
		t.Fatalf("legacy metadata lost: title=%q artists=%q", title, artists)
	}
	if !strings.Contains(output, "Saved 1 tracks") || !strings.Contains(output, "Unavailable tracks skipped: 1") {
		t.Fatalf("unavailable track was not reported: %s", output)
	}
}
