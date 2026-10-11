package download

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestRateLimitBackoffCapsAtThirtySeconds(t *testing.T) {
	for i, want := range []time.Duration{5 * time.Second, 10 * time.Second, 15 * time.Second, 20 * time.Second, 25 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second} {
		if got := rateLimitDelay(i); got != want {
			t.Fatalf("pause %d: %v != %v", i+1, got, want)
		}
	}
	d := New(&testProvider{}, 10, t.TempDir())
	if d.startInterval != time.Second {
		t.Fatal("default spacing must be one second")
	}
}

func TestGateSpacesConcurrentStarts(t *testing.T) {
	gate := &requestGate{interval: 15 * time.Millisecond, delay: rateLimitDelay}
	starts := make(chan time.Time, 10)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); gate.wait(func(bool) {}); starts <- time.Now() }()
	}
	wg.Wait()
	close(starts)
	var previous time.Time
	for started := range starts {
		if !previous.IsZero() && started.Sub(previous) < 14*time.Millisecond {
			t.Fatal("concurrent requests bypassed shared spacing")
		}
		previous = started
	}
}

func Test429PausesEntireGateAndCountsConcurrentFailuresOnce(t *testing.T) {
	gate := &requestGate{delay: func(round int) time.Duration { return time.Duration(round+1) * 25 * time.Millisecond }}
	var generation uint64
	for i := 0; i < 10; i++ {
		generation = gate.wait(func(bool) {})
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); gate.block(generation) }()
	}
	wg.Wait()
	if gate.round != 1 {
		t.Fatalf("one wave counted %d times", gate.round)
	}
	pausedUntil := gate.blockedUntil
	var seenPause bool
	nextGeneration := gate.wait(func(paused bool) { seenPause = seenPause || paused })
	if time.Now().Before(pausedUntil) || !seenPause {
		t.Fatal("request started during global pause")
	}
	if _, changed := gate.block(generation); changed {
		t.Fatal("late failure from old wave extended pause")
	}
	delay, changed := gate.block(nextGeneration)
	if !changed || delay != 50*time.Millisecond {
		t.Fatalf("next wave failed to increase pause: %v", delay)
	}
}

func Test429RetriesBeyondFiveAttemptsUntilSuccess(t *testing.T) {
	p := &retryProvider{calls: make(map[string]int), failures: 8, err: &HTTPError{Stage: "search", StatusCode: 429}}
	disp := NewDisplay()
	d := fastDownloader(p, 10, t.TempDir()).WithDisplay(disp)
	d.startInterval = 0
	d.rateLimitDelay = func(int) time.Duration { return time.Millisecond }
	d.Run([]string{"bad", "good"})
	if p.calls["bad"] != 9 || len(disp.done) != 2 || len(disp.active) != 0 {
		t.Fatalf("lost rate limited job: calls=%v done=%+v", p.calls, disp.done)
	}
	for _, ev := range disp.done {
		if ev.Kind != StatusDone {
			t.Fatalf("429 became final failure: %+v", ev)
		}
	}
}

type pacedProvider struct {
	mu                sync.Mutex
	starts            []time.Time
	searchStarts      []time.Time
	downloadStarts    []time.Time
	active, maxActive int
	failFirst         bool
	failDownload      bool
	limitedAt         time.Time
	searchHook        func(int)
}

func (p *pacedProvider) Name() string { return "paced" }
func (p *pacedProvider) record() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.starts = append(p.starts, time.Now())
	return len(p.starts)
}
func (p *pacedProvider) Search(query string) (Track, error) {
	call := p.record()
	p.mu.Lock()
	p.searchStarts = append(p.searchStarts, time.Now())
	p.mu.Unlock()
	if p.searchHook != nil {
		p.searchHook(call)
	}
	if p.failFirst && call == 1 {
		p.mu.Lock()
		p.limitedAt = time.Now()
		p.mu.Unlock()
		return Track{}, &HTTPError{Stage: "search", StatusCode: 429}
	}
	return Track{ID: query, Title: query}, nil
}
func (p *pacedProvider) Download(Track) (DownloadResponse, error) {
	p.record()
	p.mu.Lock()
	p.downloadStarts = append(p.downloadStarts, time.Now())
	if p.failDownload {
		p.failDownload = false
		p.limitedAt = time.Now()
		p.mu.Unlock()
		return DownloadResponse{}, &HTTPError{Stage: "download", StatusCode: 429}
	}
	p.mu.Unlock()
	p.mu.Lock()
	p.active++
	p.maxActive = max(p.maxActive, p.active)
	p.mu.Unlock()
	time.Sleep(30 * time.Millisecond)
	p.mu.Lock()
	p.active--
	p.mu.Unlock()
	return DownloadResponse{Body: io.NopCloser(strings.NewReader("audio")), ContentLength: 5}, nil
}

