package download

import (
	"errors"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
)

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
	go func() { b, _ := io.ReadAll(r); output <- string(b) }()
	run()
	w.Close()
	result := <-output
	r.Close()
	return result
}

func TestActiveOrderDoesNotChangeWithProgress(t *testing.T) {
	d := NewDisplay()
	d.Update(StatusEvent{JobID: "1", Kind: StatusDownloading, Name: "Alpha", TotalBytes: 100, DownloadedBytes: 10})
	d.Update(StatusEvent{JobID: "2", Kind: StatusDownloading, Name: "Beta", TotalBytes: 100, DownloadedBytes: 90})
	defer d.Stop()
	active := d.activeUnsafe()
	if len(active) != 2 || active[0].Name != "Alpha" || active[1].Name != "Beta" {
		t.Fatalf("rows changed arrival order: %+v", active)
	}
	d.Update(StatusEvent{JobID: "1", Kind: StatusDownloading, Name: "Alpha", TotalBytes: 100, DownloadedBytes: 95})
	active = d.activeUnsafe()
	if active[0].Name != "Alpha" || active[1].Name != "Beta" {
		t.Fatal("progress reordered rows")
	}
}

func TestDisplayShowsOnlyActiveTracksAndCounters(t *testing.T) {
	output := captureDisplayOutput(t, func() {
		d := NewDisplay()
		d.Update(StatusEvent{JobID: "1", Kind: StatusSearching, Name: "Active track"})
		d.Update(StatusEvent{JobID: "2", Kind: StatusDone, Name: "Completed track"})
		d.Update(StatusEvent{JobID: "3", Kind: StatusSkipped, Name: "Skipped track"})
		d.Update(StatusEvent{JobID: "4", Kind: StatusFailed, Name: "Failed track", Err: errors.New("offline")})
		d.Stop()
		d.Stop()
	})
	for _, want := range []string{"Всего: 4", "В очереди: 0", "В работе: 1", "Скачано: 1", "Пропущено: 1", "Ошибок: 1", "Ищется", "Active track"} {
		if !strings.Contains(output, want) {
			t.Errorf("missing %q: %q", want, output)
		}
	}
	for _, unwanted := range []string{"Completed track", "Skipped track", "Failed track", "Completed ─", "offline"} {
		if strings.Contains(output, unwanted) {
			t.Errorf("completed history leaked into statistics: %q", output)
		}
	}
}

func TestDisplayDoesNotGrowWithCompletedTrackCount(t *testing.T) {
	output := captureDisplayOutput(t, func() {
		d := NewDisplay()
		for i := 1; i <= 100; i++ {
			d.Update(StatusEvent{JobID: strconv.Itoa(i), Kind: StatusDone, Name: "Finished track " + strconv.Itoa(i)})
		}
		d.Stop()
	})
	if strings.Contains(output, "Finished track") || strings.Count(output, "\n") > 6 || !strings.Contains(output, "Скачано: 100") {
		t.Fatalf("summary accumulated track rows instead of counts: %q", output)
	}
}

func TestDisplayLongUnicodeNamesCannotWrapRows(t *testing.T) {
	output := captureDisplayOutput(t, func() {
		d := NewDisplay()
		d.Update(StatusEvent{JobID: "1", Kind: StatusDownloading, Name: strings.Repeat("龙 é 🎵 very long song ", 30), TotalBytes: 100, DownloadedBytes: 64})
		d.Stop()
	})
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "\033") {
			continue
		}
		if width := runewidth.StringWidth(line); width >= 80 {
			t.Fatalf("line wraps at 80 columns (%d cells): %q", width, line)
		}
	}
	if !strings.Contains(output, "64%") || !strings.Contains(output, "…") {
		t.Fatalf("truncation lost download progress: %q", output)
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
		t.Fatalf("completion removed wrong job: %+v", active)
	}
}

