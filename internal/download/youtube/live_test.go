package youtube

import (
	"crypto/sha256"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"meshvault/internal/download"
)

// Opt-in: this invokes real tools, accesses YouTube and downloads audio.
func TestLiveYouTubeSearchAndDownload(t *testing.T) {
	if os.Getenv("MESHVAULT_YOUTUBE_LIVE") != "1" {
		t.Skip("set MESHVAULT_YOUTUBE_LIVE=1 to download the test video")
	}
	p, err := NewProvider()
	if err != nil {
		t.Fatal(err)
	}
	t.Run("search", func(t *testing.T) {
		track, err := p.Search("Rick Astley Never Gonna Give You Up")
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("search: id=%s title=%s", track.ID, track.Title)
	})
	t.Run("download", func(t *testing.T) {
		response, err := p.Download(download.Track{ID: "dQw4w9WgXcQ"})
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		file := response.Body.(*temporaryAudio).File.Name()
		hash := sha256.New()
		written, err := io.Copy(hash, response.Body)
		if err != nil || written != response.ContentLength || written == 0 {
			t.Fatalf("incomplete MP3: %d/%d, %v", written, response.ContentLength, err)
		}
		// Verify actual media, not just a file with an .mp3 extension.
		probe, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "a:0", "-show_entries", "stream=codec_name", "-of", "default=noprint_wrappers=1:nokey=1", file).Output()
		if err != nil || strings.TrimSpace(string(probe)) != "mp3" {
			t.Fatalf("not an MP3 stream: %s, %v", probe, err)
		}
		duration, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", file).Output()
		if err != nil {
			t.Fatal(err)
		}
		seconds, err := strconv.ParseFloat(strings.TrimSpace(string(duration)), 64)
		if err != nil || seconds < 200 || seconds > 230 {
			t.Fatalf("unexpected video duration: %s", duration)
		}
		t.Logf("downloaded MP3: %d bytes, %.1f seconds, SHA256=%x", written, seconds, hash.Sum(nil))
	})
}
