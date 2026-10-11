package download

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

// StatusKind — тип события для статус-дисплея.
type StatusKind int

const (
	StatusSearching    StatusKind = iota // поиск треков по запросу
	StatusDownloading                    // скачивание конкретного трека
	StatusDone                           // успешно сохранён
	StatusFailed                         // ошибка на любом этапе
	StatusSkipped                        // файл уже скачан и его хеш совпал
	StatusChecking                       // проверка файла и его хеша
	StatusPreparing                      // получение разрешений и ответа сервера
	StatusNotFound                       // успешный поиск без результатов
	StatusSaving                         // сохранение файла и хеша
	StatusRetryWaiting                   // отложенный повтор в очереди
	StatusQueued                         // ожидание разрешения на старт запроса
	StatusConverting                     // сеть завершена, FFmpeg готовит MP3
)

// StatusEvent — событие жизненного цикла обработки одного трека/запроса.
type StatusEvent struct {
	JobID           string // постоянный ID запроса, не зависит от найденного названия
	Kind            StatusKind
	Source          string // имя провайдера ("youtube")
	Name            string // название запроса или "Artist — Title"
	Proxy           string // безопасный адрес выбранного прокси этого запроса
	Err             error
	TotalBytes      int64 // общий размер (если известен), иначе -1
	DownloadedBytes int64 // сколько байт уже скачано
	BytesPerSecond  float64
	ETA             time.Duration
}

// Display — live-статус в терминале через ANSI.
// Показывает счётчики и только активные треки, без списка завершённых.
type Display struct {
	mu             sync.Mutex
	active         map[string]*StatusEvent // Source::JobID → событие
	order          []string                // порядок появления запросов
	done           []StatusEvent           // только терминальные (Done/Failed/Skipped)
	lines          int                     // сколько строк напечатали в прошлый раз
	started        bool
	dirty          bool
	stopped        bool
	stop           chan struct{}
	wg             sync.WaitGroup
	total          int
	initialSkipped int
	out            io.Writer
	interactive    bool
	size           func() (width, height int)
	lastFrame      []string
	animation      int
	rateLimits     map[string]time.Time
	now            func() time.Time
}

func NewDisplay() *Display {
	fd := int(os.Stdout.Fd())
	return &Display{
		active: make(map[string]*StatusEvent), stop: make(chan struct{}),
		rateLimits: make(map[string]time.Time), now: time.Now,
		out: os.Stdout, interactive: term.IsTerminal(fd),
		size: func() (int, int) {
			width, height, err := term.GetSize(fd)
			if err != nil || width < 1 || height < 1 {
				return 80, 24
			}
			return width, height
		},
	}
}

func (d *Display) SetTotal(total int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.total = total
	d.dirty = true
	d.startUnsafe()
}

// SetInitialSkipped seeds statistics without starting terminal redraws while
// the CLI is still printing startup diagnostics and preparing connections.
func (d *Display) SetInitialSkipped(count int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.initialSkipped = max(0, count)
	d.dirty = true
}

func (d *Display) SetRateLimitPause(stage string, until time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return
	}
	if until.After(d.rateLimits[stage]) {
		d.rateLimits[stage] = until
	}
	d.dirty = true
	d.startUnsafe()
}

func (d *Display) startUnsafe() {
	if d.interactive && !d.started && !d.stopped {
		d.render()
		d.dirty = false
		d.wg.Add(1)
		go d.refresh()
	}
}

// Update — потокобезопасный вызов из горутин-воркеров.
func (d *Display) Update(evt StatusEvent) {
	evt.Proxy = ProxyLabel(evt.Proxy)
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
	if !d.interactive && (evt.Kind == StatusConverting || evt.Kind == StatusDownloading && evt.DownloadedBytes > 0) {
		line := fmt.Sprintf("%s · %s", transferStatus(evt), evt.Name)
		if evt.Proxy == "direct" {
			line += " · напрямую"
		} else if evt.Proxy != "" {
			line += " · прокси: " + evt.Proxy
		}
		line = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, line)
		fmt.Fprintln(d.out, line)
	}

	d.dirty = true
	d.startUnsafe()
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
			if !d.stopped && (d.dirty || d.hasIndeterminateUnsafe()) {
				d.render()
				d.dirty = false
			}
			d.mu.Unlock()
		}
	}
}

