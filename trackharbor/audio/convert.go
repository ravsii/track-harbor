package audio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ConvertFileMP3 converts a locally downloaded audio file to 192k MP3.
// The source is removed only after a nonempty MP3 has been verified; on
// conversion failure it is retained for a later retry.
func ConvertFileMP3(ctx context.Context, source string) (*os.File, int64, func() error, error) {
	dir, err := os.MkdirTemp("", "trackharbor-youtube-*")
	if err != nil {
		return nil, 0, nil, err
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(dir)
		}
	}()
	path := filepath.Join(dir, "audio.mp3")
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-i", source, "-map", "0:a:0", "-vn", "-c:a", "libmp3lame", "-b:a", "192k", path)
	cmd.WaitDelay = time.Second
	configureCancellation(cmd)
	if err := cmd.Run(); err != nil {
		return nil, 0, nil, fmt.Errorf("FFmpeg: MP3 conversion failed (source preserved at %q): %w", source, err)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("FFmpeg MP3 output: %w", err)
	}
	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		f.Close()
		if err == nil {
			err = fmt.Errorf("empty MP3")
		}
		return nil, 0, nil, err
	}
	if err := os.Remove(source); err != nil && !errors.Is(err, os.ErrNotExist) {
		f.Close()
		return nil, 0, nil, fmt.Errorf("remove source after MP3 conversion: %w", err)
	}
	_ = os.Remove(filepath.Dir(source))
	keep = true
	return f, info.Size(), func() error { return errors.Join(f.Close(), os.RemoveAll(dir)) }, nil
}
