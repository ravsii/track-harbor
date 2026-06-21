package mp3party

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

const (
	baseSearchURL = "https://mp3party.net/search"
	baseTrackURL  = "https://mp3party.net/music"
)

func FindMusics(songName string) ([]Track, error) {
	u, err := url.Parse(baseSearchURL)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}

	query := u.Query()
	query.Set("q", songName)
	u.RawQuery = query.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// optional but helps avoid some basic bot blocking
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Go scraper)")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var tracks []Track

	doc.Find("div.track.song-item").Each(func(i int, s *goquery.Selection) {
		userPanel := s.Find(".track__user-panel")

		songID := userPanel.AttrOr("data-js-id", "")
		artist := userPanel.AttrOr("data-js-artist-name", "")
		title := userPanel.AttrOr("data-js-song-title", "")
		playURL := userPanel.AttrOr("data-js-url", "")
		isPlaylist := userPanel.AttrOr("data-is-playlist", "false")

		// link to track page
		pageURL, _ := s.Find("a.track__title").Attr("href")

		// title text fallback (optional)
		titleText := strings.TrimSpace(s.Find("a.track__title").Text())

		// info items: duration, size, bitrate
		var info []string
		s.Find(".track__info-item").Each(func(_ int, el *goquery.Selection) {
			info = append(info, strings.TrimSpace(el.Text()))
		})

		if title == "" {
			title = titleText
		}

		track := Track{
			SongID:     songID,
			Artist:     artist,
			Title:      title,
			URL:        playURL,
			PageURL:    pageURL,
			IsPlaylist: isPlaylist == "true",
			PlayURL:    playURL,
		}

		if len(info) > 0 {
			track.Duration = info[0]
		}
		if len(info) > 1 {
			track.Size = info[1]
		}
		if len(info) > 2 {
			track.Bitrate = info[2]
		}

		tracks = append(tracks, track)
	})

	return tracks, nil
}

func pingTrackPage(trackID string) error {
	trackURL := fmt.Sprintf("%s/%s", baseTrackURL, trackID)

	req, err := http.NewRequest(http.MethodGet, trackURL, nil)
	if err != nil {
		return fmt.Errorf("create ping request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Go scraper)")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return fmt.Errorf("ping track page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ping track page: bad status %s", resp.Status)
	}

	return nil
}

func Download(track Track) ([]byte, error) {
	if err := pingTrackPage(track.SongID); err != nil {
		return nil, fmt.Errorf("ping before download: %w", err)
	}

	req, err := http.NewRequest(http.MethodGet, track.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// optional but helps avoid some basic bot blocking
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Go scraper)")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}
