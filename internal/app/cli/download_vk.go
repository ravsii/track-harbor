package cli

import (
	"fmt"
	"io"
	"strings"

	"go.uber.org/zap"
	"trackharbor/internal/download"
	vkdownload "trackharbor/internal/download/vk"
	"trackharbor/internal/repository"
)

func downloadVKTracks(repo *repository.VKRepository, store *repository.DownloadedTracksRepository, outputDir string, logger *zap.Logger, out io.Writer) error {
	songs, err := repo.GetAudio()
	if err != nil {
		return fmt.Errorf("load VK audio: %w", err)
	}
	plain := make([]repository.Song, 0, len(songs))
	for _, song := range songs {
		plain = append(plain, song.Song)
	}
	queries := buildQueriesLogged(plain, logger)
	byQuery := make(map[string]repository.VKSong, len(songs))
	for _, song := range songs {
		query := buildQueries([]repository.Song{song.Song})
		if len(query) == 1 {
			previous, exists := byQuery[query[0]]
			if !exists || previous.URL == "" && song.URL != "" {
				byQuery[query[0]] = song
			}
		}
	}
	pending := make([]string, 0, len(queries))
	skipped := 0
	unavailable := 0
	for _, query := range queries {
		path, err := download.DownloadedQueryPath(store, "vk", query, outputDir)
		if err != nil {
			return fmt.Errorf("check VK download history: %w", err)
		}
		if path != "" {
			skipped++
			continue
		}
		if song, ok := byQuery[query]; !ok || song.OwnerID == 0 || song.AudioID <= 0 || !strings.HasPrefix(song.URL, "https://") {
			unavailable++
			logger.Warn("vk_audio_url_unavailable", zap.String("query", query))
			continue
		}
		pending = append(pending, query)
	}
	fmt.Fprintf(out, "VK strategy: direct. Already downloaded: %d; remaining: %d (FFmpeg, no YouTube).\n", skipped, len(pending))
	if unavailable > 0 {
		fmt.Fprintf(out, "VK: %d tracks have no usable audio URL and will be skipped (no YouTube fallback).\n", unavailable)
	}
	if len(pending) == 0 {
		return nil
	}
	provider := vkdownload.NewProvider(byQuery)
	// Download up to ten VK tracks concurrently, independently of YouTube workers.
	d := download.New(provider, 10, outputDir).WithStartInterval(0).WithDownloadStore(store).WithLogger(logger).WithDisplay(download.NewDisplay()).WithInitialSkipped(skipped)
	d.Run(pending)
	return nil
}
