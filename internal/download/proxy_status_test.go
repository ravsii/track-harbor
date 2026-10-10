package download

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestDisplayShowsProxyForSearchAndDownloadWithoutSecrets(t *testing.T) {
	var output bytes.Buffer
	d := NewDisplay()
	d.out = &output
	d.interactive = false
	d.size = func() (int, int) { return 200, 24 }
	for _, kind := range []StatusKind{StatusSearching, StatusDownloading} {
		d.Update(StatusEvent{JobID: "1", Source: "youtube", Kind: kind, Name: "Artist — Song", Proxy: "socks5://user:SECRET@203.0.113.1:1080/?token=SECRET"})
		d.render()
	}
	for _, want := range []string{"Ищется", "Скачивается", "прокси: socks5://203.0.113.1:1080"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("missing %q: %s", want, output.String())
		}
	}
	if strings.Contains(output.String(), "SECRET") || strings.Contains(output.String(), "user:") {
		t.Fatal("proxy secrets leaked")
	}
}

type proxyStatusProvider struct {
	t       *testing.T
	display *Display
}

func (p *proxyStatusProvider) Name() string { return "youtube" }
func (p *proxyStatusProvider) Search(string) (Track, error) {
	return Track{}, fmt.Errorf("observer bypassed")
}
func (p *proxyStatusProvider) Download(Track) (DownloadResponse, error) {
	return DownloadResponse{}, fmt.Errorf("observer bypassed")
}
func (p *proxyStatusProvider) checkActive(query, proxy string, kind StatusKind) {
	p.display.mu.Lock()
	defer p.display.mu.Unlock()
	found := false
	for _, evt := range p.display.active {
		if evt.Name == query || evt.Name == "Artist — "+query {
			found = true
			if evt.Proxy != proxy || evt.Kind != kind {
				p.t.Errorf("wrong active request: %+v, proxy=%s kind=%d", evt, proxy, kind)
			}
		}
	}
	if !found {
		p.t.Error("request did not show selected proxy before running")
	}
}
func (p *proxyStatusProvider) SearchWithProxy(query string, onProxy func(string)) (Track, error) {
	n, _ := strconv.Atoi(strings.TrimPrefix(query, "Song"))
	address := fmt.Sprintf("http://203.0.113.%d:8080", n+1)
	raw := fmt.Sprintf("http://user:SECRET@203.0.113.%d:8080", n+1)
	onProxy(raw)
	p.checkActive(query, address, StatusSearching)
	return Track{ID: query, Artist: "Artist", Title: query}, nil
}
func (p *proxyStatusProvider) DownloadWithProxy(track Track, onProxy func(string)) (DownloadResponse, error) {
	n, _ := strconv.Atoi(strings.TrimPrefix(track.Title, "Song"))
	onProxy(fmt.Sprintf("socks5://user:SECRET@203.0.113.%d:1080", n+1))
	p.checkActive(track.Title, fmt.Sprintf("socks5://203.0.113.%d:1080", n+1), StatusDownloading)
	return DownloadResponse{Body: io.NopCloser(strings.NewReader("mp3")), ContentLength: 3}, nil
}

func TestDownloaderAssociatesProxyWithEachJobStageAndProgress(t *testing.T) {
	display := NewDisplay()
	display.out = io.Discard
	display.interactive = false
	provider := &proxyStatusProvider{t: t, display: display}
	core, logs := observer.New(zap.InfoLevel)
	d := New(provider, 10, t.TempDir()).WithDisplay(display).WithLogger(zap.New(core))
	d.startInterval = 0
	var queries []string
	for i := 0; i < 20; i++ {
		queries = append(queries, fmt.Sprintf("Song%d", i))
	}
	d.Run(queries)
	requests := logs.FilterMessage("request_started").All()
	if len(requests) != 40 {
		t.Fatalf("got %d request records, want 40", len(requests))
	}
	for _, entry := range requests {
		fields := entry.ContextMap()
		n, _ := strconv.Atoi(strings.TrimPrefix(fields["query"].(string), "Song"))
		want := fmt.Sprintf("http://203.0.113.%d:8080", n+1)
		if fields["stage"] == "download" {
			want = fmt.Sprintf("socks5://203.0.113.%d:1080", n+1)
		}
		if fields["proxy"] != want || fields["job_id"] == nil || fields["attempt"] != int64(1) {
			t.Fatalf("wrong request fields: %v", fields)
		}
	}
	for _, entry := range logs.FilterMessage("downloading").All() {
		fields := entry.ContextMap()
		n, _ := strconv.Atoi(strings.TrimPrefix(fields["query"].(string), "Song"))
		if fields["proxy"] != fmt.Sprintf("socks5://203.0.113.%d:1080", n+1) {
			t.Fatal("download progress logged search proxy")
		}
	}
	for _, entry := range logs.All() {
		if strings.Contains(fmt.Sprint(entry.ContextMap()), "SECRET") {
			t.Fatal("proxy credentials leaked into logs")
		}
	}
}

type retryProxyStatusProvider struct {
	proxyStatusProvider
	calls int
}

func (p *retryProxyStatusProvider) SearchWithProxy(query string, onProxy func(string)) (Track, error) {
	p.calls++
	if p.calls == 1 {
		onProxy("http://203.0.113.99:8080")
		return Track{}, &HTTPError{Stage: "search", StatusCode: 429}
	}
	return p.proxyStatusProvider.SearchWithProxy(query, onProxy)
}

func TestRetryShowsNewProxyAndLogsFailedConnection(t *testing.T) {
	display := NewDisplay()
	display.out = io.Discard
	display.interactive = false
	provider := &retryProxyStatusProvider{proxyStatusProvider: proxyStatusProvider{t: t, display: display}}
	core, logs := observer.New(zap.InfoLevel)
	d := New(provider, 1, t.TempDir()).WithDisplay(display).WithLogger(zap.New(core))
	d.startInterval = 0
	d.rateLimitDelay = func(int) time.Duration { return 0 }
	d.Run([]string{"Song0"})
	requests := logs.FilterMessage("request_started").All()
	if len(requests) != 3 {
		t.Fatalf("requests=%d, want retry plus download", len(requests))
	}
	if requests[0].ContextMap()["proxy"] != "http://203.0.113.99:8080" || requests[1].ContextMap()["proxy"] != "http://203.0.113.1:8080" || requests[1].ContextMap()["attempt"] != int64(2) {
		t.Fatal("retry retained old connection proxy")
	}
	failures := logs.FilterMessage("retry_scheduled").All()
	if len(failures) != 1 || failures[0].ContextMap()["proxy"] != "http://203.0.113.99:8080" {
		t.Fatal("429 log lost the failing proxy")
	}
}
