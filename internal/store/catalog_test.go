package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCatalogSharedFilesPrivatePlaylists(t *testing.T) {
	s := testStore(t)
	a, _ := s.CreateUser("alice", "password")
	b, _ := s.CreateUser("bob", "password")
	track := Track{SourceID: "same-id", Artist: "Artist", Title: "Song", AddedAt: 100, AudioURL: "alice-url"}
	if _, err := s.Replace(a.ID, "spotify", []Track{track}); err != nil {
		t.Fatal(err)
	}
	track.AddedAt, track.AudioURL = 200, "bob-url"
	if _, err := s.Replace(b.ID, "spotify", []Track{track}); err != nil {
		t.Fatal(err)
	}
	all, err := s.ListAll()
	if err != nil || len(all) != 1 {
		t.Fatalf("catalog: %v %v", all, err)
	}
	if all[0].AudioURL != "bob-url" {
		t.Fatal("shared downloader selected older signed URL")
	}
	p := filepath.Join(t.TempDir(), "shared.mp3")
	if err := os.WriteFile(p, []byte("mp3-data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDownload(all[0], p, "hash"); err != nil {
		t.Fatal(err)
	}
	for _, u := range []User{a, b} {
		items, err := s.ListUser(u.ID)
		if err != nil || len(items) != 1 || items[0].FilePath != p {
			t.Fatalf("shared download: %v %v", items, err)
		}
		if u.ID == a.ID && (items[0].AddedAt != 100 || items[0].AudioURL != "alice-url") {
			t.Fatal("other user overwrote personal metadata")
		}
	}
	if _, err := s.Replace(a.ID, "spotify", nil); err != nil {
		t.Fatal(err)
	}
	items, _ := s.ListUser(a.ID)
	if len(items) != 0 {
		t.Fatal("sync did not remove membership")
	}
	items, _ = s.ListUser(b.ID)
	if len(items) != 1 {
		t.Fatal("sync removed other user's track")
	}
	if err := s.DeleteUser(b.ID); err != nil {
		t.Fatal(err)
	}
	all, _ = s.ListAll()
	if len(all) != 1 || all[0].FilePath != p {
		t.Fatal("deleted shared file index")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal("deleted music file")
	}
}

func TestIDsNotNamesAndCredentialsIsolation(t *testing.T) {
	s := testStore(t)
	a, _ := s.CreateUser("alice", "password")
	b, _ := s.CreateUser("bob", "password")
	if err := s.SaveCredentials(a.ID, "vk", "alice-token", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveCredentials(b.ID, "vk", "bob-token", ""); err != nil {
		t.Fatal(err)
	}
	v, _, err := s.Credentials(a.ID, "vk")
	if err != nil || v != "alice-token" {
		t.Fatal("credentials leaked")
	}
	tracks := []Track{{SourceID: "id-one", Artist: "Same", Title: "Name"}, {SourceID: "id-two", Artist: "Same", Title: "Name"}}
	if _, err := s.Replace(a.ID, "spotify", tracks); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Replace(a.ID, "vk", tracks[:1]); err != nil {
		t.Fatal(err)
	}
	all, _ := s.ListAll()
	if len(all) != 3 {
		t.Fatalf("IDs conflated: %v", all)
	}
	tracks[0].Title = "Renamed"
	if _, err := s.Replace(a.ID, "spotify", tracks); err != nil {
		t.Fatal(err)
	}
	all, _ = s.ListAll()
	if len(all) != 3 {
		t.Fatal("renamed track duplicated")
	}
	if _, err := s.Replace(a.ID, "spotify", []Track{{Title: "Missing ID"}}); err == nil {
		t.Fatal("invented source ID")
	}
	items, _ := s.List(a.ID, "spotify")
	if len(items) != 2 {
		t.Fatal("failed sync destroyed playlist")
	}
}
