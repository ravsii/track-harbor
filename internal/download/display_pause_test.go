package download

import (
	"strings"
	"testing"
	"time"
)

func TestDisplayExplains429WhenNoJobsAreProcessing(t *testing.T) {
	output := captureDisplayOutput(t, func() {
		d := NewDisplay()
		now := time.Now()
		d.now = func() time.Time { return now }
		d.SetTotal(1050)
		d.SetRateLimitPause("download", now.Add(17*time.Second))
		d.Update(StatusEvent{JobID: "1", Kind: StatusRetryWaiting, Name: "Waiting song"})
		d.Stop()
	})
	for _, want := range []string{"В работе: 0", "HTTP 429: скачивание приостановлено", "Повтор через: 17 с", "автоматически"} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q in %s", want, output)
		}
	}
}

func TestDisplayCountdownUpdatesAndExpiredPauseDisappears(t *testing.T) {
	output := captureDisplayOutput(t, func() {
		d := NewDisplay()
		now := time.Now()
		d.now = func() time.Time { return now }
		d.SetRateLimitPause("search", now.Add(10*time.Second))
		if !d.hasIndeterminateUnsafe() {
			t.Fatal("pause countdown cannot refresh without active jobs")
		}
		d.render()
		now = now.Add(3 * time.Second)
		d.render()
		now = now.Add(8 * time.Second)
		d.render()
		if d.hasIndeterminateUnsafe() {
			t.Fatal("expired countdown keeps refreshing")
		}
	})
	for _, want := range []string{"HTTP 429: поиск приостановлен", "Повтор через: 10 с", "Повтор через: 7 с"} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q in %s", want, output)
		}
	}
	final := output[strings.LastIndex(output, "Скачивание"):]
	if strings.Contains(final, "HTTP 429") {
		t.Fatal("expired pause still displayed")
	}
}

func TestDisplayShowsSearchAndDownloadPausesSeparately(t *testing.T) {
	output := captureDisplayOutput(t, func() {
		d := NewDisplay()
		now := time.Now()
		d.now = func() time.Time { return now }
		d.SetRateLimitPause("search", now.Add(5*time.Second))
		d.SetRateLimitPause("download", now.Add(20*time.Second))
		d.SetRateLimitPause("download", now.Add(2*time.Second)) // An older wave must not shorten the UI deadline.
		d.Stop()
	})
	for _, want := range []string{"поиск приостановлен", "скачивание приостановлено", "Повтор через: 5 с", "Повтор через: 20 с"} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q in %s", want, output)
		}
	}
}

func TestPauseExplanationFitsShortTerminal(t *testing.T) {
	output := captureDisplayOutput(t, func() {
		d := NewDisplay()
		now := time.Now()
		d.now = func() time.Time { return now }
		d.interactive = true
		d.size = func() (int, int) { return 50, 10 }
		d.SetTotal(1050)
		d.SetRateLimitPause("download", now.Add(30*time.Second))
		d.Update(StatusEvent{JobID: "1", Kind: StatusRetryWaiting, Name: "Waiting"})
		d.Update(StatusEvent{JobID: "2", Kind: StatusNotFound, Name: "Missing", Err: ErrNotFound})
		d.Stop()
		if d.lines > 9 {
			t.Fatal("pause display caused terminal scrolling")
		}
	})
	final := output[strings.LastIndex(output, "\033[J")+len("\033[J"):]
	for _, want := range []string{"HTTP 429: скачивание приостановлено", "Повтор через: 30 с"} {
		if !strings.Contains(final, want) {
			t.Fatalf("short terminal hid %q: %s", want, final)
		}
	}
}
