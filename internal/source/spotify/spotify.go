package spotify

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

const playlistQueryHash = "8964e8eafb21aa992a7d951d256d83285c04be2105d209262901de70cb97584a"

func GetSongInfo(
	bearer string,
	clientToken string,
) (*PlaylistResponse, error) {
	return FindLikedSongs(bearer, clientToken)
}

// GetSongInfoPage fetches up to 50 playlist entries starting at offset.
func GetSongInfoPage(bearer, clientToken string, offset int) (*PlaylistResponse, error) {
	info, err := FindLikedSongs(bearer, clientToken)
	if err != nil {
		return nil, err
	}
	return GetPlaylistPage(bearer, clientToken, info.Data.PlaylistV2.URI, offset)
}

func GetPlaylistPage(bearer, clientToken, uri string, offset int) (*PlaylistResponse, error) {
	return fetchPlaylist(bearer, clientToken, uri, offset, "fetchPlaylistContents")
}

func fetchPlaylist(bearer, clientToken, uri string, offset int, operation string) (*PlaylistResponse, error) {
	var response struct {
		Data struct {
			PlaylistV2 json.RawMessage `json:"playlistV2"`
		} `json:"data"`
	}
	variables := map[string]any{"uri": uri, "offset": offset, "limit": 50, "includeEpisodeContentRatingsV2": true}
	if operation == "fetchPlaylist" {
		variables["enableWatchFeedEntrypoint"] = true
	}
	if err := query(bearer, clientToken, operation, playlistQueryHash, variables, &response); err != nil {
		return nil, err
	}
	if len(response.Data.PlaylistV2) == 0 || string(response.Data.PlaylistV2) == "null" {
		return nil, fmt.Errorf("Spotify response is missing the playlist")
	}
	var shape struct {
		Typename string `json:"__typename"`
		Content  *struct {
			Items      *json.RawMessage `json:"items"`
			TotalCount *int             `json:"totalCount"`
		} `json:"content"`
	}
	if err := json.Unmarshal(response.Data.PlaylistV2, &shape); err != nil {
		return nil, fmt.Errorf("read Spotify playlist: %w", err)
	}
	if shape.Typename == "NotFound" {
		return nil, fmt.Errorf("Spotify could not resolve the Liked Songs collection to a playlist")
	}
	if shape.Content == nil || shape.Content.Items == nil || string(*shape.Content.Items) == "null" {
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
		return nil, fmt.Errorf("Spotify returned an unexpected playlist offset")
	}
	return &responseFull, nil
}

func query(bearer, clientToken, operation, hash string, variables any, result any) error {
	payload, err := json.Marshal(map[string]any{
		"operationName": operation, "variables": variables,
		"extensions": map[string]any{"persistedQuery": map[string]any{"version": 1, "sha256Hash": hash}},
	})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api-partner.spotify.com/pathfinder/v2/query", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Add("accept-encoding", "gzip, zstd")
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
		return fmt.Errorf("Spotify request failed: %w", err)
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	var reader io.Reader = resp.Body
	switch strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding"))) {
	case "", "identity":
	case "gzip":
		decoder, err := gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("decompress Spotify response: %w", err)
		}
		defer decoder.Close()
		reader = decoder
	case "zstd":
		decoder, err := zstd.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("decompress Spotify response: %w", err)
		}
		defer decoder.Close()
		reader = decoder
	default:
		return fmt.Errorf("unsupported Spotify response encoding")
	}
	data, err := io.ReadAll(io.LimitReader(reader, 16*1024*1024+1))
	if err != nil {
		return fmt.Errorf("read Spotify response: %w", err)
	}
	if len(data) > 16*1024*1024 {
		return fmt.Errorf("Spotify response exceeds size limit")
	}
	var envelope struct {
		Errors []json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("read Spotify response: %w", err)
	}
	// Never expose raw API error messages, which may include session details.
	if len(envelope.Errors) > 0 {
		return fmt.Errorf("Spotify returned a request error for %s", operation)
	}
	return json.Unmarshal(data, result)
}
