package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSpotifyIDsHaveIndependentFilesEvenWhenYouTubeMatchIsShared(t *testing.T) {
	dir := t.TempDir()
	fixture := filepath.Join(dir, "input.mp3")
	cmd := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "anullsrc=r=44100:cl=mono", "-t", "1", "-codec:a", "libmp3lame", "-q:a", "9", fixture)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("audio fixture: %v %s", err, output)
	}
	fake := `#!/bin/sh
set -eu
search=0
output=''
while [ "$#" -gt 0 ]; do
  case "$1" in
    --dump-single-json) search=1 ;;
    --output) shift; output="$1" ;;
  esac
  shift
done
if [ "$search" = 1 ]; then
  printf '%s\n' '{"entries":[{"id":"sameVIDEOid"}]}'
else
  test -n "$output"
  cp "$TRACKHARBOR_FIXTURE_AUDIO" "${output%.*}.mp3"
fi
`
	if err := os.WriteFile(filepath.Join(dir, "yt-dlp"), []byte(fake), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TRACKHARBOR_FIXTURE_AUDIO", fixture)
	save := func(id, title, tracked string) (string, string) {
		t.Helper()
		routes := &Routes{Order: []string{"direct"}}
		// The subprocess boundary is local; omit optional artwork without making
		// a network request by disabling routes after the audio phase.
		path, hash, err := Save(context.Background(), Track{Source: "spotify", SourceID: id, Artist: "Artist", Title: title}, dir, tracked, routes, func(event Event) {
			if event.Stage == "metadata" {
				routes.Order = nil
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		return path, hash
	}
	first, _ := save("abcdefghijklmnopqrstuv", "First", "")
	second, secondHash := save("ABCDEFGHIJKLMNOPQRSTUV", "Second", "")
	if first == second {
		t.Fatal("distinct Spotify IDs share a mutable file")
	}
	if filepath.Base(first) != "6162636465666768696a6b6c6d6e6f70717273747576.mp3" || filepath.Base(second) != "4142434445464748494a4b4c4d4e4f50515253545556.mp3" {
		t.Fatal("files do not preserve source identity")
	}
	if err := os.WriteFile(first, []byte("corrupted audio"), 0600); err != nil {
		t.Fatal(err)
	}
	save("abcdefghijklmnopqrstuv", "Repaired", first)
	data, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != secondHash {
		t.Fatal("repair changed another identity's file/checksum")
	}
}
