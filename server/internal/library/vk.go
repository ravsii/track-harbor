package library

import (
	"context"
	"fmt"
	"strings"

	"trackharbor/server/internal/store"
	"trackharbor/vk"
)

func SyncVK(ctx context.Context, db *store.Store) (int, error) {
	token, _, err := db.Credentials("vk")
	if err != nil {
		return 0, fmt.Errorf("load VK credentials: %w", err)
	}
	client := vk.NewClient(token)
	defer client.Close()
	songs, err := client.MyMusic(ctx, nil)
	if err != nil {
		return 0, err
	}
	tracks := make([]store.Track, 0, len(songs))
	for _, song := range songs {
		artist, title := strings.TrimSpace(song.Artist), strings.TrimSpace(song.Title)
		if artist != "" && title != "" {
			tracks = append(tracks, store.Track{Artist: artist, Title: title})
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return db.Replace("vk", tracks)
}
