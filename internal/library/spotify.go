package library

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ravsii/trackharbor/internal/store"
	"github.com/ravsii/trackharbor/spotify"
)

func SyncSpotify(ctx context.Context, db *store.Store, userID int64) (count int, syncErr error) {
	stage, reason := "credentials", "operation_failed"
	logger := zap.L()
	defer func() {
		if syncErr != nil {
			// Fixed categories only: never log the provider error or credentials.
			logger.Error("spotify sync stage failed", zap.String("stage", stage), zap.String("reason", reason), zap.Bool("unauthorized", errors.Is(syncErr, spotify.ErrUnauthorized)), zap.Bool("timed_out", errors.Is(syncErr, context.DeadlineExceeded)))
		}
	}()
	first, second, err := db.Credentials(userID, "spotify")
	if err != nil {
		return 0, fmt.Errorf("load Spotify credentials: %w", err)
	}
	stage = "discover_liked_songs"
	info, err := spotify.FindLikedSongsContext(ctx, first, second)
	if err != nil {
		return 0, err
	}
	playlist := info.Data.PlaylistV2
	total := playlist.Content.TotalCount
	stage = "validate_first_page"
	if total < 0 || len(playlist.Content.Items) > total || len(playlist.Content.Items) == 0 && total > 0 {
		reason = "invalid_first_page"
		return 0, fmt.Errorf("Spotify returned an invalid first page")
	}
	for len(playlist.Content.Items) < total {
		stage = "paginate_tracks"
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		offset := len(playlist.Content.Items)
		next, err := spotify.GetPlaylistPageContext(ctx, first, second, playlist.URI, offset)
		if err != nil {
			return 0, err
		}
		page := next.Data.PlaylistV2.Content
		if page.TotalCount != total || page.PagingInfo.Offset != offset || len(page.Items) == 0 || len(page.Items) > total-offset {
			reason = "incomplete_or_changed_library"
			return 0, fmt.Errorf("Spotify returned an incomplete or changed library")
		}
		playlist.Content.Items = append(playlist.Content.Items, page.Items...)
	}
	stage = "read_track_metadata"
	tracks := make([]store.Track, 0, len(playlist.Content.Items))
	skippedLocal := 0
	for index, item := range playlist.Content.Items {
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
			id, err := spotifyTrackID(item)
			if err != nil {
				v3Kind := spotifyURIKind(item.ItemV3.Data.URI)
				v2Kind := spotifyURIKind(item.ItemV2.Data.URI)
				if (v3Kind == "local" && (v2Kind == "missing" || v2Kind == "local")) || (v2Kind == "local" && v3Kind == "missing") {
					// Local files have no Spotify catalog ID or remotely available
					// audio. Exclude them without blocking the catalog tracks.
					skippedLocal++
					continue
				}
				reason = "track_id_unavailable"
				logger = logger.With(
					zap.Int("item_index", index),
					zap.String("v3_uri_kind", v3Kind),
					zap.Int("v3_uri_length", len(item.ItemV3.Data.URI)),
					zap.String("v2_uri_kind", v2Kind),
					zap.Int("v2_uri_length", len(item.ItemV2.Data.URI)),
				)
				return 0, err
			}
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
			tracks = append(tracks, store.Track{SourceID: id, Artist: strings.Join(artists, ", "), Title: title, AddedAt: timestamp, CoverURL: cover})
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	stage = "save_playlist"
	count, syncErr = db.Replace(userID, "spotify", tracks)
	if syncErr == nil && skippedLocal > 0 {
		logger.Warn("spotify local files excluded from sync", zap.Int("skipped_local", skippedLocal))
	}
	return count, syncErr
}

var spotifyID = regexp.MustCompile(`^spotify:track:([A-Za-z0-9]{22})$`)

// Report only known URI categories, never identifiers or local file metadata.
func spotifyURIKind(uri string) string {
	if uri == "" {
		return "missing"
	}
	for _, kind := range []string{"track", "local", "episode", "album"} {
		if strings.HasPrefix(uri, "spotify:"+kind+":") {
			return kind
		}
	}
	return "other"
}

func spotifyTrackID(item spotify.Item) (string, error) {
	for _, uri := range []string{item.ItemV3.Data.URI, item.ItemV2.Data.URI} {
		if parts := spotifyID.FindStringSubmatch(uri); parts != nil {
			return parts[1], nil
		}
	}
	return "", fmt.Errorf("Spotify track ID is unavailable; previous playlist was kept")
}