func TestDownloaderPacesOnlyAudioDownloads(t *testing.T) {
	p := &pacedProvider{}
	d := fastDownloader(p, 10, t.TempDir())
	d.startInterval = 5 * time.Millisecond
	var queries []string
	for i := 0; i < 12; i++ {
		queries = append(queries, fmt.Sprint(i))
	}
	d.Run(queries)
	if len(p.starts) != 24 {
		t.Fatalf("missing requests: %d", len(p.starts))
	}
	for i := 1; i < len(p.downloadStarts); i++ {
		if p.downloadStarts[i].Sub(p.downloadStarts[i-1]) < 4*time.Millisecond {
			t.Fatal("audio start bypassed pacing")
		}
	}
	if p.maxActive < 2 || p.maxActive > 10 {
		t.Fatalf("parallelism broken: %d", p.maxActive)
	}
}

func TestQueuedWorkDoesNotStartDuring429Cooldown(t *testing.T) {
	p := &pacedProvider{failFirst: true}
	d := fastDownloader(p, 1, t.TempDir())
	d.startInterval = 5 * time.Millisecond
	d.rateLimitDelay = func(int) time.Duration { return 40 * time.Millisecond }
	d.Run([]string{"first", "second", "third"})
	if len(p.starts) != 7 || p.starts[1].Sub(p.starts[0]) < 40*time.Millisecond {
		t.Fatalf("queued requests bypassed cooldown: %v", p.starts)
	}
}

func TestAudio429PausesDownloadsButNotSearches(t *testing.T) {
	p := &pacedProvider{failDownload: true}
	d := fastDownloader(p, 1, t.TempDir())
	d.startInterval = 5 * time.Millisecond
	d.rateLimitDelay = func(int) time.Duration { return 40 * time.Millisecond }
	d.Run([]string{"first", "second", "third"})
	var afterLimit []time.Time
	for _, start := range p.downloadStarts {
		if start.After(p.limitedAt) {
			afterLimit = append(afterLimit, start)
		}
	}
	if len(afterLimit) == 0 || afterLimit[0].Sub(p.limitedAt) < 40*time.Millisecond {
		t.Fatalf("audio 429 did not stop downloads: %v", p.downloadStarts)
	}
	if p.searchStarts[1].Sub(p.limitedAt) >= 40*time.Millisecond {
		t.Fatalf("audio 429 blocked search: %v", p.searchStarts)
	}
}

func TestSearchesStartInParallelWithoutOneSecondSpacing(t *testing.T) {
	allSearching := make(chan struct{})
	var searchMu sync.Mutex
	entered := 0
	p := &pacedProvider{searchHook: func(int) {
		searchMu.Lock()
		entered++
		if entered == 10 {
			close(allSearching)
		}
		searchMu.Unlock()
		select {
		case <-allSearching:
		case <-time.After(time.Second):
		}
	}}
	d := fastDownloader(p, 10, t.TempDir())
	d.startInterval = 40 * time.Millisecond
	var queries []string
	for i := 0; i < 10; i++ {
		queries = append(queries, fmt.Sprint(i))
	}
	d.Run(queries)
	if len(p.searchStarts) != 10 || p.searchStarts[9].Sub(p.searchStarts[0]) >= 200*time.Millisecond {
		t.Fatalf("search was paced like downloads: %v", p.searchStarts)
	}
}

func TestQueuedStatusDoesNotCountAsProcessing(t *testing.T) {
	output := captureDisplayOutput(t, func() {
		d := NewDisplay()
		d.SetTotal(2)
		d.Update(StatusEvent{JobID: "1", Kind: StatusQueued, Name: "Queued"})
		d.Update(StatusEvent{JobID: "2", Kind: StatusSearching, Name: "Active"})
		d.Stop()
	})
	for _, want := range []string{"Queued: 1", "Active: 1", "Errors: 0"} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %s in %s", want, output)
		}
	}
	if strings.Contains(output, "Waiting to retry") {
		t.Fatal("start spacing mistaken for retry")
	}
}

type blockedSearchLog struct {
	blocked, release, paused chan struct{}
	once                     sync.Once
}

func (w *blockedSearchLog) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte(`"msg":"searching"`)) && bytes.Contains(p, []byte(`"query":"second"`)) {
		w.once.Do(func() { close(w.blocked); <-w.release })
	}
	if bytes.Contains(p, []byte(`"msg":"rate_limit_pause"`)) {
		close(w.paused)
	}
	return len(p), nil
}
func (w *blockedSearchLog) Sync() error { return nil }

func TestSlowLoggerCannotStartRequestDuringCooldown(t *testing.T) {
	sink := &blockedSearchLog{blocked: make(chan struct{}), release: make(chan struct{}), paused: make(chan struct{})}
	p := &pacedProvider{failFirst: true, searchHook: func(call int) {
		if call == 1 {
			<-sink.blocked
		}
	}}
	d := fastDownloader(p, 2, t.TempDir())
	d.startInterval = 5 * time.Millisecond
	d.rateLimitDelay = func(int) time.Duration { return 40 * time.Millisecond }
	d.WithLogger(zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), sink, zap.InfoLevel)))
	go func() { <-sink.paused; close(sink.release) }()
	d.Run([]string{"first", "second"})
	if len(p.starts) != 5 || p.starts[1].Sub(p.limitedAt) < 40*time.Millisecond {
		t.Fatalf("logging delay bypassed cooldown: %v", p.starts)
	}
}
