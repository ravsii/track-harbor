package mp3party

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"meshvault/internal/download"
)

const (
	baseSearchURL      = "https://mp3party.net/search"
	baseTrackURL       = "https://mp3party.net/music"
	baseDownloadURL    = "https://dl2.mp3party.net/download"
	songPermissionsURL = "https://mp3party.net/song_permissions/"
)

var defaultUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:150.0) Gecko/20100101 Firefox/150.0"

// Provider реализует download.Provider для mp3party.net
type Provider struct {
	client *http.Client
}

func NewProvider() *Provider {
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(fmt.Sprintf("create cookie jar: %v", err))
	}
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	return &Provider{
		client: &http.Client{
			Jar:       jar,
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

func (p *Provider) Name() string { return "mp3party" }

func (p *Provider) Search(query string) (download.Track, error) {
	tracks, err := p.findMusics(query)
	if err != nil {
		return download.Track{}, err
	}
	return firstToCommon(tracks)
}

func (p *Provider) Download(track download.Track) (download.DownloadResponse, error) {
	if err := p.pingTrackPage(track.ID); err != nil {
		return download.DownloadResponse{}, fmt.Errorf("ping before download: %w", err)
	}

	req, err := http.NewRequest(http.MethodGet, baseDownloadURL+"/"+track.ID, nil)
	if err != nil {
		return download.DownloadResponse{}, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", defaultUA)

	resp, err := p.client.Do(req)
	if err != nil {
		return download.DownloadResponse{}, fmt.Errorf("do request: %w", err)
	}

	return download.DownloadResponse{
		Body:          resp.Body,
		ContentLength: resp.ContentLength,
	}, nil
}

func (p *Provider) findMusics(songName string) ([]trackInternal, error) {
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

	req.Header.Set("User-Agent", defaultUA)

	resp, err := p.client.Do(req)
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

	var tracks []trackInternal

	doc.Find("div.track.song-item").Each(func(i int, s *goquery.Selection) {
		userPanel := s.Find(".track__user-panel")

		songID := userPanel.AttrOr("data-js-id", "")
		artist := userPanel.AttrOr("data-js-artist-name", "")
		title := userPanel.AttrOr("data-js-song-title", "")
		playURL := userPanel.AttrOr("data-js-url", "")
		isPlaylist := userPanel.AttrOr("data-is-playlist", "false")

		pageURL, _ := s.Find("a.track__title").Attr("href")

		titleText := strings.TrimSpace(s.Find("a.track__title").Text())

		var info []string
		s.Find(".track__info-item").Each(func(_ int, el *goquery.Selection) {
			info = append(info, strings.TrimSpace(el.Text()))
		})

		if title == "" {
			title = titleText
		}

		track := trackInternal{
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

func (p *Provider) pingTrackPage(trackID string) error {
	trackURL := fmt.Sprintf("%s/%s", baseTrackURL, trackID)

	// Step 1: GET track page — получаем cookies и CSRF-токен
	req, err := http.NewRequest(http.MethodGet, trackURL, nil)
	if err != nil {
		return fmt.Errorf("create track page request: %w", err)
	}
	req.Header.Set("User-Agent", defaultUA)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("get track page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get track page: bad status %s", resp.Status)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return fmt.Errorf("parse track page: %w", err)
	}

	csrfToken, exists := doc.Find("meta[name=csrf-token]").Attr("content")
	if !exists {
		return fmt.Errorf("csrf token not found on track page")
	}

	// Step 2: POST song_permissions только для текущего трека
	payload := map[string][]string{"ids": {trackID}}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal permissions payload: %w", err)
	}

	req2, err := http.NewRequest(http.MethodPost, songPermissionsURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return fmt.Errorf("create permissions request: %w", err)
	}

	req2.Header.Set("User-Agent", defaultUA)
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-CSRF-Token", csrfToken)
	req2.Header.Set("Referer", trackURL)
	req2.Header.Set("Accept", "application/json")
	req2.Header.Set("Origin", "https://mp3party.net")

	resp2, err := p.client.Do(req2)
	if err != nil {
		return fmt.Errorf("send permissions request: %w", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK && resp2.StatusCode != http.StatusNoContent {
		return fmt.Errorf("permissions request: bad status %s", resp2.Status)
	}

	return nil
}
