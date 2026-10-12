package web

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
	"trackharbor/internal/config"
	"trackharbor/internal/store"
)

func TestQueuedVKTrackUsesLatestSignedURL(t *testing.T) {
	db, _, dir := webFixture(t)
	path := filepath.Join(dir, "input.mp3")
	cmd := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "anullsrc=r=44100:cl=mono", "-t", "1", "-codec:a", "libmp3lame", "-q:a", "9", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("audio fixture: %v %s", err, output)
	}
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fresh.mp3" {
			http.Error(w, "expired", 410)
			return
		}
		http.ServeFile(w, r, path)
	}))
	ts.TLS = &tls.Config{MaxVersion: tls.VersionTLS12}
	ts.StartTLS()
	defer ts.Close()
	certPath := filepath.Join(dir, "fixture-ca.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ts.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", certPath)
	cmd = exec.Command("ffmpeg", "-v", "error", "-i", ts.URL+"/fresh.mp3", "-f", "null", "-")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("HTTPS audio fixture: %v %s", err, output)
	}
	track := store.Track{SourceID: "-12_34", Artist: "Artist", Title: "Song", AudioURL: ts.URL + "/expired.mp3"}
	if _, err := db.Replace(1, "vk", []store.Track{track}); err != nil {
		t.Fatal(err)
	}
	stale, err := db.ListUser(1)
	if err != nil {
		t.Fatal(err)
	}
	track.AudioURL = ts.URL + "/fresh.mp3"
	if _, err := db.Replace(1, "vk", []store.Track{track}); err != nil {
		t.Fatal(err)
	}
	d := newDownloadJob(db, config.Config{DownloadDir: dir}, zap.NewNop())
	d.runBatch(context.Background(), stale)
	if state := d.tracks[trackKey("vk", "-12_34")]; state.Stage != "done" {
		t.Fatalf("fresh URL ignored: %+v", state)
	}
}

func TestQueueSkipsStaleSnapshotAfterAnotherUsersDownload(t *testing.T) {
	db, _, dir := webFixture(t)
	if _, err := db.Replace(1, "spotify", []store.Track{{SourceID: "id", Title: "Song"}}); err != nil {
		t.Fatal(err)
	}
	tracks, err := db.ListUser(1)
	if err != nil {
		t.Fatal(err)
	}
	stale := tracks[0]
	data := []byte("existing shared audio")
	path := filepath.Join(dir, "existing.mp3")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	if err := db.SaveDownload(stale, path, hex.EncodeToString(hash[:])); err != nil {
		t.Fatal(err)
	}
	d := newDownloadJob(db, config.Config{DownloadDir: dir}, zap.NewNop())
	d.runBatch(context.Background(), []store.Track{stale})
	if state := d.tracks[trackKey("spotify", "id")]; state.Stage != "skipped" {
		t.Fatalf("re-downloaded a shared file: %+v", state)
	}
	current, err := db.Track(stale.ID)
	if err != nil || current.FilePath != path {
		t.Fatal("shared download replaced")
	}
}

func TestQueueDeduplicatesBySourceIDNotDisplayName(t *testing.T) {
	d := newDownloadJob(nil, config.Config{}, zap.NewNop())
	d.state.Running = true
	d.enqueue([]store.Track{{ID: 1, Source: "spotify", SourceID: "one", Artist: "A", Title: "Song"}, {ID: 1, Source: "spotify", SourceID: "one", Artist: "Changed", Title: "New title"}, {ID: 2, Source: "spotify", SourceID: "two", Artist: "A", Title: "Song"}})
	if len(d.queue) != 2 {
		t.Fatalf("want 2 unique identities, queued %d", len(d.queue))
	}
}

