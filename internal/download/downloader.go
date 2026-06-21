package download

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sync"
	"sync/atomic"
	"time"
)

// progressReader оборачивает io.ReadCloser и вызывает callback
// асинхронно с частотой не чаще чем throttle (не блокирует чтение).
type progressReader struct {
	reader     io.ReadCloser
	total      int64
	downloaded atomic.Int64
	onProgress func(downloaded, total int64)
}

func newProgressReader(r io.ReadCloser, total int64, onProgress func(downloaded, total int64)) *progressReader {
	return &progressReader{
		reader:     r,
		total:      total,
		onProgress: onProgress,
	}
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.downloaded.Add(int64(n))
		downloaded := pr.downloaded.Load()

		// колбэк асинхронно — не блокируем io.Copy
		if pr.onProgress != nil {
			pr.onProgress(downloaded, pr.total)
		}
	}
	return n, err
}

func (pr *progressReader) Close() error {
	return pr.reader.Close()
}

// Downloader — оркестратор загрузок с worker pool.
type Downloader struct {
	provider  Provider
	workers   int
	outputDir string
	display   *Display
}

// New создаёт Downloader.
func New(provider Provider, workers int, outputDir string) *Downloader {
	return &Downloader{provider: provider, workers: workers, outputDir: outputDir}
}

// WithDisplay привязывает live-дисплей для отображения статуса.
func (d *Downloader) WithDisplay(disp *Display) *Downloader {
	d.display = disp
	return d
}

// Run запускает worker pool для параллельной обработки поисковых запросов.
func (d *Downloader) Run(queries []string) {
	sem := make(chan struct{}, d.workers)
	var wg sync.WaitGroup

	for _, query := range queries {
		wg.Add(1)
		sem <- struct{}{}

		go func(q string) {
			defer wg.Done()
			defer func() { <-sem }()
			d.downloadQuery(q)
		}(query)
	}

	wg.Wait()

	if d.display != nil {
		time.Sleep(100 * time.Millisecond)
		d.display.Stop()
	}
}

func (d *Downloader) sendEvent(kind StatusKind, name string, err error) {
	if d.display == nil {
		switch kind {
		case StatusSearching:
			fmt.Println(name)
		case StatusFailed:
			fmt.Printf("err: %s — %v\n", name, err)
		}
		return
	}

	d.display.Update(StatusEvent{
		Kind:   kind,
		Source: d.provider.Name(),
		Name:   name,
		Err:    err,
	})
}

func (d *Downloader) downloadQuery(query string) {
	d.sendEvent(StatusSearching, query, nil)

	track, err := d.provider.Search(query)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			d.sendEvent(StatusFailed, query, fmt.Errorf("no results"))
		} else {
			d.sendEvent(StatusFailed, query, err)
		}
		return
	}

	name := track.Artist + " — " + track.Title

	resp, err := d.provider.Download(track)
	if err != nil {
		d.sendEvent(StatusFailed, name, err)
		return
	}

	filePath := path.Join(d.outputDir, track.ID+".mp3")
	f, err := os.Create(filePath)
	if err != nil {
		resp.Body.Close()
		d.sendEvent(StatusFailed, name, err)
		return
	}

	// Шлём начальное событие старта скачивания
	d.sendEventDownloading(name, resp.ContentLength, 0)

	// pr := newProgressReader(resp.Body, resp.ContentLength,
	// 	func(downloaded, total int64) {
	// 		d.sendEventDownloading(name, total, downloaded)
	// 	})

	_, err = io.Copy(f, resp.Body)
	// pr.Close()
	f.Close()

	if err != nil {
		os.Remove(filePath)
		d.sendEvent(StatusFailed, name, err)
		return
	}

	d.sendEvent(StatusDone, name, nil)
}

// sendEventDownloading отправляет событие скачивания с прогрессом.
func (d *Downloader) sendEventDownloading(name string, total, downloaded int64) {
	if d.display == nil {
		return
	}
	d.display.Update(StatusEvent{
		Kind:            StatusDownloading,
		Source:          d.provider.Name(),
		Name:            name,
		TotalBytes:      total,
		DownloadedBytes: downloaded,
	})
}
