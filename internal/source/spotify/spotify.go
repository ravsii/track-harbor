package spotify

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/klauspost/compress/zstd"
)

func GetSongInfo(
	bearer string,
	clientToken string,
) (*PlaylistResponse, error) {
	return GetSongInfoPage(bearer, clientToken, 0)
}

// GetSongInfoPage fetches up to 50 playlist entries starting at offset.
func GetSongInfoPage(bearer, clientToken string, offset int) (*PlaylistResponse, error) {
	url := "https://api-partner.spotify.com/pathfinder/v2/query"

	payload := strings.NewReader(fmt.Sprintf(`{
		"variables": {
			"uri": "spotify:playlist:37i9dQZF1F5p3rmiWPIYgZ",
			"offset": %d,
			"limit": 50,
			"includeEpisodeContentRatingsV2": true
		},
		"operationName": "fetchPlaylistContents",
		"extensions": {
			"persistedQuery": {
				"version": 1,
				"sha256Hash": "a65e12194ed5fc443a1cdebed5fabe33ca5b07b987185d63c72483867ad13cb4"
			}
		}
	}`, offset))

	req, _ := http.NewRequest("POST", url, payload)

	req.Header.Add("accept-encoding", "gzip, deflate, br, zstd")
	req.Header.Add("user-agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:150.0) Gecko/20100101 Firefox/150.0")
	req.Header.Add("accept", "application/json")
	req.Header.Add("accept-language", "en")
	req.Header.Add("content-type", "application/json;charset=UTF-8")
	req.Header.Add("referer", "https://open.spotify.com/")
	req.Header.Add("app-platform", "WebPlayer")
	req.Header.Add("spotify-app-version", "1.2.93.434.gba0d48fa")
	req.Header.Add("client-token", clientToken)
	req.Header.Add("origin", "https://open.spotify.com")
	req.Header.Add("sec-gpc", "1")
	req.Header.Add("sec-fetch-dest", "empty")
	req.Header.Add("sec-fetch-mode", "cors")
	req.Header.Add("sec-fetch-site", "same-site")
	req.Header.Add("authorization", bearer)
	req.Header.Add("connection", "keep-alive")
	req.Header.Add("priority", "u=4")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	decoder, err := zstd.NewReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("decompress response: %w", err)
	}
	defer decoder.Close()

	var response struct {
		Data struct {
			PlaylistV2 json.RawMessage `json:"playlistV2"`
		} `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	if err := json.NewDecoder(decoder).Decode(&response); err != nil {
		return nil, fmt.Errorf("read Spotify response: %w", err)
	}
	if len(response.Errors) > 0 {
		// Do not log response messages: they may contain session information.
		return nil, fmt.Errorf("Spotify returned a request error")
	}
	if len(response.Data.PlaylistV2) == 0 || string(response.Data.PlaylistV2) == "null" {
		return nil, fmt.Errorf("Spotify response is missing the playlist")
	}
	var shape struct {
		Content *struct {
			Items      *json.RawMessage `json:"items"`
			TotalCount *int             `json:"totalCount"`
		} `json:"content"`
	}
	if err := json.Unmarshal(response.Data.PlaylistV2, &shape); err != nil {
		return nil, fmt.Errorf("read Spotify playlist: %w", err)
	}
	if shape.Content == nil || shape.Content.Items == nil {
		return nil, fmt.Errorf("Spotify response is missing the track list")
	}
	if shape.Content.TotalCount == nil || *shape.Content.TotalCount < 0 {
		return nil, fmt.Errorf("Spotify response is missing a valid track count")
	}
	var responseFull PlaylistResponse
	if err := json.Unmarshal(response.Data.PlaylistV2, &responseFull.Data.PlaylistV2); err != nil {
		return nil, fmt.Errorf("read Spotify playlist: %w", err)
	}
	if responseFull.Data.PlaylistV2.Content.PagingInfo.Offset != offset {
		return nil, fmt.Errorf("Spotify returned offset %d instead of %d", responseFull.Data.PlaylistV2.Content.PagingInfo.Offset, offset)
	}
	return &responseFull, nil
}
