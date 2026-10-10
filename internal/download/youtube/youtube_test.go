package youtube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"meshvault/internal/download"
)

func fakeTools(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{"yt-dlp": script, "ffmpeg": "exit 0"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	return dir
}

func TestSearchViaYtDlp(t *testing.T) {
	dir := fakeTools(t, `printf '%s\n' "$@" > "$ARGS_FILE"
printf '%s' '{"entries":[{"id":"abcdefghijk","title":"Song title","uploader":"Artist"}]}'`)
	argsFile := filepath.Join(dir, "args")
	t.Setenv("ARGS_FILE", argsFile)
	p, err := NewProvider()
	if err != nil {
		t.Fatal(err)
	}
	track, err := p.Search(`Artist song; $(anything)`)
	if err != nil || track.ID != "abcdefghijk" || track.Title != "Song title" || track.Artist != "Artist" {
		t.Fatalf("track=%+v err=%v", track, err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--ignore-config", "--flat-playlist", "--dump-single-json", "--extractor-retries\n0", "--socket-timeout\n15", "ytsearch1:Artist song; $(anything)"} {
		if !strings.Contains(string(args), want) {
			t.Fatalf("missing %s in %s", want, args)
		}
	}
}

func TestEmptySearchIsNotTechnicalFailure(t *testing.T) {
	for _, tc := range []struct {
		script   string
		notFound bool
	}{
		{`printf '%s' '{"entries":[]}'`, true},
		{`printf '%s' 'invalid JSON'`, false},
		{`printf '%s' '{"entries":[{"id":"../../bad","title":"Song"}]}'`, false},
		{`printf '%s' 'ERROR: HTTP Error 500: server error' >&2; exit 1`, false},
	} {
		t.Run(tc.script, func(t *testing.T) {
			fakeTools(t, tc.script)
			p, err := NewProvider()
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.Search("Song")
			if err == nil || errors.Is(err, download.ErrNotFound) != tc.notFound {
				t.Fatalf("wrong classification: %v", err)
			}
			if strings.Contains(tc.script, "HTTP Error") {
				var httpErr *download.HTTPError
				if !errors.As(err, &httpErr) || httpErr.StatusCode != 500 {
					t.Fatalf("HTTP status lost: %v", err)
				}
			}
		})
	}
}

func TestDownloadConvertsMP3AndCleansTemporaryFiles(t *testing.T) {
	dir := fakeTools(t, `printf '%s\n' "$@" > "$ARGS_FILE"
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output" ]; then shift; output="$1"; fi
  shift
done
printf '%s' 'mp3 bytes' > "${output%.*}.mp3"`)
	argsFile := filepath.Join(dir, "args")
	t.Setenv("ARGS_FILE", argsFile)
	p, err := NewProvider()
	if err != nil {
		t.Fatal(err)
	}
	response, err := p.Download(download.Track{ID: "abcdefghijk"})
	if err != nil {
		t.Fatal(err)
	}
	file := response.Body.(*temporaryAudio).File.Name()
	data, err := io.ReadAll(response.Body)
	if err != nil || string(data) != "mp3 bytes" || response.ContentLength != 9 {
		t.Fatalf("body=%s size=%d err=%v", data, response.ContentLength, err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(file)); !os.IsNotExist(err) {
		t.Fatalf("temporary directory survived Close: %v", err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--extract-audio", "--audio-format\nmp3", "--audio-quality\n192K", "--no-playlist", "https://www.youtube.com/watch?v=abcdefghijk"} {
		if !strings.Contains(string(args), want) {
			t.Fatalf("missing %s in %s", want, args)
		}
	}
}

func TestMissingToolsFailBeforeDownloading(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := NewProvider(); err == nil || !strings.Contains(err.Error(), "yt-dlp") {
		t.Fatalf("missing yt-dlp: %v", err)
	}
	dir := fakeTools(t, "exit 0")
	if err := os.Remove(filepath.Join(dir, "ffmpeg")); err != nil {
		t.Fatal(err)
	}
	if _, err := NewProvider(); err == nil || !strings.Contains(err.Error(), "ffmpeg") {
		t.Fatalf("missing ffmpeg: %v", err)
	}
}

func TestDownloadFailureRemovesStagingDirectory(t *testing.T) {
	for _, script := range []string{"exit 1", "exit 0", `while [ "$#" -gt 0 ]; do
if [ "$1" = "--output" ]; then shift; output="$1"; fi
shift
done
: > "${output%.*}.mp3"`} {
		t.Run(script, func(t *testing.T) {
			fakeTools(t, script)
			staging := t.TempDir()
			t.Setenv("TMPDIR", staging)
			p, err := NewProvider()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := p.Download(download.Track{ID: "dQw4w9WgXcQ"}); err == nil {
				t.Fatal("failed or missing/empty MP3 accepted")
			}
			files, err := os.ReadDir(staging)
			if err != nil || len(files) != 0 {
				t.Fatalf("staging directory leaked: %v %v", files, err)
			}
		})
	}
}

func TestNetworkTimeoutIsTechnicalAndRedacted(t *testing.T) {
	fakeTools(t, `printf '%s' 'ERROR: https://host/?token=secret Read timed out' >&2; exit 1`)
	p, err := NewProvider()
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Search("Song")
	if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, download.ErrNotFound) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("timeout not identified safely: %v", err)
	}
}

func TestDeadlineStopsChildProcesses(t *testing.T) {
	dir := fakeTools(t, `printf ready > "$READY"
(/bin/sleep 2; printf survived > "$MARKER") &
wait`)
	marker := filepath.Join(dir, "survived")
	t.Setenv("MARKER", marker)
	t.Setenv("READY", filepath.Join(dir, "ready"))
	p, err := NewProvider()
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	_, err = p.run("test", time.Second)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline lost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ready")); err != nil {
		t.Fatal("child process was never started")
	}
	if time.Since(started) > 1700*time.Millisecond {
		t.Fatal("child output pipes kept process alive past deadline")
	}
	time.Sleep(1500 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("descendant survived cancellation")
	}
}

func TestProcessExitCodeIsPreserved(t *testing.T) {
	fakeTools(t, "exit 7")
	p, err := NewProvider()
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Search("Song")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("exit code lost: %v", err)
	}
}

