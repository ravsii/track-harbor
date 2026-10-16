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

// YouTubeID accepts a single video ID or a supported YouTube URL, never a
// playlist or arbitrary URL. Only the canonical watch URL is sent to yt-dlp.
func YouTubeID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if videoID.MatchString(raw) {
		return raw, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Port() != "" {
		return "", fmt.Errorf("enter a YouTube video link or an 11-character video ID")
	}
	var id string
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	switch strings.ToLower(u.Hostname()) {
	case "youtu.be", "www.youtu.be":
		if len(parts) == 1 {
			id = parts[0]
		}
	case "youtube.com", "www.youtube.com", "m.youtube.com", "music.youtube.com":
		if u.Path == "/watch" {
			id = u.Query().Get("v")
		} else if len(parts) == 2 && (parts[0] == "shorts" || parts[0] == "embed" || parts[0] == "live") {
			id = parts[1]
		}
	}
	if !videoID.MatchString(id) {
		return "", fmt.Errorf("enter a single YouTube video link or an 11-character video ID")
	}
	return id, nil
}

func YouTubeTrack(ctx context.Context, id string, routes *Routes) (Track, error) {
	if !videoID.MatchString(id) {
		return Track{}, fmt.Errorf("invalid YouTube video ID")
	}
	executable, err := exec.LookPath("yt-dlp")
	if err != nil {
		return Track{}, fmt.Errorf("yt-dlp is required")
	}
	if routes == nil {
		routes = &Routes{}
	}
	track := Track{Source: "youtube", SourceID: id, AudioURL: "https://www.youtube.com/watch?v=" + id, CoverURL: "https://i.ytimg.com/vi/" + id + "/hqdefault.jpg"}
	err = routes.try(ctx, func(Event) {}, "checking", func(proxy string) error {
		attemptCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		output, stderr, err := youtube.Run(attemptCtx, executable, proxy, "", nil, "--no-playlist", "--simulate", "--dump-single-json", "--", track.AudioURL)
		if err != nil {
			return ytError(stderr, err)
		}
		var info struct {
			ID         string `json:"id"`
			Title      string `json:"title"`
			Artist     string `json:"artist"`
			Channel    string `json:"channel"`
			Uploader   string `json:"uploader"`
			LiveStatus string `json:"live_status"`
		}
		if json.Unmarshal(output, &info) != nil || info.ID != id || strings.TrimSpace(info.Title) == "" {
			return fmt.Errorf("invalid YouTube video response")
		}
		if info.LiveStatus == "is_live" || info.LiveStatus == "is_upcoming" {
			return errNoMatch
		}
		track.Title = strings.TrimSpace(info.Title)
		for _, artist := range []string{info.Artist, info.Channel, info.Uploader} {
			if strings.TrimSpace(artist) != "" {
				track.Artist = strings.TrimSpace(artist)
				break
			}
		}
		return nil
	})
	return track, err
}
