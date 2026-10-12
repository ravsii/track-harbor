package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestUpgradeKeepsKnownVKFilesAndDoesNotGuessSpotifyIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"001_initial.sql", "002_download_control.sql", "003_download_settings.sql", "004_personal_proxies.sql"} {
		contents, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`PRAGMA user_version=4;
	INSERT INTO credentials VALUES('spotify','old-bearer','old-client');
	INSERT INTO credentials VALUES('vk','old-vk','');
	INSERT INTO tracks(source,artist,title,owner_id,audio_id,audio_url) VALUES('vk','Artist','Known',-123,456,'private-url');
	INSERT INTO tracks(source,artist,title) VALUES('spotify','Artist','Unknown');
	INSERT INTO downloads VALUES('vk','Artist','Known','known.mp3','vk-hash');
	INSERT INTO downloads VALUES('spotify','Artist','Unknown','unknown.mp3','spotify-hash');`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	tracks, err := s.ListAll()
	if err != nil || len(tracks) != 1 || tracks[0].SourceID != "-123_456" || tracks[0].FilePath != "known.mp3" {
		t.Fatalf("exact migration: %+v %v", tracks, err)
	}
	var oldTokens, oldFiles, privateURLs int
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM credentials`).Scan(&oldTokens); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM downloads`).Scan(&oldFiles); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM tracks WHERE audio_url<>''`).Scan(&privateURLs); err != nil {
		t.Fatal(err)
	}
	if oldTokens != 0 || oldFiles != 2 || privateURLs != 0 {
		t.Fatalf("old data: tokens=%d files=%d private URLs=%d", oldTokens, oldFiles, privateURLs)
	}
	admin, err := s.Authenticate("admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if items, err := s.ListUser(admin.ID); err != nil || len(items) != 0 {
		t.Fatal("bootstrap inherited legacy playlist")
	}
	key, err := s.SigningKey()
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.CreateUser("alice", "password")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CreateSession(u, "persistent-session", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteUser(admin.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.Authenticate("admin", "admin"); !errors.Is(err, ErrAuthentication) {
		t.Fatal("bootstrap recreated on restart")
	}
	if _, err = s.SessionUser("persistent-session", time.Now()); err != nil {
		t.Fatal("restart revoked valid session")
	}
	newKey, err := s.SigningKey()
	if err != nil || string(key) != string(newKey) {
		t.Fatal("signing key changed on restart")
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("database not private")
	}
}

func TestConcurrentDeletionCannotRemoveEveryUser(t *testing.T) {
	s := testStore(t)
	u, err := s.CreateUser("alice", "password")
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []int64{1, u.ID} {
		wg.Add(1)
		go func(id int64) { defer wg.Done(); <-start; results <- s.DeleteUser(id) }(id)
	}
	close(start)
	wg.Wait()
	close(results)
	deleted, blocked := 0, 0
	for err := range results {
		if err == nil {
			deleted++
		} else if errors.Is(err, ErrLastUser) {
			blocked++
		} else {
			t.Fatal(err)
		}
	}
	users, err := s.Users()
	if err != nil || len(users) != 1 || deleted != 1 || blocked != 1 {
		t.Fatalf("users=%v deleted=%d blocked=%d err=%v", users, deleted, blocked, err)
	}
}
