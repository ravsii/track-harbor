package download

import (
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestActiveOrderDoesNotChangeWithProgress(t *testing.T) {
	d := NewDisplay()
	d.Update(StatusEvent{Kind: StatusDownloading, Source: "test", Name: "Alpha", TotalBytes: 100, DownloadedBytes: 10})
	d.Update(StatusEvent{Kind: StatusDownloading, Source: "test", Name: "Beta", TotalBytes: 100, DownloadedBytes: 90})
	defer d.Stop()

	active := d.activeUnsafe()
	if len(active) != 2 || active[0].Name != "Alpha" || active[1].Name != "Beta" {
		t.Fatalf("rows must stay in arrival order, got %+v", active)
	}
	d.Update(StatusEvent{Kind: StatusDownloading, Source: "test", Name: "Alpha", TotalBytes: 100, DownloadedBytes: 95})
	active = d.activeUnsafe()
	if active[0].Name != "Alpha" || active[1].Name != "Beta" {
		t.Fatalf("progress reordered rows: %+v", active)
	}
}

func captureDisplayOutput(t *testing.T, run func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = stdout }()
	output := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		output <- string(b)
	}()
	run()
	w.Close()
	result := <-output
	r.Close()
	return result
}

func TestDisplayCoalescesRapidProgressUpdates(t *testing.T) {
	output := captureDisplayOutput(t, func() {
		d := NewDisplay()
		for i := int64(0); i <= 100; i++ {
			d.Update(StatusEvent{JobID: "1", Kind: StatusDownloading, Name: "Track", TotalBytes: 100, DownloadedBytes: i})
		}
		d.Stop()
	})
	if frames := strings.Count(output, "━━━ Downloading"); frames > 2 {
		t.Fatalf("rapid updates caused %d redraws, want at most initial and final frames", frames)
	}
	if !strings.Contains(output, "100%") {
		t.Fatal("final frame lost the latest progress")
	}
}

func TestDisplayStopKeepsFinalResults(t *testing.T) {
	output := captureDisplayOutput(t, func() {
		d := NewDisplay()
		d.Update(StatusEvent{JobID: "1", Kind: StatusSearching, Name: "query"})
		d.Update(StatusEvent{JobID: "1", Kind: StatusDone, Name: "Artist — Title"})
		d.Stop()
		d.Stop()
	})
	// The last clear must be followed by the results, not just a count.
	final := output[strings.LastIndex(output, "\033[J")+len("\033[J"):]
	if !strings.Contains(final, "Artist — Title") || !strings.Contains(final, "Done: 1 ok, 0 failed") {
		t.Fatalf("missing final results: %q", final)
	}
	if strings.Count(output, "Done:") != 1 {
		t.Fatal("Stop printed the summary more than once")
	}
}

func TestDisplayDistinguishesIdenticalQueries(t *testing.T) {
	d := NewDisplay()
	defer d.Stop()
	d.Update(StatusEvent{JobID: "1", Kind: StatusSearching, Name: "same query"})
	d.Update(StatusEvent{JobID: "2", Kind: StatusSearching, Name: "same query"})
	d.Update(StatusEvent{JobID: "1", Kind: StatusDone, Name: "found title"})
	active := d.activeUnsafe()
	if len(active) != 1 || active[0].JobID != "2" {
		t.Fatalf("completion removed the wrong job: %+v", active)
	}
}

func TestDisplayStopKeepsAllResults(t *testing.T) {
	output := captureDisplayOutput(t, func() {
		d := NewDisplay()
		for i := 1; i <= 12; i++ {
			id := strconv.Itoa(i)
			d.Update(StatusEvent{JobID: id, Kind: StatusDone, Name: "Track " + id})
		}
		d.Stop()
	})
	final := output[strings.LastIndex(output, "\033[J")+len("\033[J"):]
	if got := strings.Count(final, "✓"); got != 12 {
		t.Fatalf("final output retained %d of 12 results: %q", got, final)
	}
}
