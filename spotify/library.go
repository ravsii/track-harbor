package spotify

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
)

const libraryQueryHash = "390c78e5b951029bad359785e69b07b536a509c581cbcd0aded5e5067f187455"
const likedSongsURI = "spotify:collection:tracks"

// Spotify's web-player bundle (module 87232) defines this built-in playlist URI
// for Liked Songs. The collection URI is a sidebar identifier, not a playlist.
const likedSongsPlaylistURI = "spotify:playlist:37i9dQZF1F5p3rmiWPIYgZ"

type libraryResponse struct {
	Data struct {
		Me struct {
			Library *struct {
				Items []struct {
					Item struct {
						URI  string `json:"_uri"`
						Data struct {
							URI    string `json:"uri"`
							Format string `json:"format"`
						} `json:"data"`
					} `json:"item"`
				} `json:"items"`
				TotalCount *int       `json:"totalCount"`
				PagingInfo PagingInfo `json:"pagingInfo"`
			} `json:"libraryV3"`
		} `json:"me"`
	} `json:"data"`
}

// Discover by semantic identifiers, never by a localized display name. Request
// the full flattened library, including folders; no user-specific playlist ID.
func FindLikedSongs(bearer, clientToken string, loggers ...*zap.Logger) (*PlaylistResponse, error) {
	logger := spotifyLogger(loggers)
	features := []string{"LIKED_SONGS"}
	expectedTotal := -1
	for offset := 0; ; {
		var response libraryResponse
		variables := map[string]any{
			"order": nil, "textFilter": "", "features": features,
			"limit": 50, "offset": offset, "flatten": true,
			"expandedFolders": []string{}, "folderUri": nil, "includeFoldersWhenFlattening": true,
		}
		if err := query(bearer, clientToken, "libraryV3", libraryQueryHash, variables, &response, loggers...); err != nil {
			return nil, err
		}
		library := response.Data.Me.Library
		if library == nil || library.TotalCount == nil || *library.TotalCount < 0 || library.Items == nil {
			return nil, fmt.Errorf("Spotify response is missing a valid library page")
		}
		if expectedTotal < 0 {
			expectedTotal = *library.TotalCount
		}
		if *library.TotalCount != expectedTotal {
			return nil, fmt.Errorf("Spotify library changed during discovery; run spotify sync again")
		}
		if library.PagingInfo.Offset != offset || len(library.Items) > *library.TotalCount-offset {
			return nil, fmt.Errorf("Spotify returned an invalid library offset or item count")
		}
		logger.Info("spotify_library_page", zap.Int("offset", offset), zap.Int("items", len(library.Items)), zap.Int("total", *library.TotalCount), zap.Strings("features", features))
		for _, entry := range library.Items {
			uri := entry.Item.Data.URI
			if uri == "" {
				uri = entry.Item.URI
			}
			if uri == likedSongsURI {
				logger.Info("spotify_liked_candidate", zap.String("uri", uri), zap.String("matched_by", "collection_uri"))
				logger.Info("spotify_collection_resolved", zap.String("collection_uri", uri), zap.String("playlist_uri", likedSongsPlaylistURI), zap.String("resolved_by", "web_player_builtin"))
				return resolveLikedPlaylist(bearer, clientToken, likedSongsPlaylistURI, loggers...)
			}
			if entry.Item.Data.Format == "liked-songs" && strings.HasPrefix(uri, "spotify:playlist:") {
				logger.Info("spotify_liked_candidate", zap.String("uri", diagnosticText(uri, bearer, clientToken)), zap.String("matched_by", "playlist_format"))
				return resolveLikedPlaylist(bearer, clientToken, uri, loggers...)
			}
		}
		offset += len(library.Items)
		if offset >= *library.TotalCount {
			break
		}
		if len(library.Items) == 0 {
			return nil, fmt.Errorf("Spotify returned an incomplete library page")
		}
	}
	return nil, fmt.Errorf("Liked Songs was not found in the Spotify library")
}

func resolveLikedPlaylist(bearer, clientToken, uri string, loggers ...*zap.Logger) (*PlaylistResponse, error) {
	info, err := fetchPlaylist(bearer, clientToken, uri, 0, "fetchPlaylist", loggers...)
	if err != nil {
		return nil, err
	}
	playlist := &info.Data.PlaylistV2
	if playlist.Format != "liked-songs" || !strings.HasPrefix(playlist.URI, "spotify:playlist:") {
		return nil, fmt.Errorf("Spotify did not return a verified Liked Songs playlist")
	}
	return info, nil
}
