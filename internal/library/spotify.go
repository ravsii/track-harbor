package library

import (
	"context"
	"fmt"
	"strings"
	"time"

	"trackharbor/internal/store"
	"trackharbor/spotify"
)

func SyncSpotify(ctx context.Context, db *store.Store) (int, error) {
	first, second, err := db.Credentials("spotify")
	if err != nil {
		return 0, fmt.Errorf("load Spotify credentials: %w", err)
	}
	info, err := spotify.FindLikedSongs(first, second)
	if err != nil {
		return 0, err
	}
	playlist := info.Data.PlaylistV2
	total := playlist.Content.TotalCount
	if total < 0 || len(playlist.Content.Items) > total || len(playlist.Content.Items) == 0 && total > 0 {
		return 0, fmt.Errorf("Spotify returned an invalid first page")
	}
	for len(playlist.Content.Items) < total {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		offset := len(playlist.Content.Items)
		next, err := spotify.GetPlaylistPage(first, second, playlist.URI, offset)
		if err != nil {
			return 0, err
		}
		page := next.Data.PlaylistV2.Content
		if page.TotalCount != total || page.PagingInfo.Offset != offset || len(page.Items) == 0 || len(page.Items) > total-offset {
			return 0, fmt.Errorf("Spotify returned an incomplete or changed library")
		}
		playlist.Content.Items = append(playlist.Content.Items, page.Items...)
	}
	tracks := make([]store.Track, 0, len(playlist.Content.Items))
	for _, item := range playlist.Content.Items {
		title := item.ItemV3.Data.IdentityTrait.Name
		artists := make([]string, 0, len(item.ItemV3.Data.IdentityTrait.Contributors.Items))
		for _, c := range item.ItemV3.Data.IdentityTrait.Contributors.Items {
			if c.Name != "" {
				artists = append(artists, c.Name)
			}
		}
		if title == "" {
			title = item.ItemV2.Data.Name
			artists = artists[:0]
			for _, a := range item.ItemV2.Data.Artists.Items {
				if a.Profile.Name != "" {
					artists = append(artists, a.Profile.Name)
				}
			}
		}
		if strings.TrimSpace(title) != "" {
			addedAt, err := time.Parse(time.RFC3339Nano, item.AddedAt.ISOString)
			var timestamp int64
			if err != nil {
				// Some older entries have no timestamp; do not fabricate a source date.
				timestamp = 0
			} else {
				timestamp = addedAt.Unix()
			}
			cover := ""
			for _, image := range item.ItemV2.Data.AlbumOfTrack.CoverArt.Sources {
				if image.URL != "" && (cover == "" || image.Width >= 300) {
					cover = image.URL
				}
			}
			if cover == "" {
				for _, image := range item.ItemV3.Data.VisualIdentityTrait.SquareCoverImage.Image.Data.Sources {
					if image.URL != "" && (cover == "" || image.MaxWidth >= 300) {
						cover = image.URL
					}
				}
			}
			tracks = append(tracks, store.Track{Artist: strings.Join(artists, ", "), Title: title, AddedAt: timestamp, CoverURL: cover})
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return db.Replace("spotify", tracks)
}
