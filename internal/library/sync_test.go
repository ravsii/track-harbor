package library

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ravsii/trackharbor/internal/store"
)

// Exercise the real provider clients against a local HTTPS fixture; no tokens
// or requests leave the test process. Do not run these tests in parallel:
// Spotify uses DefaultClient and VK clones DefaultTransport.
func syncFixture(t *testing.T, handler http.HandlerFunc) *store.Store {
	t.Helper()
	ts := httptest.NewTLSServer(handler)
	transport := ts.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	transport.TLSClientConfig.ServerName = "example.com"
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, ts.Listener.Addr().String())
	}
	client, oldTransport := http.DefaultClient, http.DefaultTransport
	http.DefaultTransport = transport
	http.DefaultClient = &http.Client{Transport: transport}
	t.Cleanup(func() {
		http.DefaultClient = client
		http.DefaultTransport = oldTransport
		transport.CloseIdleConnections()
		ts.Close()
	})
	db, err := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSpotifySyncUsesOnlyCurrentUsersCredentialsAndKeepsIDs(t *testing.T) {
	db := syncFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer alice" || r.Header.Get("Client-Token") != "client-alice" {
			t.Error("another user's credentials used")
			http.Error(w, "bad credentials", 401)
			return
		}
		var payload struct {
			Operation string `json:"operationName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if payload.Operation == "libraryV3" {
			_, _ = w.Write([]byte(`{"data":{"me":{"libraryV3":{"totalCount":1,"pagingInfo":{"offset":0},"items":[{"item":{"data":{"uri":"spotify:collection:tracks"}}}]}}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"playlistV2":{"uri":"spotify:playlist:fixture","format":"liked-songs","content":{"totalCount":1,"items":[{"uid":"not-the-track-id","addedAt":{"isoString":"2024-01-02T03:04:05Z"},"itemV2":{"data":{"uri":"spotify:track:abcdefghijklmnopqrstuv","name":"Song","artists":{"items":[{"profile":{"name":"Artist"}}]},"albumOfTrack":{"coverArt":{"sources":[{"url":"https://images.example/cover.jpg","width":300}]}}}}}]}}}}`))
	})
	alice, err := db.CreateUser("alice", "password")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.SaveCredentials(alice.ID, "spotify", "Bearer alice", "client-alice"); err != nil {
		t.Fatal(err)
	}
	if err = db.SaveCredentials(1, "spotify", "Bearer other", "client-other"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Replace(1, "spotify", []store.Track{{SourceID: "other", Title: "Other"}}); err != nil {
		t.Fatal(err)
	}
	if count, err := SyncSpotify(context.Background(), db, alice.ID); err != nil || count != 1 {
		t.Fatalf("sync: %d %v", count, err)
	}
	tracks, err := db.ListUser(alice.ID)
	if err != nil || len(tracks) != 1 || tracks[0].SourceID != "abcdefghijklmnopqrstuv" || tracks[0].Artist != "Artist" || tracks[0].AddedAt != 1704164645 || tracks[0].CoverURL != "https://images.example/cover.jpg" {
		t.Fatalf("synced metadata: %+v %v", tracks, err)
	}
	tracks, err = db.ListUser(1)
	if err != nil || len(tracks) != 1 || tracks[0].Title != "Other" {
		t.Fatal("sync changed another playlist")
	}
}

func TestVKSyncUsesOwnerTokenAndPreservesOtherUsersPlaylist(t *testing.T) {
	db := syncFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		if r.PostForm.Get("access_token") != "alice-token" {
			t.Error("wrong VK token")
			http.Error(w, "bad token", 401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/method/users.get" {
			_, _ = w.Write([]byte(`{"response":[{"id":123}]}`))
			return
		}
		if r.PostForm.Get("owner_id") != "123" {
			t.Error("synced wrong owner")
		}
		_, _ = w.Write([]byte(`{"response":{"count":1,"items":[{"id":456,"owner_id":-123,"artist":"Artist","title":"Song","url":"https://audio.example/signed-private-url","date":100,"album":{"thumb":{"photo_600":"https://images.example/vk.jpg"}}}]}}`))
	})
	alice, err := db.CreateUser("alice", "password")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.SaveCredentials(alice.ID, "vk", "alice-token", ""); err != nil {
		t.Fatal(err)
	}
	if err = db.SaveCredentials(1, "vk", "other-token", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Replace(1, "vk", []store.Track{{SourceID: "1_2", Title: "Other"}}); err != nil {
		t.Fatal(err)
	}
	if n, err := SyncVK(context.Background(), db, alice.ID); err != nil || n != 1 {
		t.Fatalf("sync: %d %v", n, err)
	}
	tracks, err := db.ListUser(alice.ID)
	if err != nil || len(tracks) != 1 || tracks[0].SourceID != "-123_456" || tracks[0].AudioURL != "https://audio.example/signed-private-url" {
		t.Fatalf("VK metadata: %+v %v", tracks, err)
	}
	tracks, err = db.ListUser(1)
	if err != nil || len(tracks) != 1 || tracks[0].Title != "Other" {
		t.Fatal("sync changed other user's VK tracks")
	}
}

type requestFunc func(*http.Request) (*http.Response, error)

func (f requestFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSpotifySyncDeadlineCancelsDiscoveryRequest(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.SaveCredentials(1, "spotify", "Bearer alice", "client-alice"); err != nil {
		t.Fatal(err)
	}
	client := http.DefaultClient
	defer func() { http.DefaultClient = client }()
	http.DefaultClient = &http.Client{Transport: requestFunc(func(r *http.Request) (*http.Response, error) {
		select {
		case <-r.Context().Done():
			return nil, r.Context().Err()
		case <-time.After(200 * time.Millisecond):
			return nil, errors.New("parent cancellation did not reach the request")
		}
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err = SyncSpotify(ctx, db, 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("sync ignores its deadline: %v", err)
	}
}