func (d *Display) hasIndeterminateUnsafe() bool {
	if len(d.rateLimits) > 0 {
		return true
	}
	for _, ev := range d.active {
		if ev.Kind != StatusDownloading || ev.TotalBytes <= 0 {
			return true
		}
	}
	return false
}

// Stop — финальный вывод (отключает ANSI-перерисовку).
func (d *Display) Stop() {
	d.mu.Lock()
	if !d.stopped {
		d.stopped = true
		close(d.stop)
		d.render()
	}
	d.mu.Unlock()
	d.wg.Wait()
}

// render перерисовывает экран.
// Мьютекс должен быть захвачен вызывающим.
func (d *Display) render() {
	var ok, fail, skipped, notFound int
	skipped = d.initialSkipped
	for _, ev := range d.done {
		switch ev.Kind {
		case StatusDone:
			ok++
		case StatusFailed:
			fail++
		case StatusSkipped:
			skipped++
		case StatusNotFound:
			notFound++
		}
	}
	active := d.activeUnsafe()
	completed := len(d.done) + d.initialSkipped
	total := max(d.total, len(active)+completed)
	queued := max(0, total-len(active)-completed)
	waiting := 0
	processing := active[:0:0]
	for _, ev := range active {
		if ev.Kind == StatusRetryWaiting || ev.Kind == StatusQueued {
			queued++
			if ev.Kind == StatusRetryWaiting {
				waiting++
			}
		} else {
			processing = append(processing, ev)
		}
	}
	active = processing
	width, height := d.size()
	frame := []string{"Скачивание"}
	// Wrap counters between fields, not through values, in narrow windows.
	for _, group := range [][]string{
		{fmt.Sprintf("Всего: %d", total), fmt.Sprintf("В очереди: %d", queued), fmt.Sprintf("В работе: %d", len(active))},
		{fmt.Sprintf("Скачано: %d", ok), fmt.Sprintf("Пропущено: %d", skipped), fmt.Sprintf("Ошибок: %d", fail)},
	} {
		line := ""
		for _, counter := range group {
			if line != "" && runewidth.StringWidth(line+"  "+counter) > width-1 {
				frame = append(frame, line)
				line = ""
			}
			if line != "" {
				line += "  "
			}
			line += counter
		}
		frame = append(frame, line)
	}
	if waiting > 0 {
		frame = append(frame, fmt.Sprintf("Ожидают повтора: %d", waiting))
	}
	for _, pause := range []struct{ stage, label string }{
		{"search", "поиск приостановлен"}, {"download", "скачивание приостановлено"},
	} {
		remaining := d.rateLimits[pause.stage].Sub(d.now())
		if remaining <= 0 {
			delete(d.rateLimits, pause.stage)
			continue
		}
		seconds := int((remaining + time.Second - 1) / time.Second)
		frame = append(frame, "HTTP 429: "+pause.label, fmt.Sprintf("  Повтор через: %d с (автоматически)", seconds))
	}
	var sections []string
	sectionCount := 0
	if notFound > 0 {
		sectionCount++
	}
	if fail > 0 {
		sectionCount++
	}
	detailLimit := 3
	if d.interactive && sectionCount > 0 {
		detailLimit = min(3, max(0, (height-1-len(frame)-sectionCount-min(len(active), 2))/sectionCount))
	}
	for _, group := range []struct {
		kind  StatusKind
		title string
		count int
	}{
		{StatusNotFound, "Не найдено на сайте", notFound},
		{StatusFailed, "Технические ошибки", fail},
	} {
		if group.count == 0 {
			continue
		}
		sections = append(sections, fmt.Sprintf("%s: %d", group.title, group.count))
		shown := 0
		for i := len(d.done) - 1; i >= 0 && shown < detailLimit; i-- {
			ev := d.done[i]
			if ev.Kind != group.kind {
				continue
			}
			line := "  " + ev.Name
			if ev.Kind == StatusFailed && ev.Err != nil {
				line = "  " + safeError(ev.Err) + " — " + ev.Name
			}
			sections = append(sections, line)
			shown++
		}
	}
	// Leave one column and one screen row free to avoid auto-wrap/scroll.
	visible := len(active)
	if d.interactive {
		visible = min(visible, max(0, height-1-len(frame)-len(sections)))
		if visible < len(active) && visible > 0 {
			visible--
		}
	}
	for _, ev := range active[:visible] {
		status := "Ищется"
		pct := -1.0
		switch ev.Kind {
		case StatusChecking:
			status = "Проверка файла"
		case StatusPreparing:
			status = "Подготовка"
		case StatusSaving:
			status = "Сохранение"
		case StatusConverting:
			status = "Конвертация MP3"
		case StatusDownloading:
			status = "Скачивается"
			pct = progressPercent(ev)
			if pct >= 0 {
				status = fmt.Sprintf("Скачивается %3.0f%%", pct)
			}
			status += transferDetails(ev.BytesPerSecond, ev.ETA)
		}
		if ev.Proxy == "direct" {
			status += " · напрямую"
		} else if ev.Proxy != "" {
			status += " · прокси: " + ev.Proxy
		}
		// Keep the progress bar complete; shorten the name first. Narrow
		// windows get a shorter bar rather than a clipped closing bracket.
		available := max(0, width-1-runewidth.StringWidth("  "+status+" []  "))
		nameReserve := min(12, max(0, available-4))
		barCellWidth := max(runewidth.RuneWidth('█'), runewidth.RuneWidth('░'))
		barWidth := min(20, (available-nameReserve)/max(1, barCellWidth))
		if pct < 0 {
			barWidth = min(10, barWidth)
		}
		if barWidth > 0 {
			bar := make([]rune, barWidth)
			filled := int(pct / 100 * float64(barWidth))
			for i := range bar {
				bar[i] = '░'
				if pct >= 0 && i < filled || pct < 0 && i == d.animation%barWidth {
					bar[i] = '█'
				}
			}
			status += " [" + string(bar) + "]"
		}
		frame = append(frame, fmt.Sprintf("  %s  %s", status, ev.Name))
	}
	if visible < len(active) && (!d.interactive || len(frame)+len(sections) < height-1) {
		frame = append(frame, fmt.Sprintf("  … ещё в работе: %d", len(active)-visible))
	}
	frame = append(frame, sections...)
	if d.interactive && len(frame) >= height {
		frame = frame[:max(1, height-1)]
	}
	for i, line := range frame {
		line = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) || unicode.IsSpace(r) {
				return ' '
			}
			return r
		}, line)
		frame[i] = runewidth.Truncate(line, max(0, width-1), "…")
	}
	if d.interactive && d.started && d.lines > 0 {
		// Recalculate physical rows if the terminal became narrower since
		// the preceding frame and reflowed its previously printed text.
		rows := 0
		for _, line := range d.lastFrame {
			rows += physicalRows(line, width)
		}
		rows = min(rows, max(1, height-1))
		fmt.Fprintf(d.out, "\033[%dA\r\033[J", rows)
	}
	for _, line := range frame {
		fmt.Fprintln(d.out, line)
	}
	d.lastFrame = frame
	d.lines = len(frame)
	d.started = true
	d.animation++
}

