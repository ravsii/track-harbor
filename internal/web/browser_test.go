package web

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"trackharbor/internal/store"
)

func TestBrowser(t *testing.T) {
	if os.Getenv("TRACKHARBOR_BROWSER_TEST") != "1" {
		t.Skip("run npm run test:web to run real Chromium integration tests")
	}
	db, h, dir := webFixture(t)
	path := filepath.Join(dir, "fixture.mp3")
	cmd := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "anullsrc=r=44100:cl=mono", "-t", "60", "-codec:a", "libmp3lame", "-q:a", "9", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate audio fixture: %v %s", err, output)
	}
	if _, err := db.Replace(1, "spotify", []store.Track{{SourceID: "one", Artist: "Fixture", Title: "First track"}, {SourceID: "two", Artist: "Fixture", Title: "Second track"}}); err != nil {
		t.Fatal(err)
	}
	tracks, err := db.ListUser(1)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	for _, track := range tracks {
		if err := db.SaveDownload(track, path, hex.EncodeToString(hash[:])); err != nil {
			t.Fatal(err)
		}
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/fixture/restore" {
			for _, track := range tracks {
				if err := db.SaveDownload(track, path, hex.EncodeToString(hash[:])); err != nil {
					t.Error(err)
					http.Error(w, "fixture failed", 500)
					return
				}
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/fixture/pending" {
			bulk := append([]store.Track(nil), tracks...)
			for i := 0; i < 1000; i++ {
				bulk = append(bulk, store.Track{SourceID: fmt.Sprintf("pending-%d", i), Title: "Pending track"})
			}
			if _, err := db.Replace(1, "spotify", bulk); err != nil {
				t.Error(err)
				http.Error(w, "fixture failed", 500)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/fixture/invalidate" {
			for _, track := range tracks {
				current, err := db.Track(track.ID)
				if err != nil {
					t.Error(err)
					http.Error(w, "fixture failed", 500)
					return
				}
				if err = db.InvalidateDownload(current); err != nil {
					t.Error(err)
					http.Error(w, "fixture failed", 500)
					return
				}
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	}))
	defer ts.Close()
	cmd = exec.Command("node", "tests/web/browser.cjs", ts.URL)
	cmd.Dir = filepath.Join("..", "..")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("browser tests: %v\n%s", err, output)
	} else {
		t.Log(string(output))
	}
}
