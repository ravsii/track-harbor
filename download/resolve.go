package download

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/ravsii/trackharbor/youtube"
)

func youtubeVideo(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "www.youtube.com" || u.User != nil || u.Path != "/watch" {
		return ""
	}
	id := u.Query().Get("v")
	if !videoID.MatchString(id) {
		return ""
	}
	return id
}

// Resolve keeps catalog identity separate from the URL used to fetch its audio.
func Resolve(ctx context.Context, t Track, routes *Routes, event func(Event)) (string, error) {
	if t.Source == "vk" {
		if t.AudioURL == "" {
			return "", fmt.Errorf("VK audio URL unavailable; sync the source again")
		}
		return t.AudioURL, nil
	}
	if t.Source == "youtube" && videoID.MatchString(t.SourceID) {
		return "https://www.youtube.com/watch?v=" + t.SourceID, nil
	}
	if t.Source != "spotify" || !spotifyID.MatchString(t.SourceID) {
		return "", fmt.Errorf("invalid track source or key")
	}
	if youtubeVideo(t.AudioURL) != "" {
		return t.AudioURL, nil
	}
	if routes == nil {
		routes = &Routes{}
	}
	if event == nil {
		event = func(Event) {}
	}
	executable, err := exec.LookPath("yt-dlp")
	if err != nil {
		return "", fmt.Errorf("yt-dlp is required: %w", err)
	}
	var resolved string
	err = routes.try(ctx, event, "searching", func(proxy string) error {
		attemptCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		output, stderr, err := youtube.Run(attemptCtx, executable, proxy, "", nil,
			"--flat-playlist", "--dump-single-json", "--", "ytsearch1:"+strings.TrimSpace(t.Artist+" "+t.Title))
		if err != nil {
			return ytError(stderr, err)
		}
		var result struct {
			Entries []struct {
				ID string `json:"id"`
			} `json:"entries"`
		}
		if json.Unmarshal(output, &result) != nil || result.Entries == nil {
			return fmt.Errorf("invalid YouTube search response")
		}
		if len(result.Entries) == 0 {
			return errNoMatch
		}
		if !videoID.MatchString(result.Entries[0].ID) {
			return fmt.Errorf("invalid YouTube video ID")
		}
		resolved = "https://www.youtube.com/watch?v=" + result.Entries[0].ID
		return nil
	})
	return resolved, err
}