// Wide glyphs wrap before a one-cell gap, so dividing the total width can
// underestimate the number of physical rows after terminal reflow.
func physicalRows(line string, width int) int {
	width = max(1, width)
	rows := 1
	for runewidth.StringWidth(line) > width {
		prefix := runewidth.Truncate(line, width, "")
		if prefix == "" {
			// Even a single glyph cannot fit in an extremely narrow window.
			_, size := utf8.DecodeRuneInString(line)
			line = line[size:]
		} else {
			line = line[len(prefix):]
		}
		rows++
	}
	return rows
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

func transferStatus(ev StatusEvent) string {
	if ev.Kind == StatusConverting {
		return "Конвертация MP3"
	}
	status := "Скачивается"
	if pct := progressPercent(ev); pct >= 0 {
		status = fmt.Sprintf("Скачивается %.0f%%", pct)
	} else if ev.DownloadedBytes > 0 {
		status += " " + humanBytes(ev.DownloadedBytes)
	}
	return status + transferDetails(ev.BytesPerSecond, ev.ETA)
}

func transferDetails(speed float64, eta time.Duration) string {
	text := ""
	if speed > 0 {
		text += " · " + humanBytes(int64(speed)) + "/с"
	}
	if eta > 0 {
		seconds := int((eta + time.Second - 1) / time.Second)
		text += fmt.Sprintf(" · осталось %02d:%02d", seconds/60, seconds%60)
	}
	return text
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
	return k == StatusDone || k == StatusFailed || k == StatusSkipped || k == StatusNotFound
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