type sequenceProxies struct {
	calls atomic.Int64
	err   error
}

func (p *sequenceProxies) Next() (string, error) {
	if p.err != nil {
		return "", p.err
	}
	return fmt.Sprintf("http://203.0.113.%d:8080", p.calls.Add(1)), nil
}

func TestEverySearchAndDownloadSelectsANewProxy(t *testing.T) {
	dir := fakeTools(t, `if [ -n "$HTTP_PROXY$HTTPS_PROXY$ALL_PROXY$NO_PROXY$http_proxy$https_proxy$all_proxy$no_proxy" ]; then exit 9; fi
printf '%s\n' "$@" >> "$ARGS_FILE"
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output" ]; then shift; output="$1"; fi
  shift
done
if [ -n "$output" ]; then
  printf '%s' 'mp3 bytes' > "${output%.*}.mp3"
else
  printf '%s' '{"entries":[{"id":"dQw4w9WgXcQ","title":"Song","uploader":"Artist"}]}'
fi`)
	argsFile := filepath.Join(dir, "args")
	t.Setenv("ARGS_FILE", argsFile)
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy"} {
		t.Setenv(key, "inherited-proxy")
	}
	p, err := NewProvider()
	if err != nil {
		t.Fatal(err)
	}
	pool := &sequenceProxies{}
	p.WithProxies(pool)
	track, err := p.Search("Song")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Search("Song retry"); err != nil {
		t.Fatal(err)
	}
	response, err := p.Download(track)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if pool.calls.Load() != 3 {
		t.Fatal("not every invocation selected a proxy")
	}
	for i := 1; i <= 3; i++ {
		if !strings.Contains(string(args), fmt.Sprintf("--proxy\nhttp://203.0.113.%d:8080", i)) {
			t.Fatalf("missing rotated proxy in %s", args)
		}
	}
}

