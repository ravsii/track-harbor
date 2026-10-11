package download

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestAudioFilenameIncludesArtistTitleAndStableID(t *testing.T) {
	track := Track{ID: "dQw4w9WgXcQ", Artist: "Rick Astley", Title: "Never Gonna Give You Up"}
	if got := audioFilename(track); got != "Rick Astley — Never Gonna Give You Up [dQw4w9WgXcQ].mp3" {
		t.Fatal(got)
	}
	other := track
	other.ID = "abcdefghijk"
	if audioFilename(track) == audioFilename(other) {
		t.Fatal("different videos collide")
	}
}

func TestAudioFilenameIsSafeAndPreservesUnicode(t *testing.T) {
	for _, track := range []Track{
		{ID: "video", Artist: "../Группа/\\", Title: "Песня:*?\"<>|\x00\n"},
		{ID: "video", Artist: strings.Repeat("Ж", 200), Title: strings.Repeat("曲", 200)},
		{ID: "video"},
	} {
		name := audioFilename(track)
		if strings.ContainsAny(name, "/\\:*?\"<>|\x00\n\r") || filepath.Base(name) != name || len(name) > 240 || !utf8.ValidString(name) || !strings.HasSuffix(name, "[video].mp3") {
			t.Fatalf("unsafe or unbounded name: %q", name)
		}
	}
	if name := audioFilename(Track{ID: "video", Artist: "Группа", Title: "Песня"}); !strings.Contains(name, "Группа — Песня") {
		t.Fatal("Unicode names lost")
	}
}

func TestVerifiedLegacyFileIsRenamedWithoutDownloading(t *testing.T) {
	_, store := cacheDatabase(t)
	dir := t.TempDir()
	legacy := filepath.Join(dir, "track.mp3")
	body := []byte("abcdef")
	if err := os.WriteFile(legacy, body, 0600); err != nil {
		t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(body))
	if err := store.SaveHash("test", "track", legacy, hash); err != nil {
		t.Fatal(err)
	}
	p := &cacheProvider{body: "unused", length: 6}
	display := runCached(t, p, dir, store, "query", "duplicate query")
	if p.calls.Load() != 0 {
		t.Fatal("legacy file downloaded again")
	}
	for _, ev := range display.done {
		if ev.Kind != StatusSkipped {
			t.Fatalf("verified rename failed: %+v", ev)
		}
	}
	file := filepath.Join(dir, "Artist — Title [track].mp3")
	got, err := os.ReadFile(file)
	if err != nil || string(got) != string(body) {
		t.Fatalf("renamed content lost: %s %v", got, err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("legacy name still exists")
	}
	saved, err := store.LookupHash("test", "track", file)
	if err != nil || saved != hash {
		t.Fatalf("new path hash missing: %s %v", saved, err)
	}
	runCached(t, p, dir, store, "query")
	if p.calls.Load() != 0 {
		t.Fatal("renamed hash not reused")
	}
}

func TestUntrackedOrCorruptLegacyFilesAreNotTrusted(t *testing.T) {
	for _, tracked := range []bool{false, true} {
		t.Run(fmt.Sprint(tracked), func(t *testing.T) {
			_, store := cacheDatabase(t)
			dir := t.TempDir()
			legacy := filepath.Join(dir, "track.mp3")
			if err := os.WriteFile(legacy, []byte("old file"), 0600); err != nil {
				t.Fatal(err)
			}
			if tracked {
				if err := store.SaveHash("test", "track", legacy, strings.Repeat("0", 64)); err != nil {
					t.Fatal(err)
				}
			}
			p := &cacheProvider{body: "abcdef", length: 6}
			display := runCached(t, p, dir, store, "query")
			if p.calls.Load() != 1 || display.done[0].Kind != StatusDone {
				t.Fatal("unverified legacy file trusted")
			}
			got, err := os.ReadFile(legacy)
			if err != nil || string(got) != "old file" {
				t.Fatal("unverified legacy file was modified")
			}
		})
	}
}

func TestFailedMigrationKeepsOriginalFile(t *testing.T) {
	db, store := cacheDatabase(t)
	dir := t.TempDir()
	legacy := filepath.Join(dir, "track.mp3")
	body := []byte("abcdef")
	if err := os.WriteFile(legacy, body, 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveHash("test", "track", legacy, fmt.Sprintf("%x", sha256.Sum256(body))); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_migration BEFORE INSERT ON downloaded_tracks BEGIN SELECT RAISE(ABORT, 'write failed'); END`); err != nil {
		t.Fatal(err)
	}
	p := &cacheProvider{body: "unused", length: 6}
	display := runCached(t, p, dir, store, "query")
	if display.done[0].Kind != StatusFailed || p.calls.Load() != 0 {
		t.Fatal("failed migration reported success or redownloaded")
	}
	got, err := os.ReadFile(legacy)
	if err != nil || string(got) != string(body) {
		t.Fatal("migration discarded original")
	}
	if _, err := os.Stat(filepath.Join(dir, "Artist — Title [track].mp3")); !os.IsNotExist(err) {
		t.Fatal("failed migration left untracked target")
	}
}

func TestMigrationDoesNotClobberExistingTarget(t *testing.T) {
	_, store := cacheDatabase(t)
	dir := t.TempDir()
	legacy := filepath.Join(dir, "track.mp3")
	body := []byte("abcdef")
	if err := os.WriteFile(legacy, body, 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveHash("test", "track", legacy, fmt.Sprintf("%x", sha256.Sum256(body))); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "Artist — Title [track].mp3")
	if err := os.WriteFile(target, []byte("existing target"), 0600); err != nil {
		t.Fatal(err)
	}
	display := runCached(t, &cacheProvider{body: "partial", length: 100}, dir, store, "query")
	if display.done[0].Kind != StatusFailed {
		t.Fatal("untracked existing file trusted")
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "existing target" {
		t.Fatal("legacy migration overwrote existing target")
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatal("legacy file removed without migrating")
	}
}
