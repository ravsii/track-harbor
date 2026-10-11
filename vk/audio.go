package vk

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DownloadMP3 converts a VK HTTPS audio stream to a temporary 192k MP3.
// Closing the returned file removes the temporary directory. Signed URLs and
// FFmpeg stderr are intentionally omitted from errors.
func DownloadMP3(source string) (*os.File, int64, func() error, error) {
	if !strings.HasPrefix(source, "https://") {
		return nil, 0, nil, fmt.Errorf("VK audio URL unavailable; run vk sync")
	}
	dir, err := os.MkdirTemp("", "trackharbor-vk-*")
	if err != nil {
		return nil, 0, nil, err
	}
	path := filepath.Join(dir, "audio.mp3")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-rw_timeout", "15000000", "-i", source, "-map", "0:a:0", "-vn", "-c:a", "libmp3lame", "-b:a", "192k", path)
	cmd.Env = withoutProxies(os.Environ())
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		os.RemoveAll(dir)
		if errors.Is(err, exec.ErrNotFound) {
			return nil, 0, nil, fmt.Errorf("FFmpeg not installed: add ffmpeg to PATH")
		}
		if ctx.Err() != nil {
			return nil, 0, nil, fmt.Errorf("VK FFmpeg download timed out")
		}
		return nil, 0, nil, fmt.Errorf("VK FFmpeg download failed; run vk sync to refresh expired audio URLs")
	}
	f, err := os.Open(path)
	if err != nil {
		os.RemoveAll(dir)
		return nil, 0, nil, fmt.Errorf("open converted VK MP3: %w", err)
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		f.Close()
		os.RemoveAll(dir)
		return nil, 0, nil, fmt.Errorf("VK FFmpeg output is empty or invalid")
	}
	return f, info.Size(), func() error { return errors.Join(f.Close(), os.RemoveAll(dir)) }, nil
}

func withoutProxies(env []string) []string {
	clean := make([]string, 0, len(env))
	for _, pair := range env {
		name, _, _ := strings.Cut(pair, "=")
		switch strings.ToLower(name) {
		case "http_proxy", "https_proxy", "all_proxy", "no_proxy":
			continue
		}
		clean = append(clean, pair)
	}
	return clean
}