func TestProxySelectionFailureNeverRunsDirectly(t *testing.T) {
	dir := fakeTools(t, `printf ran > "$ARGS_FILE"`)
	argsFile := filepath.Join(dir, "args")
	t.Setenv("ARGS_FILE", argsFile)
	p, err := NewProvider()
	if err != nil {
		t.Fatal(err)
	}
	p.WithProxies(&sequenceProxies{err: errors.New("pool unavailable")})
	if _, err := p.Search("Song"); err == nil {
		t.Fatal("unavailable pool accepted")
	}
	if _, err := os.Stat(argsFile); !os.IsNotExist(err) {
		t.Fatal("yt-dlp ran without a selected proxy")
	}
}

type rateLimitedProxies struct {
	sequenceProxies
	reported         string
	selectionContext bool
}

func (p *rateLimitedProxies) NextContext(ctx context.Context) (string, error) {
	if _, ok := ctx.Deadline(); !ok {
		return "", errors.New("proxy wait has no timeout")
	}
	p.selectionContext = true
	return p.Next()
}
func (p *rateLimitedProxies) ReportRateLimit(address string) { p.reported = address }

func TestYouTubeReports429ForTheSelectedProxy(t *testing.T) {
	for _, status := range []int{429, 403} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			fakeTools(t, fmt.Sprintf("echo 'ERROR: HTTP Error %d' >&2; exit 1", status))
			p, err := NewProvider()
			if err != nil {
				t.Fatal(err)
			}
			pool := &rateLimitedProxies{}
			p.WithProxies(pool)
			_, err = p.Search("Song")
			var httpErr *download.HTTPError
			if !errors.As(err, &httpErr) || httpErr.StatusCode != status {
				t.Fatalf("typed error lost: %v", err)
			}
			if !pool.selectionContext {
				t.Fatal("proxy wait ignores operation deadline")
			}
			want := ""
			if status == 429 {
				want = "http://203.0.113.1:8080"
			}
			if pool.reported != want {
				t.Fatalf("reported %s, want %s", pool.reported, want)
			}
		})
	}
}

func TestYouTubeReports429EvenAlongsideOtherSubprocessErrors(t *testing.T) {
	for _, stderr := range []string{"HTTP Error 429; connection timed out", "HTTP Error 403; HTTP Error 429"} {
		t.Run(stderr, func(t *testing.T) {
			fakeTools(t, "echo '"+stderr+"' >&2; exit 1")
			p, err := NewProvider()
			if err != nil {
				t.Fatal(err)
			}
			pool := &rateLimitedProxies{}
			p.WithProxies(pool)
			_, err = p.Search("Song")
			var httpErr *download.HTTPError
			if !errors.As(err, &httpErr) || httpErr.StatusCode != 429 {
				t.Fatalf("429 masked by other stderr: %v", err)
			}
			if pool.reported != "http://203.0.113.1:8080" {
				t.Fatal("mixed error did not pause selected proxy")
			}
		})
	}
}

func TestYouTubeObservesActualProxyBeforeStartingEachStage(t *testing.T) {
	dir := fakeTools(t, `if [ ! -f "$OBSERVER_MARKER" ]; then exit 9; fi
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output" ]; then shift; output="$1"; fi
  shift
done
if [ -n "$output" ]; then
  printf 'mp3' > "${output%.*}.mp3"
else
  echo '{"entries":[{"id":"dQw4w9WgXcQ","title":"Song","uploader":"Artist"}]}'
fi`)
	marker := filepath.Join(dir, "observer")
	t.Setenv("OBSERVER_MARKER", marker)
	p, err := NewProvider()
	if err != nil {
		t.Fatal(err)
	}
	p.WithProxies(&sequenceProxies{})
	var selected []string
	onProxy := func(address string) {
		selected = append(selected, address)
		if err := os.WriteFile(marker, []byte(address), 0600); err != nil {
			t.Error(err)
		}
	}
	track, err := p.SearchWithProxy("Song", onProxy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	response, err := p.DownloadWithProxy(track, onProxy)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if fmt.Sprint(selected) != "[http://203.0.113.1:8080 http://203.0.113.2:8080]" {
		t.Fatalf("wrong request proxies: %v", selected)
	}
}
