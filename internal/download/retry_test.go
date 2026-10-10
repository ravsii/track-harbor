package download

import (
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func fastDownloader(p Provider, workers int, dir string) *Downloader {
	d := New(p, workers, dir)
	d.retryDelay = func(int) time.Duration { return time.Millisecond }
	d.startInterval = 0
	d.rateLimitDelay = func(int) time.Duration { return time.Millisecond }
	return d
}

type retryProvider struct {
	mu       sync.Mutex
	calls    map[string]int
	order    []string
	failures int
	err      error
}

func (p *retryProvider) Name() string { return "retry" }
func (p *retryProvider) Search(query string) (Track, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls[query]++
	p.order = append(p.order, query)
	if query == "bad" && p.calls[query] <= p.failures {
		return Track{}, p.err
	}
	return Track{ID: query, Title: query}, nil
}
func (p *retryProvider) Download(Track) (DownloadResponse, error) {
	return DownloadResponse{Body: io.NopCloser(strings.NewReader("audio")), ContentLength: 5}, nil
}

func TestRetryBackoff(t *testing.T) {
	for i, want := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second} {
		if got := retryDelay(i); got != want {
			t.Fatalf("retry %d: %v != %v", i+1, got, want)
		}
	}
}

func TestTechnicalRetriesReenterQueueWithoutBlockingWorker(t *testing.T) {
	p := &retryProvider{calls: make(map[string]int), failures: 2, err: errors.New("offline")}
	disp := NewDisplay()
	d := fastDownloader(p, 1, t.TempDir()).WithDisplay(disp)
	d.retryDelay = func(int) time.Duration { return 20 * time.Millisecond }
	d.Run([]string{"bad", "good"})
	if p.calls["bad"] != 3 || p.calls["good"] != 1 || p.order[1] != "good" {
		t.Fatalf("retry blocked queue or run finished early: %v", p.order)
	}
	if len(disp.done) != 2 || len(disp.active) != 0 {
		t.Fatalf("attempts counted as jobs: %+v", disp.done)
	}
	for _, ev := range disp.done {
		if ev.Kind != StatusDone {
			t.Fatalf("temporary failure became final: %+v", ev)
		}
	}
}

func TestRetryLimitAndNotFound(t *testing.T) {
	for _, tc := range []struct {
		err   error
		calls int
		kind  StatusKind
	}{
		{&HTTPError{Stage: "search", StatusCode: 404}, 6, StatusFailed},
		{&HTTPError{Stage: "search", StatusCode: 500}, 6, StatusFailed},
		{ErrNotFound, 1, StatusNotFound},
	} {
		p := &retryProvider{calls: make(map[string]int), failures: 100, err: tc.err}
		disp := NewDisplay()
		fastDownloader(p, 10, t.TempDir()).WithDisplay(disp).Run([]string{"bad"})
		if p.calls["bad"] != tc.calls || len(disp.done) != 1 || disp.done[0].Kind != tc.kind {
			t.Fatalf("unexpected retry outcome: calls=%v done=%+v", p.calls, disp.done)
		}
	}
}

func TestWaitingRetryIsQueuedNotProcessingOrFailed(t *testing.T) {
	output := captureDisplayOutput(t, func() {
		d := NewDisplay()
		d.SetTotal(2)
		d.Update(StatusEvent{JobID: "1", Kind: StatusRetryWaiting, Name: "Waiting track", Err: errors.New("offline")})
		d.Update(StatusEvent{JobID: "2", Kind: StatusSearching, Name: "Active track"})
		d.Stop()
	})
	for _, want := range []string{"В очереди: 1", "В работе: 1", "Ошибок: 0", "Ожидают повтора: 1", "Active track"} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q in %s", want, output)
		}
	}
	if strings.Contains(output, "Waiting track") {
		t.Fatal("waiting job displayed as active")
	}
}
