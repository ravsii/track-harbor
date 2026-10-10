package download

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"sync"
	"sync/atomic"
)

// progressReader сообщает число прочитанных байт после каждого чтения.
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

	for i, query := range queries {
		wg.Add(1)
		sem <- struct{}{}
		jobID := strconv.Itoa(i + 1)
		d.sendEvent(StatusSearching, jobID, query, nil)

		go func(id, q string) {
			defer wg.Done()
			defer func() { <-sem }()
			d.downloadQuery(id, q)
		}(jobID, query)
	}

	wg.Wait()

	if d.display != nil {
		d.display.Stop()
	}
}

func (d *Downloader) sendEvent(kind StatusKind, jobID, name string, err error) {
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
		JobID:  jobID,
		Kind:   kind,
		Source: d.provider.Name(),
		Name:   name,
		Err:    err,
	})
}

func (d *Downloader) downloadQuery(jobID, query string) {
	track, err := d.provider.Search(query)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			d.sendEvent(StatusFailed, jobID, query, fmt.Errorf("no results"))
		} else {
			d.sendEvent(StatusFailed, jobID, query, err)
		}
		return
	}

	name := track.Artist + " — " + track.Title
	d.sendEvent(StatusSearching, jobID, name, nil)

	resp, err := d.provider.Download(track)
	if err != nil {
		d.sendEvent(StatusFailed, jobID, name, err)
		return
	}

	filePath := path.Join(d.outputDir, track.ID+".mp3")
	f, err := os.Create(filePath)
	if err != nil {
		resp.Body.Close()
		d.sendEvent(StatusFailed, jobID, name, err)
		return
	}

	// Шлём начальное событие старта скачивания
	d.sendEventDownloading(jobID, name, resp.ContentLength, 0)

	pr := newProgressReader(resp.Body, resp.ContentLength,
		func(downloaded, total int64) {
			d.sendEventDownloading(jobID, name, total, downloaded)
		})

	_, err = io.Copy(f, pr)
	pr.Close()
	f.Close()

	if err != nil {
		os.Remove(filePath)
		d.sendEvent(StatusFailed, jobID, name, err)
		return
	}

	d.sendEvent(StatusDone, jobID, name, nil)
}

// sendEventDownloading отправляет событие скачивания с прогрессом.
func (d *Downloader) sendEventDownloading(jobID, name string, total, downloaded int64) {
	if d.display == nil {
		return
	}
	d.display.Update(StatusEvent{
		JobID:           jobID,
		Kind:            StatusDownloading,
		Source:          d.provider.Name(),
		Name:            name,
		TotalBytes:      total,
		DownloadedBytes: downloaded,
	})
}