func TestDisplayPipedOutputIsOneSummaryWithoutANSI(t *testing.T) {
	output := captureDisplayOutput(t, func() {
		d := NewDisplay()
		for i := int64(0); i <= 100; i++ {
			d.Update(StatusEvent{JobID: "1", Kind: StatusDownloading, Name: "Track", TotalBytes: 100, DownloadedBytes: i})
		}
		d.Update(StatusEvent{JobID: "1", Kind: StatusDone, Name: "Track"})
		d.Stop()
	})
	if strings.Contains(output, "\033") || strings.Count(output, "Всего:") != 1 || !strings.Contains(output, "Скачано: 1") {
		t.Fatalf("pipe received redraws instead of a summary: %q", output)
	}
}

func TestDisplayQueueCountsDoNotChangeWithPhaseUpdates(t *testing.T) {
	output := captureDisplayOutput(t, func() {
		d := NewDisplay()
		d.SetTotal(12)
		d.Update(StatusEvent{JobID: "1", Kind: StatusSearching, Name: "First"})
		d.Update(StatusEvent{JobID: "2", Kind: StatusSearching, Name: "Second"})
		d.Update(StatusEvent{JobID: "1", Kind: StatusChecking, Name: "First"})
		d.Update(StatusEvent{JobID: "1", Kind: StatusPreparing, Name: "First"})
		d.Update(StatusEvent{JobID: "1", Kind: StatusDownloading, Name: "First", TotalBytes: 100, DownloadedBytes: 50})
		d.Update(StatusEvent{JobID: "2", Kind: StatusSkipped, Name: "Second"})
		d.Stop()
	})
	for _, want := range []string{"Всего: 12", "В очереди: 10", "В работе: 1", "Пропущено: 1", "50%"} {
		if !strings.Contains(output, want) {
			t.Errorf("incorrect queue/phase counters, missing %q: %q", want, output)
		}
	}
}

func TestInteractiveDisplayFitsNarrowAndShortTerminal(t *testing.T) {
	output := captureDisplayOutput(t, func() {
		d := NewDisplay()
		d.interactive = true
		width := 80
		d.size = func() (int, int) { return width, 8 }
		d.SetTotal(12)
		for i := 0; i < 10; i++ {
			d.Update(StatusEvent{JobID: strconv.Itoa(i), Kind: StatusDownloading, Name: strings.Repeat("龙 é 🎵 ", 20) + "\n\033[2J", TotalBytes: 100, DownloadedBytes: 64})
		}
		// Resize after the initial frame. Render under the same lock as refresh.
		d.mu.Lock()
		width = 32
		d.mu.Unlock()
		d.Stop()
		if d.lines > 7 {
			t.Fatalf("frame scrolls beyond terminal height: %d rows", d.lines)
		}
		for _, line := range d.lastFrame {
			if runewidth.StringWidth(line) >= 32 || strings.ContainsAny(line, "\n\r\t\033") {
				t.Fatalf("terminal row can wrap or move cursor: %q", line)
			}
		}
	})
	if !strings.Contains(output, "64%") || !strings.Contains(output, "ещё в работе") || !strings.Contains(output, "…") {
		t.Fatalf("narrow display lost useful status: %q", output)
	}
	if strings.Contains(output, "\033[2J") {
		t.Fatal("track metadata injected a clear-screen escape")
	}
}

func TestInteractiveDisplayCoalescesRapidProgressUpdates(t *testing.T) {
	output := captureDisplayOutput(t, func() {
		d := NewDisplay()
		d.interactive = true
		d.size = func() (int, int) { return 80, 24 }
		for i := int64(0); i <= 100; i++ {
			d.Update(StatusEvent{JobID: "1", Kind: StatusDownloading, Name: "Track", TotalBytes: 100, DownloadedBytes: i})
		}
		d.Stop()
	})
	if frames := strings.Count(output, "Скачивание\n"); frames > 2 {
		t.Fatalf("progress caused %d redraws instead of coalescing", frames)
	}
	if !strings.Contains(output, "100%") {
		t.Fatal("final frame lost latest progress")
	}
}

