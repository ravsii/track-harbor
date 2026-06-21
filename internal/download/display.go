package download

import (
	"fmt"
	"sync"
)

// StatusKind — тип события для статус-дисплея.
type StatusKind int

const (
	StatusSearching   StatusKind = iota // поиск треков по запросу
	StatusDownloading                    // скачивание конкретного трека
	StatusDone                           // успешно сохранён
	StatusFailed                         // ошибка на любом этапе
)

// StatusEvent — событие жизненного цикла обработки одного трека/запроса.
type StatusEvent struct {
	Kind   StatusKind
	Source string // имя провайдера ("mp3party")
	Name   string // название запроса или "Artist — Title"
	Err    error
}

// Display — live-статус в терминале через ANSI.
// Выводит две секции: "Downloading" (активные) и "Completed" (завершённые).
type Display struct {
	mu      sync.Mutex
	done    []StatusEvent
	lines   int  // сколько строк напечатали в прошлый раз
	started bool
}

func NewDisplay() *Display { return &Display{} }

// Update — потокобезопасный вызов из горутин-воркеров.
func (d *Display) Update(evt StatusEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.done = append(d.done, evt)
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
			desc = "downloading..."
		}
		fmt.Printf("  %s [%s] %s  %s\n", icon, ev.Source, ev.Name, desc)
		rows++
	}

	if len(active) == 0 {
		fmt.Println("  (idle)")
		rows++
	}

	// ---- Completed ----
	if len(d.done) > 0 {
		fmt.Println("━━━ Completed ───────────────────────")
		rows++

		const maxDisplay = 15
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

// activeUnsafe возвращает активные (без завершающего Done/Failed для того же Name+Source).
func (d *Display) activeUnsafe() []StatusEvent {
	// помечаем индексы терминальных событий
	terminalIdx := make(map[string]int)
	for i, ev := range d.done {
		if isTerminal(ev.Kind) {
			key := ev.Source + "::" + ev.Name
			terminalIdx[key] = i
		}
	}

	var active []StatusEvent
	for i := len(d.done) - 1; i >= 0 && len(active) < 5; i-- {
		ev := d.done[i]
		if ev.Kind == StatusSearching || ev.Kind == StatusDownloading {
			key := ev.Source + "::" + ev.Name
			if termIdx, ok := terminalIdx[key]; ok && termIdx > i {
				continue // есть более поздний терминальный — пропускаем
			}
			active = append(active, ev)
		}
	}

	// разворачиваем в хронологический порядок
	for i, j := 0, len(active)-1; i < j; i, j = i+1, j-1 {
		active[i], active[j] = active[j], active[i]
	}
	return active
}

func isTerminal(k StatusKind) bool {
	return k == StatusDone || k == StatusFailed
}
