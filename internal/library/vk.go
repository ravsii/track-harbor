package library

import (
	"context"
	"fmt"
	"strings"

	"github.com/ravsii/trackharbor/internal/store"
	"github.com/ravsii/trackharbor/vk"
)

func SyncVK(ctx context.Context, db *store.Store, userID int64) (int, error) {
	var credentials vk.Credentials
	if err := db.Credentials(userID, "vk", &credentials); err != nil {
		return 0, fmt.Errorf("load VK credentials: %w", err)
	}
	client := vk.NewClient(credentials.Token)
	defer client.Close()
	songs, err := client.MyMusic(ctx, nil)
	if err != nil {
		return 0, err
	}
	tracks := make([]store.Track, 0, len(songs))
	for _, song := range songs {
		artist, title := strings.TrimSpace(song.Artist), strings.TrimSpace(song.Title)
		if artist != "" && title != "" {
			addedAt := max(song.Date, 0)
			cover := song.Album.Thumb.Photo600
			if cover == "" {
				cover = song.Album.Thumb.Photo300
			}
			tracks = append(tracks, store.Track{SourceID: fmt.Sprintf("%d_%d", song.OwnerID, song.ID), Artist: artist, Title: title, AddedAt: addedAt, AudioURL: song.URL, CoverURL: cover})
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return db.Replace(userID, "vk", tracks)
}