func TestInteractiveResizeClearsAlreadyRenderedWideTrack(t *testing.T) {
	var before, after []string
	output := captureDisplayOutput(t, func() {
		d := NewDisplay()
		d.interactive = true
		width := 120
		d.size = func() (int, int) { return width, 24 }
		// 43 prefix/bar cells + 37 double-width glyphs = 117 cells. At
		// 39 columns this occupies FOUR rows, not ceil(117 / 39) = three.
		d.Update(StatusEvent{JobID: "1", Kind: StatusDownloading, Name: strings.Repeat("龙", 37), TotalBytes: 100, DownloadedBytes: 64})
		d.mu.Lock()
		before = append([]string(nil), d.lastFrame...)
		width = 39
		d.render()
		after = append([]string(nil), d.lastFrame...)
		d.mu.Unlock()
		d.Stop()
	})
	// Simulate terminal reflow independently: the fixture contains only
	// single-cell text and 龙 (two cells), which wraps BEFORE a one-cell gap.
	var screen []string
	for _, line := range before {
		row, col := "", 0
		for _, r := range line {
			cells := 1
			if r == '龙' {
				cells = 2
			}
			if col+cells > 39 {
				screen = append(screen, row)
				row, col = "", 0
			}
			row += string(r)
			col += cells
		}
		screen = append(screen, row)
	}
	if len(screen) != 7 {
		t.Fatalf("unexpected fixture screen: %q", screen)
	}
	match := regexp.MustCompile("\033\\[([0-9]+)A\r\033\\[J").FindStringSubmatch(output)
	if match == nil {
		t.Fatalf("resize did not clear the previous frame: %q", output)
	}
	up, err := strconv.Atoi(match[1])
	if err != nil || up > len(screen) {
		t.Fatalf("invalid cursor movement: %v %v", up, err)
	}
	// CSI J clears from the cursor down; anything above that position survives.
	screen = append(screen[:len(screen)-up], after...)
	if headings := strings.Count(strings.Join(screen, "\n"), "Скачивание"); headings != 1 {
		t.Fatalf("resize left an old heading on screen (%d headings): %q", headings, screen)
	}
}

func TestActiveDownloadsKeepProgressBarWithLongNames(t *testing.T) {
	for _, width := range []int{80, 32} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			output := captureDisplayOutput(t, func() {
				d := NewDisplay()
				d.size = func() (int, int) { return width, 24 }
				d.Update(StatusEvent{JobID: "1", Kind: StatusDownloading, Name: strings.Repeat("Long title ", 20), TotalBytes: 100, DownloadedBytes: 64})
				d.Stop()
			})
			if !strings.Contains(output, "64%") || !regexp.MustCompile(`\[[█░]+\]`).MatchString(output) || !strings.Contains(output, "…") {
				t.Fatalf("long title removed progress bar: %q", output)
			}
			if width == 80 && !strings.Contains(output, "[████████████░░░░░░░░]") {
				t.Fatalf("progress bar does not reflect 64%%: %q", output)
			}
			for _, line := range strings.Split(output, "\n") {
				if runewidth.StringWidth(line) >= width {
					t.Fatalf("progress row wraps: %q", line)
				}
			}
		})
	}
}

func TestSearchShowsIndeterminateProgressWithoutInventedPercentage(t *testing.T) {
	output := captureDisplayOutput(t, func() {
		d := NewDisplay()
		d.Update(StatusEvent{JobID: "1", Kind: StatusSearching, Name: "Track"})
		d.Stop()
	})
	if !strings.Contains(output, "Ищется") || !regexp.MustCompile(`\[[█░]+\]`).MatchString(output) || strings.Contains(output, "%") {
		t.Fatalf("search needs an indeterminate bar, not a fabricated percentage: %q", output)
	}
}

type searchFrameObserver struct{ frames chan string }

func (w searchFrameObserver) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "Ищется") {
		select {
		case w.frames <- string(p):
		default:
		}
	}
	return len(p), nil
}

func TestSearchProgressAnimatesWhileWaitingWithoutNewEvents(t *testing.T) {
	frames := make(chan string, 16)
	d := NewDisplay()
	d.interactive = true
	d.out = searchFrameObserver{frames: frames}
	d.size = func() (int, int) { return 80, 24 }
	d.Update(StatusEvent{JobID: "1", Kind: StatusSearching, Name: "Track"})
	defer d.Stop()
	first := <-frames
	select {
	case next := <-frames:
		if next == first {
			t.Fatal("indeterminate progress bar did not move")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("search progress froze while waiting for the provider")
	}
}
