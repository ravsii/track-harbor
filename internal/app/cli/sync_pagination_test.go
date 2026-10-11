package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"trackharbor/internal/source/spotify"
)

func playlistPageResponse(t *testing.T, offset, count, total int) *http.Response {
	t.Helper()
	var response spotify.PlaylistResponse
	playlist := &response.Data.PlaylistV2
	playlist.URI = "spotify:playlist:37i9dQZF1F5p3rmiWPIYgZ"
	playlist.Name = "Large playlist"
	playlist.Content.TotalCount = total
	playlist.Content.PagingInfo.Offset = offset
	playlist.Content.PagingInfo.Limit = 50
	playlist.Content.Items = make([]spotify.Item, count)
	for i := range playlist.Content.Items {
		item := &playlist.Content.Items[i]
		item.UID = strconv.Itoa(offset + i + 1)
		item.ItemV3.Data.IdentityTrait.Name = fmt.Sprintf("Song %d", offset+i+1)
		item.ItemV3.Data.IdentityTrait.Contributors.Items = []spotify.Contributor{{Name: "Artist"}}
	}
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

func pageOffset(t *testing.T, req *http.Request) int {
	t.Helper()
	var payload struct {
		Variables struct {
			Offset int
			Limit  int
		}
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Variables.Limit != 50 {
		t.Errorf("expected page size 50, got %d", payload.Variables.Limit)
	}
	return payload.Variables.Offset
}

func TestSyncSavesAllPlaylistPages(t *testing.T) {
	var offsets []int
	output, err := runSyncTest(t, "authorization: Bearer secret-auth\nclient-token: secret-client\n\n", syncTransport(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("authorization") != "Bearer secret-auth" || req.Header.Get("client-token") != "secret-client" {
			t.Error("later page lost the verified headers")
		}
		offset := pageOffset(t, req)
		offsets = append(offsets, offset)
		count := min(50, 127-offset)
		return playlistPageResponse(t, offset, count, 127), nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(offsets, []int{0, 50, 100}) {
		t.Errorf("expected three consecutive pages, got offsets %v", offsets)
	}
	database, err := sql.Open("sqlite3", "meshtune.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var count, last int
	if err := database.QueryRow("SELECT COUNT(*) FROM songs").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow("SELECT COUNT(*) FROM songs WHERE title = 'Song 127'").Scan(&last); err != nil {
		t.Fatal(err)
	}
	if count != 127 || last != 1 {
		t.Errorf("not all songs saved: count=%d last=%d", count, last)
	}
	for _, want := range []string{"Page 1", "Page 2", "Page 3", "50 of 127", "100 of 127", "127 of 127", "Saved 127 tracks"} {
		if !strings.Contains(output, want) {
			t.Errorf("missing pagination progress %q", want)
		}
	}
}

func TestSyncKeepsOldSongsWhenPaginationFails(t *testing.T) {
	for _, failure := range []string{"http", "empty page", "wrong offset", "changed count", "empty first page", "wrong first offset", "overrun"} {
		t.Run(failure, func(t *testing.T) {
			output, err := runSyncTest(t, "authorization: Bearer secret-auth\nclient-token: secret-client\n\n", syncTransport(func(req *http.Request) (*http.Response, error) {
				offset := pageOffset(t, req)
				if offset == 0 {
					if failure == "empty first page" {
						return playlistPageResponse(t, 0, 0, 127), nil
					}
					if failure == "wrong first offset" {
						return playlistPageResponse(t, 50, 50, 127), nil
					}
					return playlistPageResponse(t, 0, 50, 127), nil
				}
				switch failure {
				case "http":
					return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("offline"))}, nil
				case "empty page":
					return playlistPageResponse(t, offset, 0, 127), nil
				case "wrong offset":
					return playlistPageResponse(t, 0, 50, 127), nil
				case "overrun":
					return playlistPageResponse(t, offset, 78, 127), nil
				case "wrong first offset":
					return playlistPageResponse(t, offset, min(50, 127-offset), 127), nil
				default:
					return playlistPageResponse(t, offset, 50, 128), nil
				}
			}), func(database *sql.DB) {
				if _, err := database.Exec(`INSERT INTO songs (artists, title, created_at) VALUES ('["Old artist"]', 'Old song', CURRENT_TIMESTAMP)`); err != nil {
					t.Fatal(err)
				}
			})
			if err == nil || !strings.Contains(err.Error(), "[2/4]") {
				t.Errorf("expected pagination error, got %v", err)
			}
			if strings.Contains(output, "[3/4]") || strings.Contains(output, "[4/4]") {
				t.Error("incomplete playlist was sent to persistence")
			}
			database, err := sql.Open("sqlite3", "meshtune.db")
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			var count int
			if err := database.QueryRow("SELECT COUNT(*) FROM songs WHERE title = 'Old song'").Scan(&count); err != nil || count != 1 {
				t.Errorf("old library was lost: count=%d err=%v", count, err)
			}
		})
	}
}

func TestSyncPaginatesWithStoredOrRenewedHeaders(t *testing.T) {
	for _, renew := range []bool{false, true} {
		t.Run(fmt.Sprintf("renew=%t", renew), func(t *testing.T) {
			input := ""
			if renew {
				input = "authorization: Bearer secret-auth\nclient-token: secret-client\n\n"
			}
			var offsets []int
			output, err := runSyncTest(t, input, syncTransport(func(req *http.Request) (*http.Response, error) {
				offset := pageOffset(t, req)
				offsets = append(offsets, offset)
				if renew && len(offsets) == 1 {
					return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("expired"))}, nil
				}
				auth, client := "Bearer old-auth", "old-client"
				if renew {
					auth, client = "Bearer secret-auth", "secret-client"
				}
				if req.Header.Get("authorization") != auth || req.Header.Get("client-token") != client {
					t.Error("page did not use the verified headers")
				}
				return playlistPageResponse(t, offset, min(50, 127-offset), 127), nil
			}), func(database *sql.DB) { seedStoredHeaders(t, database) })
			if err != nil || !strings.Contains(output, "Saved 127 tracks") {
				t.Fatalf("pagination failed: err=%v output=%s", err, output)
			}
			want := []int{0, 50, 100}
			if renew {
				want = []int{0, 0, 50, 100}
			}
			if !reflect.DeepEqual(offsets, want) {
				t.Errorf("unexpected page requests: %v", offsets)
			}
			if !renew && strings.Contains(output, "Paste Spotify request headers") {
				t.Error("working cached headers still prompted for input")
			}
		})
	}
}