func TestPrivatePagesAndSharedAudio(t *testing.T) {
	db, h, dir := webFixture(t)
	a, _ := db.CreateUser("alice", "password")
	b, _ := db.CreateUser("bob", "password")
	if _, err := db.Replace(a.ID, "spotify", []store.Track{{SourceID: "one", Artist: "A", Title: "Alice track"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Replace(b.ID, "spotify", []store.Track{{SourceID: "two", Artist: "B", Title: "Bob track"}}); err != nil {
		t.Fatal(err)
	}
	items, _ := db.ListUser(b.ID)
	path := filepath.Join(dir, "song.mp3")
	if err := os.WriteFile(path, []byte("0123456789"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDownload(items[0], path, "hash"); err != nil {
		t.Fatal(err)
	}
	c := login(t, h, "alice", "password")
	w := request(h, "GET", "/", nil, c)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Alice track") || strings.Contains(w.Body.String(), "Bob track") {
		t.Fatal("personal page leaked another playlist")
	}
	w = request(h, "GET", "/audio/tracks", nil, c)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Bob track") || strings.Contains(w.Body.String(), "file_path") {
		t.Fatal("shared playable catalog unavailable")
	}
	r := httptest.NewRequest("GET", fmt.Sprintf("http://localhost/audio?id=%d", items[0].ID), nil)
	r.AddCookie(c)
	r.Header.Set("Range", "bytes=2-4")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 206 || w.Body.String() != "234" {
		t.Fatalf("shared ranged audio: %d %q", w.Code, w.Body.String())
	}
	outside := filepath.Join(t.TempDir(), "secret.mp3")
	os.WriteFile(outside, []byte("secret"), 0600)
	if err := db.SaveDownload(items[0], outside, "hash"); err != nil {
		t.Fatal(err)
	}
	w = request(h, "GET", fmt.Sprintf("/audio?id=%d", items[0].ID), nil, c)
	if w.Code != 404 {
		t.Fatal("outside file served")
	}
}

func TestDownloadStatusSeparatesPersonalTracksAndQueue(t *testing.T) {
	db, h, _ := webFixture(t)
	a, _ := db.CreateUser("alice", "password")
	b, _ := db.CreateUser("bob", "password")
	db.Replace(a.ID, "spotify", []store.Track{{SourceID: "a", Title: "Alice"}})
	db.Replace(b.ID, "spotify", []store.Track{{SourceID: "b", Title: "Bob"}})
	c := login(t, h, "alice", "password")
	w := request(h, "GET", "/download/status", nil, c)
	var snapshot struct {
		Tracks []struct{ Title string }
		Mine   []struct{ Title string } `json:"my_tracks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Tracks) != 2 || len(snapshot.Mine) != 1 || snapshot.Mine[0].Title != "Alice" {
		t.Fatalf("queue/mine isolation: %s", w.Body.String())
	}
}

func TestUserManagementConfirmationAndBootstrapRemoval(t *testing.T) {
	db, h, _ := webFixture(t)
	c := login(t, h, "admin", "admin")
	w := request(h, "POST", "/settings/users", url.Values{"username": {"alice"}, "password": {"password"}}, c)
	if w.Code != 303 {
		t.Fatal(w.Code)
	}
	u, _ := db.Authenticate("alice", "password")
	alice := login(t, h, "alice", "password")
	w = request(h, "POST", fmt.Sprintf("/settings/users/%d/delete", u.ID), nil, c)
	if w.Code != 400 {
		t.Fatal("deletion without confirmation")
	}
	w = request(h, "POST", "/settings/users/1/delete", url.Values{"confirm": {"yes"}}, alice)
	if w.Code != 303 {
		t.Fatalf("delete admin %d", w.Code)
	}
	w = request(h, "GET", "/auth/status", nil, alice)
	var account struct {
		Bootstrap bool `json:"bootstrap"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &account); err != nil {
		t.Fatal(err)
	}
	if account.Bootstrap {
		t.Fatal("warning remains after deletion")
	}
	w = request(h, "POST", fmt.Sprintf("/settings/users/%d/delete", u.ID), url.Values{"confirm": {"yes"}}, alice)
	if w.Code != 409 {
		t.Fatal("last user removed")
	}
}
