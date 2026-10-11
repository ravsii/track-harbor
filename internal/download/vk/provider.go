package vk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"trackharbor/internal/download"
	"trackharbor/internal/repository"
)

// Provider uses the URLs returned by VK sync; it never searches YouTube.
type Provider struct {
	songs map[string]repository.VKSong
	byID  map[string]string
}

func NewProvider(songs map[string]repository.VKSong) *Provider {
	byID := make(map[string]string, len(songs))
	for _, song := range songs {
		byID[strconv.FormatInt(song.OwnerID, 10)+"_"+strconv.FormatInt(song.AudioID, 10)] = song.URL
	}
	return &Provider{songs: songs, byID: byID}
}
func (*Provider) Name() string { return "vk" }

func (p *Provider) Search(query string) (download.Track, error) {
	song, ok := p.songs[query]
	if !ok {
		return download.Track{}, download.ErrNotFound
	}
	if song.OwnerID == 0 || song.AudioID <= 0 || !strings.HasPrefix(song.URL, "https://") {
		return download.Track{}, fmt.Errorf("VK audio URL unavailable; run trackharbor vk sync to refresh the library")
	}
	var artists []string
	if err := json.Unmarshal([]byte(song.Artists), &artists); err != nil {
		return download.Track{}, fmt.Errorf("invalid VK artist list")
	}
	return download.Track{ID: strconv.FormatInt(song.OwnerID, 10) + "_" + strconv.FormatInt(song.AudioID, 10), Artist: strings.Join(artists, ", "), Title: song.Title}, nil
}

func (p *Provider) Download(track download.Track) (download.DownloadResponse, error) {
	return p.DownloadWithProgress(track, nil, nil)
}

func (p *Provider) DownloadWithProgress(track download.Track, onProxy func(string), onProgress func(download.DownloadProgress)) (download.DownloadResponse, error) {
	source := p.byID[track.ID]
	if !strings.HasPrefix(source, "https://") {
		return download.DownloadResponse{}, fmt.Errorf("VK audio URL unavailable; run trackharbor vk sync")
	}
	if onProgress != nil {
		onProgress(download.DownloadProgress{Converting: true})
	}
	dir, err := os.MkdirTemp("", "trackharbor-vk-*")
	if err != nil {
		return download.DownloadResponse{}, err
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
			return download.DownloadResponse{}, fmt.Errorf("FFmpeg not installed: add ffmpeg to PATH")
		}
		if ctx.Err() != nil {
			return download.DownloadResponse{}, fmt.Errorf("VK FFmpeg download timed out")
		}
		// FFmpeg's stderr and exec errors can embed signed audio URLs.
		return download.DownloadResponse{}, fmt.Errorf("VK FFmpeg download failed; run vk sync to refresh expired audio URLs")
	}
	f, err := os.Open(path)
	if err != nil {
		os.RemoveAll(dir)
		return download.DownloadResponse{}, fmt.Errorf("open converted VK MP3: %w", err)
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		f.Close()
		os.RemoveAll(dir)
		return download.DownloadResponse{}, fmt.Errorf("VK FFmpeg output is empty or invalid")
	}
	return download.DownloadResponse{Body: &temporaryFile{File: f, dir: dir}, ContentLength: info.Size()}, nil
}

type temporaryFile struct {
	*os.File
	dir string
}

func (f *temporaryFile) Close() error { return errors.Join(f.File.Close(), os.RemoveAll(f.dir)) }

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
