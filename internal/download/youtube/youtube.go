// Package youtube uses yt-dlp and FFmpeg executables available in PATH.
package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"meshvault/internal/download"
)

var videoID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
var httpStatus = regexp.MustCompile(`HTTP Error ([0-9]{3})`)

type ProxySelector interface{ Next() (string, error) }

type Provider struct {
	executable string
	strategy   DownloadStrategy
	sourceDir  string
}

func (p *Provider) WithSourceDirectory(directory string) *Provider {
	p.sourceDir = directory
	return p
}

func (p *Provider) WithProxies(proxies ProxySelector) *Provider {
	return p.WithStrategy(ProxyOnly{Pool: proxies})
}

func (p *Provider) WithStrategy(strategy DownloadStrategy) *Provider {
	p.strategy = strategy
	return p
}

func (p *Provider) connectionStrategy() DownloadStrategy {
	if p.strategy == nil {
		return Direct{}
	}
	return p.strategy
}

func NewProvider() (*Provider, error) {
	path, err := exec.LookPath("yt-dlp")
	if err != nil {
		return nil, fmt.Errorf("YouTube: yt-dlp must be installed and available in PATH: %w", err)
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("YouTube: ffmpeg must be installed and available in PATH: %w", err)
	}
	return &Provider{executable: path}, nil
}

func (p *Provider) Name() string { return "youtube" }

func (p *Provider) run(stage string, timeout time.Duration, args ...string) ([]byte, error) {
	return p.runWithProxy(stage, timeout, nil, args...)
}

func (p *Provider) runWithProxy(stage string, timeout time.Duration, onProxy func(string), args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var output []byte
	err := p.connectionStrategy().Execute(ctx, func(address string) error {
		var err error
		output, err = p.runConnection(ctx, stage, onProxy, address, args...)
		return err
	})
	return output, err
}

func (p *Provider) runConnection(ctx context.Context, stage string, onProxy func(string), proxyAddress string, args ...string) ([]byte, error) {
	return p.runConnectionWithProgress(ctx, stage, onProxy, proxyAddress, nil, args...)
}

