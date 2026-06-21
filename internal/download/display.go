package download

import (
	"fmt"
	"sort"
	"sync"
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
	active  map[string]*StatusEvent // текущее состояние активных (Source::Name → событие)
	done    []StatusEvent           // только терминальные (Done/Failed)
	lines   int                     // сколько строк напечатали в прошлый раз
	started bool
}

func NewDisplay() *Display {
	return &Display{active: make(map[string]*StatusEvent)}
}

// Update — потокобезопасный вызов из горутин-воркеров.
func (d *Display) Update(evt StatusEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := evt.Source + "::" + evt.Name

	if isTerminal(evt.Kind) {
		// Терминальное — убираем из активных и добавляем в завершённые
		delete(d.active, key)
		d.done = append(d.done, evt)
	} else {
		// Промежуточное (Searching/Downloading) — обновляем или создаём в active
		d.active[key] = &evt
	}

	d.render()
}

// Stop — финальный вывод (отключает ANSI-перерисовку).
func (d *Display) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.lines > 0 {
		fmt.Printf("\033[%dA", d.lines)
		fmt.Print("\033[J")
	}

	d.printSummary()
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
		if len(d.done) > maxDisplay {
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

		if len(d.done) > maxDisplay {
			remaining := len(d.done) - maxDisplay
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

// progress сортирует активные события:
// 1. Downloading — по убыванию процента прогресса (самые близкие к завершению сверху)
// 2. Searching — в конце
func sortActive(evs []StatusEvent) {
	sort.Slice(evs, func(i, j int) bool {
		// Сначала Downloading, потом Searching
		if evs[i].Kind != evs[j].Kind {
			return evs[i].Kind == StatusDownloading
		}
		// Оба Downloading — сортируем по убыванию прогресса
		if evs[i].Kind == StatusDownloading {
			pctI := progressPercent(evs[i])
			pctJ := progressPercent(evs[j])
			if pctI != pctJ {
				return pctI > pctJ
			}
		}
		// Стабильный порядок для одинаковых — по имени
		return evs[i].Name < evs[j].Name
	})
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

// activeUnsafe возвращает активные события (из карты active), отсортированные по прогрессу.
// Мьютекс должен быть захвачен вызывающим.
func (d *Display) activeUnsafe() []StatusEvent {
	if len(d.active) == 0 {
		return nil
	}
	out := make([]StatusEvent, 0, len(d.active))
	for _, ev := range d.active {
		out = append(out, *ev)
	}
	sortActive(out)
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
