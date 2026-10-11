package spotify

import (
	"fmt"
	"strings"
)

const libraryQueryHash = "390c78e5b951029bad359785e69b07b536a509c581cbcd0aded5e5067f187455"
const likedSongsURI = "spotify:collection:tracks"

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
func FindLikedSongs(bearer, clientToken string) (*PlaylistResponse, error) {
	collectionFound := false
	// Without LIKED_SONGS the server may expose the underlying real playlist.
	// With it, the HAR shows a PseudoPlaylist whose URI is collection:tracks.
	for _, features := range [][]string{{}, {"LIKED_SONGS"}} {
		expectedTotal := -1
		for offset := 0; ; {
			var response libraryResponse
			variables := map[string]any{
				"order": nil, "textFilter": "", "features": features,
				"limit": 50, "offset": offset, "flatten": true,
				"expandedFolders": []string{}, "folderUri": nil, "includeFoldersWhenFlattening": true,
			}
			if err := query(bearer, clientToken, "libraryV3", libraryQueryHash, variables, &response); err != nil {
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
			for _, entry := range library.Items {
				uri := entry.Item.Data.URI
				if uri == "" {
					uri = entry.Item.URI
				}
				if uri == likedSongsURI {
					collectionFound = true
				}
				if entry.Item.Data.Format == "liked-songs" && strings.HasPrefix(uri, "spotify:playlist:") {
					return resolveLikedPlaylist(bearer, clientToken, uri)
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
	}
	if !collectionFound {
		return nil, fmt.Errorf("Liked Songs was not found in the Spotify library")
	}
	return resolveLikedPlaylist(bearer, clientToken, likedSongsURI)
}

func resolveLikedPlaylist(bearer, clientToken, uri string) (*PlaylistResponse, error) {
	info, err := fetchPlaylist(bearer, clientToken, uri, 0, "fetchPlaylist")
	if err != nil {
		return nil, err
	}
	playlist := &info.Data.PlaylistV2
	if playlist.Format != "liked-songs" || !(strings.HasPrefix(playlist.URI, "spotify:playlist:") || playlist.URI == likedSongsURI) {
		return nil, fmt.Errorf("Spotify did not return a verified Liked Songs playlist; capture opening /collection/tracks in a new HAR to inspect collection resolution")
	}
	return info, nil
}
