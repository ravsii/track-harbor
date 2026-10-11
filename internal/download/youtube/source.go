package youtube

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"meshvault/internal/config"
	"meshvault/internal/download"
)

// Only a successfully completed yt-dlp output is published in the source cache.
// Partial transfers stay in isolated staging directories, never reused as audio.
func (p *Provider) prepareSource(ctx context.Context, track download.Track, onProxy func(string), onProgress func(download.DownloadProgress)) (string, error) {
	root := p.sourceDir
	if root == "" {
		dir, err := config.Directory()
		if err != nil {
			return "", err
		}
		root = filepath.Join(dir, "downloaded", ".sources")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", fmt.Errorf("создание кеша исходников: %w", err)
	}
	destination := filepath.Join(root, track.ID)
	if source, err := cachedSource(destination); err != nil || source != "" {
		return source, err
	}
	var source string
	err := p.connectionStrategy().Execute(ctx, func(address string) error {
		dir, err := os.MkdirTemp(root, ".download-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		_, err = p.runConnectionWithProgress(ctx, "download", onProxy, address, onProgress,
			"--no-playlist", "--concurrent-fragments", "5", "--format", "bestaudio", "--fixup", "never",
			"--output", filepath.Join(dir, "audio.%(ext)s"), "--", "https://www.youtube.com/watch?v="+track.ID)
		if err != nil {
			return err
		}
		files, err := filepath.Glob(filepath.Join(dir, "audio.*"))
		if err != nil {
			return err
		}
		var audio string
		for _, path := range files {
			switch filepath.Ext(path) {
			case ".webm", ".m4a", ".mp4", ".ogg", ".opus", ".mp3", ".aac", ".flac", ".wav":
				info, err := os.Stat(path)
				if err != nil {
					return err
				}
				if info.Mode().IsRegular() && info.Size() > 0 {
					audio = path
				}
			}
		}
		if audio == "" {
			return fmt.Errorf("yt-dlp: исходное аудио отсутствует или пустое")
		}
		// A fixed cache filename avoids depending on the downloaded extension.
		if err := os.Rename(audio, filepath.Join(dir, "source")); err != nil {
			return err
		}
		if err := os.Rename(dir, destination); err != nil {
			// Another worker/process may have published the same video already.
			if existing, checkErr := cachedSource(destination); checkErr == nil && existing != "" {
				source = existing
				return nil
			}
			return fmt.Errorf("сохранение исходника: %w", err)
		}
		source = filepath.Join(destination, "source")
		return nil
	})
	return source, err
}

func cachedSource(directory string) (string, error) {
	path := filepath.Join(directory, "source")
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return "", fmt.Errorf("некорректный кеш исходника %q: удалите его для повторного скачивания", path)
	}
	return path, nil
}
