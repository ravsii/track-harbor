package download

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// Only the external provider is replaced; the downloader, display and files are real.
type testProvider struct {
	searchErr   error
	downloadErr error
	body        io.ReadCloser
}

func (p *testProvider) Name() string { return "test" }
func (p *testProvider) Search(string) (Track, error) {
	return Track{ID: "track", Artist: "Artist", Title: "Title"}, p.searchErr
}
func (p *testProvider) Download(Track) (DownloadResponse, error) {
	return DownloadResponse{Body: p.body, ContentLength: 6}, p.downloadErr
}

func TestDownloaderLeavesNoSearchingRows(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider *testProvider
		want     StatusKind
	}{
		{"success", &testProvider{body: io.NopCloser(strings.NewReader("abcdef"))}, StatusDone},
		{"empty search", &testProvider{searchErr: ErrNotFound}, StatusNotFound},
		{"HTTP search failure", &testProvider{searchErr: &HTTPError{Stage: "search", StatusCode: 404}}, StatusFailed},
		{"download failure", &testProvider{downloadErr: errors.New("offline")}, StatusFailed},
		{"download not found is technical", &testProvider{downloadErr: ErrNotFound}, StatusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDisplay()
			fastDownloader(tc.provider, 1, t.TempDir()).WithDisplay(d).Run([]string{"original query"})
			if len(d.active) != 0 {
				t.Errorf("finished job left active rows: %+v", d.activeUnsafe())
			}
			if len(d.done) != 1 || d.done[0].Kind != tc.want {
				t.Errorf("unexpected final events: %+v", d.done)
			}
		})
	}
}

type observedBody struct {
	display *Display
	reads   int
	seen    int64
}

func (b *observedBody) Read(p []byte) (int, error) {
	b.display.mu.Lock()
	for _, ev := range b.display.active {
		if ev.DownloadedBytes > b.seen {
			b.seen = ev.DownloadedBytes
		}
	}
	b.display.mu.Unlock()
	if b.reads == 2 {
		return 0, io.EOF
	}
	b.reads++
	return copy(p, "abc"), nil
}
func (b *observedBody) Close() error { return nil }

func TestDownloaderReportsProgressDuringCopy(t *testing.T) {
	d := NewDisplay()
	body := &observedBody{display: d}
	New(&testProvider{body: body}, 1, t.TempDir()).WithDisplay(d).Run([]string{"query"})
	if body.seen != 6 {
		t.Fatalf("expected live byte progress, got %d", body.seen)
	}
}