func (p *Provider) runConnectionWithProgress(ctx context.Context, stage string, onProxy func(string), proxyAddress string, onProgress func(download.DownloadProgress), args ...string) ([]byte, error) {
	// Never invoke a shell; query strings are a single argument, not commands.
	options := []string{"--ignore-config", "--no-progress", "--no-cache-dir", "--socket-timeout", "15", "--retries", "0", "--fragment-retries", "0", "--extractor-retries", "0", "--file-access-retries", "0"}
	if onProgress != nil {
		options = append(options, "--progress", "--newline", "--progress-delta", "0.5", "--progress-template", progressTemplate)
	}
	// --proxy "" explicitly disables environment proxies for direct requests.
	options = append(options, "--proxy", proxyAddress)
	if onProxy != nil {
		label := "direct"
		if proxyAddress != "" {
			label = download.ProxyLabel(proxyAddress)
		}
		onProxy(label)
	}
	cmd := exec.CommandContext(ctx, p.executable, append(options, args...)...)
	// Explicit routing must not be bypassed by inherited NO_PROXY or a
	// conflicting global HTTP_PROXY; never change the parent environment.
	cmd.Env = make([]string, 0, len(os.Environ()))
	for _, env := range os.Environ() {
		key, _, _ := strings.Cut(env, "=")
		switch strings.ToUpper(key) {
		case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY":
			continue
		}
		cmd.Env = append(cmd.Env, env)
	}
	cmd.WaitDelay = time.Second
	configureCancellation(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	var output []byte
	var err error
	if onProgress == nil {
		output, err = cmd.Output()
	} else {
		writer := &progressWriter{onProgress: onProgress}
		cmd.Stdout = writer
		err = cmd.Run()
		writer.flush()
	}
	rateLimited := false
	if err != nil {
		for _, match := range httpStatus.FindAllStringSubmatch(stderr.String(), -1) {
			if match[1] == "429" {
				rateLimited = true
				break
			}
		}
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("yt-dlp %s: %w", stage, ctx.Err())
	}
	if err != nil {
		if rateLimited {
			return nil, &download.HTTPError{Stage: stage, StatusCode: 429}
		}
		if strings.Contains(strings.ToLower(stderr.String()), "timed out") {
			return nil, fmt.Errorf("yt-dlp %s: %w", stage, context.DeadlineExceeded)
		}
		if match := httpStatus.FindStringSubmatch(stderr.String()); len(match) > 0 {
			code, _ := strconv.Atoi(match[1])
			return nil, &download.HTTPError{Stage: stage, StatusCode: code}
		}
		// Do not persist raw stderr: it can contain signed URLs and cookies.
		return nil, fmt.Errorf("yt-dlp %s: %w (check the yt-dlp version, YouTube access and JS runtime)", stage, err)
	}
	return output, nil
}

func (p *Provider) Search(query string) (download.Track, error) {
	return p.SearchWithProxy(query, nil)
}

func (p *Provider) SearchWithProxy(query string, onProxy func(string)) (download.Track, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var track download.Track
	err := p.connectionStrategy().Execute(ctx, func(address string) error {
		var err error
		track, err = p.searchConnection(ctx, query, onProxy, address)
		return err
	})
	return track, err
}

func (p *Provider) searchConnection(ctx context.Context, query string, onProxy func(string), address string) (download.Track, error) {
	output, err := p.runConnection(ctx, "search", onProxy, address, "--flat-playlist", "--dump-single-json", "--", "ytsearch1:"+query)
	if err != nil {
		return download.Track{}, err
	}
	var result struct {
		Entries []struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			Uploader string `json:"uploader"`
			Channel  string `json:"channel"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return download.Track{}, fmt.Errorf("yt-dlp search JSON: %w", err)
	}
	if result.Entries == nil {
		return download.Track{}, fmt.Errorf("yt-dlp search: missing entries")
	}
	if len(result.Entries) == 0 {
		return download.Track{}, download.ErrNotFound
	}
	entry := result.Entries[0]
	if !videoID.MatchString(entry.ID) || entry.Title == "" {
		return download.Track{}, fmt.Errorf("yt-dlp search: invalid video metadata")
	}
	artist := entry.Uploader
	if artist == "" {
		artist = entry.Channel
	}
	return download.Track{ID: entry.ID, Title: entry.Title, Artist: artist}, nil
}

func (p *Provider) Download(track download.Track) (download.DownloadResponse, error) {
	return p.DownloadWithProxy(track, nil)
}

func (p *Provider) DownloadWithProxy(track download.Track, onProxy func(string)) (download.DownloadResponse, error) {
	return p.DownloadWithProgress(track, onProxy, nil)
}

func (p *Provider) DownloadWithProgress(track download.Track, onProxy func(string), onProgress func(download.DownloadProgress)) (download.DownloadResponse, error) {
	if !videoID.MatchString(track.ID) {
		return download.DownloadResponse{}, fmt.Errorf("YouTube: invalid video ID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	source, err := p.prepareSource(ctx, track, onProxy, onProgress)
	if err != nil {
		return download.DownloadResponse{}, err
	}
	// Conversion is local: errors must not switch routes or download again.
	conversionCtx, conversionCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer conversionCancel()
	return p.convertSource(conversionCtx, source, onProgress)
}

func (p *Provider) convertSource(ctx context.Context, source string, onProgress func(download.DownloadProgress)) (download.DownloadResponse, error) {
	dir, err := os.MkdirTemp("", "meshvault-youtube-*")
	if err != nil {
		return download.DownloadResponse{}, err
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(dir)
		}
	}()
	if onProgress != nil {
		onProgress(download.DownloadProgress{Converting: true})
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-i", source, "-map", "0:a:0", "-vn", "-c:a", "libmp3lame", "-b:a", "192k", filepath.Join(dir, "audio.mp3"))
	cmd.WaitDelay = time.Second
	configureCancellation(cmd)
	err = cmd.Run()
	if err != nil {
		return download.DownloadResponse{}, fmt.Errorf("FFmpeg: MP3 conversion failed (source preserved at %q): %w", source, err)
	}
	f, err := os.Open(filepath.Join(dir, "audio.mp3"))
	if err != nil {
		return download.DownloadResponse{}, fmt.Errorf("FFmpeg MP3 output: %w", err)
	}
	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		f.Close()
		if err == nil {
			err = fmt.Errorf("empty MP3")
		}
		return download.DownloadResponse{}, err
	}
	keep = true
	return download.DownloadResponse{Body: &temporaryAudio{File: f, dir: dir}, ContentLength: info.Size()}, nil
}

// The downloader hashes/copies the MP3, then closes it; remove staged media too.
type temporaryAudio struct {
	*os.File
	dir string
}

func (f *temporaryAudio) Close() error { return errors.Join(f.File.Close(), os.RemoveAll(f.dir)) }
