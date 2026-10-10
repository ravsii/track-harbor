package download

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	store     DownloadStore
	fileLocks sync.Map // absolute file path → *sync.Mutex
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
	if d.display != nil {
		d.display.SetTotal(len(queries))
	}
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
	d.sendEvent(StatusChecking, jobID, name, nil)
	filePath, err := filepath.Abs(filepath.Join(d.outputDir, track.ID+".mp3"))
	if err != nil {
		d.sendEvent(StatusFailed, jobID, name, err)
		return
	}
	// Different playlist entries can resolve to the same provider track.
	// Check and download under one lock so they cannot overwrite each other.
	lock, _ := d.fileLocks.LoadOrStore(filePath, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	verified, err := d.alreadyDownloaded(track.ID, filePath)
	if err != nil {
		d.sendEvent(StatusFailed, jobID, name, fmt.Errorf("проверка скачанного файла: %w", err))
		return
	}
	if verified {
		d.sendEvent(StatusSkipped, jobID, name, nil)
		return
	}
	d.sendEvent(StatusPreparing, jobID, name, nil)

	resp, err := d.provider.Download(track)
	if err != nil {
		d.sendEvent(StatusFailed, jobID, name, err)
		return
	}

	// Only replace the destination after a complete, successfully closed copy.
	f, err := os.CreateTemp(filepath.Dir(filePath), ".download-*.tmp")
	if err != nil {
		resp.Body.Close()
		d.sendEvent(StatusFailed, jobID, name, err)
		return
	}
	defer os.Remove(f.Name())
	defer f.Close()

	// Шлём начальное событие старта скачивания
	d.sendEventDownloading(jobID, name, resp.ContentLength, 0)

	pr := newProgressReader(resp.Body, resp.ContentLength,
		func(downloaded, total int64) {
			d.sendEventDownloading(jobID, name, total, downloaded)
		})

	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(f, hash), pr)
	bodyCloseErr := pr.Close()
	fileCloseErr := f.Close()
	if err == nil {
		err = errors.Join(bodyCloseErr, fileCloseErr)
	}
	if err == nil && resp.ContentLength >= 0 && written != resp.ContentLength {
		err = fmt.Errorf("неполный файл: получено %d из %d байт", written, resp.ContentLength)
	}
	if err == nil {
		err = os.Rename(f.Name(), filePath)
	}
	if err == nil && d.store != nil {
		err = d.store.SaveHash(d.provider.Name(), track.ID, filePath, fmt.Sprintf("%x", hash.Sum(nil)))
	}

	if err != nil {
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
