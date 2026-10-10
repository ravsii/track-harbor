package download

import (
	"fmt"
	"sync"
	"time"
)

// StatusKind — тип события для статус-дисплея.
type StatusKind int

const (
	StatusSearching   StatusKind = iota // поиск треков по запросу
	StatusDownloading                   // скачивание конкретного трека
	StatusDone                          // успешно сохранён
	StatusFailed                        // ошибка на любом этапе
)

// StatusEvent — событие жизненного цикла обработки одного трека/запроса.
type StatusEvent struct {
	JobID           string // постоянный ID запроса, не зависит от найденного названия
	Kind            StatusKind
	Source          string // имя провайдера ("mp3party")
	Name            string // название запроса или "Artist — Title"
	Err             error
	TotalBytes      int64 // общий размер (если известен), иначе -1
	DownloadedBytes int64 // сколько байт уже скачано
}

// Display — live-статус в терминале через ANSI.
// Выводит две секции: "Downloading" (активные) и "Completed" (завершённые).
type Display struct {
	mu      sync.Mutex
	active  map[string]*StatusEvent // Source::JobID → событие
	order   []string                // порядок появления запросов
	done    []StatusEvent           // только терминальные (Done/Failed)
	lines   int                     // сколько строк напечатали в прошлый раз
	started bool
	dirty   bool
	stopped bool
	stop    chan struct{}
	wg      sync.WaitGroup
}

func NewDisplay() *Display {
	return &Display{active: make(map[string]*StatusEvent), stop: make(chan struct{})}
}

// Update — потокобезопасный вызов из горутин-воркеров.
func (d *Display) Update(evt StatusEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return
	}

	id := evt.JobID
	if id == "" {
		id = evt.Name
	}
	key := evt.Source + "::" + id

	if isTerminal(evt.Kind) {
		// Терминальное — убираем из активных и добавляем в завершённые
		delete(d.active, key)
		for i, k := range d.order {
			if k == key {
				d.order = append(d.order[:i], d.order[i+1:]...)
				break
			}
		}
		d.done = append(d.done, evt)
	} else {
		// Промежуточное (Searching/Downloading) — обновляем или создаём в active
		if _, exists := d.active[key]; !exists {
			d.order = append(d.order, key)
		}
		d.active[key] = &evt
	}

	d.dirty = true
	if !d.started {
		d.render()
		d.dirty = false
		d.wg.Add(1)
		go d.refresh()
	}
}

// refresh отделяет частоту перерисовки от скорости чтения файлов.
func (d *Display) refresh() {
	defer d.wg.Done()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			d.mu.Lock()
			if d.dirty && !d.stopped {
				d.render()
				d.dirty = false
			}
			d.mu.Unlock()
		}
	}
}

// Stop — финальный вывод (отключает ANSI-перерисовку).
func (d *Display) Stop() {
	d.mu.Lock()
	if !d.stopped {
		d.stopped = true
		close(d.stop)
		d.render()
		d.printSummary()
	}
	d.mu.Unlock()
	d.wg.Wait()
}

// render перерисовывает экран.
// Мьютекс должен быть захвачен вызывающим.
func (d *Display) render() {
	if !d.started {
		d.started = true
	} else if d.lines > 0 {
		fmt.Printf("\033[%dA", d.lines)
		fmt.Print("\033[J")
	}

	// ---- Downloading ----
	fmt.Println("━━━ Downloading ━━━━━━━━━━━━━━━━━━━━━")
	rows := 1

	active := d.activeUnsafe()
	for _, ev := range active {
		icon := "⋯"
		desc := "searching..."
		if ev.Kind == StatusDownloading {
			icon = "⬇"
			desc = ev.progressBar()
		}
		fmt.Printf("  %s [%s] %s  %s\n", icon, ev.Source, ev.Name, desc)
		rows++
	}

	if len(active) == 0 {
		fmt.Println("  (idle)")
		rows++
	}

	// ---- Completed (только терминальные) ----
	if len(d.done) > 0 {
		fmt.Println("━━━ Completed ───────────────────────")
		rows++

		const maxDisplay = 10
		start := 0
		if !d.stopped && len(d.done) > maxDisplay {
			start = len(d.done) - maxDisplay
		}

		for _, ev := range d.done[start:] {
			icon := "✓"
			var desc string
			if ev.Kind == StatusFailed {
				icon = "✗"
				desc = ev.Err.Error()
			}
			if desc != "" {
				fmt.Printf("  %s [%s] %s — %s\n", icon, ev.Source, ev.Name, desc)
			} else {
				fmt.Printf("  %s [%s] %s\n", icon, ev.Source, ev.Name)
			}
			rows++
		}

		if start > 0 {
			remaining := start
			fmt.Printf("  … and %d more\n", remaining)
			rows++
		}
	}

	d.lines = rows
}

// printSummary печатает итоговую статистику.
func (d *Display) printSummary() {
	var ok, fail int
	for _, ev := range d.done {
		switch ev.Kind {
		case StatusDone:
			ok++
		case StatusFailed:
			fail++
		}
	}
	fmt.Printf("\nDone: %d ok, %d failed\n", ok, fail)
}

// progressPercent возвращает 0..100 прогресс скачивания (или -1 если неизвестен).
func progressPercent(ev StatusEvent) float64 {
	if ev.TotalBytes <= 0 {
		return -1
	}
	pct := float64(ev.DownloadedBytes) / float64(ev.TotalBytes) * 100
	if pct > 100 {
		pct = 100
	}
	return pct
}

// activeUnsafe возвращает активные события в порядке появления запросов.
// Мьютекс должен быть захвачен вызывающим.
func (d *Display) activeUnsafe() []StatusEvent {
	if len(d.active) == 0 {
		return nil
	}
	out := make([]StatusEvent, 0, len(d.active))
	for _, key := range d.order {
		out = append(out, *d.active[key])
	}
	return out
}

func isTerminal(k StatusKind) bool {
	return k == StatusDone || k == StatusFailed
}

// progressBar возвращает визуальный индикатор прогресса скачивания,
// либо "downloading..." если размер неизвестен.
func (e StatusEvent) progressBar() string {
	if e.TotalBytes <= 0 {
		return "downloading..."
	}
	if e.DownloadedBytes >= e.TotalBytes {
		return " 100% " + progressBarString(1.0)
	}
	pct := float64(e.DownloadedBytes) / float64(e.TotalBytes)
	return fmt.Sprintf(" %3.0f%% %s", pct*100, progressBarString(pct))
}

// progressBarString рисует полоску: [████████░░░░] шириной 20 символов.
func progressBarString(pct float64) string {
	const barWidth = 20
	filled := int(pct * barWidth)
	if filled > barWidth {
		filled = barWidth
	}
	bar := make([]rune, barWidth+2)
	bar[0] = '['
	for i := 1; i <= barWidth; i++ {
		if i <= filled {
			bar[i] = '█'
		} else {
			bar[i] = '░'
		}
	}
	bar[barWidth+1] = ']'
	return string(bar)
}

// humanBytes форматирует байты в человекопонятный вид (КБ, МБ).
func humanBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
}
