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
	"time"

	"go.uber.org/zap"
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
	provider       Provider
	workers        int
	outputDir      string
	display        *Display
	store          DownloadStore
	logger         *zap.Logger
	retryDelay     func(int) time.Duration
	startInterval  time.Duration
	rateLimitDelay func(int) time.Duration
	searchGate     *requestGate
	downloadGate   *requestGate
	fileLocks      sync.Map // absolute file path → *sync.Mutex
}

// New создаёт Downloader.
func New(provider Provider, workers int, outputDir string) *Downloader {
	return &Downloader{provider: provider, workers: max(1, workers), outputDir: outputDir, logger: zap.NewNop(), retryDelay: retryDelay, startInterval: time.Second, rateLimitDelay: rateLimitDelay}
}

const maxRetries = 5

func retryDelay(retry int) time.Duration { return time.Second << retry }

// WithDisplay привязывает live-дисплей для отображения статуса.
func (d *Downloader) WithDisplay(disp *Display) *Downloader {
	d.display = disp
	return d
}

// Run запускает worker pool для параллельной обработки поисковых запросов.
func (d *Downloader) Run(queries []string) {
	d.searchGate = &requestGate{delay: d.rateLimitDelay}
	d.downloadGate = &requestGate{interval: d.startInterval, delay: d.rateLimitDelay}
	started := time.Now()
	d.logger.Info("run_started", zap.Int("total", len(queries)), zap.Int("workers", d.workers), zap.String("output_dir", d.outputDir))
	if d.display != nil {
		d.display.SetTotal(len(queries))
	}
	type job struct {
		id, query      string
		retry, attempt int
	}
	queue := make(chan job, len(queries))
	var jobs, workers sync.WaitGroup
	jobs.Add(len(queries))
	for i, query := range queries {
		queue <- job{id: strconv.Itoa(i + 1), query: query}
	}
	for i := 0; i < d.workers; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for current := range queue {
				retryNeeded, rateLimited := d.downloadQuery(current.id, current.query, current.retry, current.attempt)
				if retryNeeded {
					delay := d.retryDelay(current.retry)
					if rateLimited {
						delay = 0
					} else {
						current.retry++
					}
					current.attempt++
					time.AfterFunc(delay, func() { queue <- current })
				} else {
					jobs.Done()
				}
			}
		}()
	}
	// A job remains outstanding while its delayed retry is waiting.
	jobs.Wait()
	close(queue)
	workers.Wait()
	d.logger.Info("run_finished", zap.Int("total", len(queries)), zap.Duration("duration", time.Since(started)))

	if d.display != nil {
		d.display.Stop()
	}
}

func (d *Downloader) sendEvent(kind StatusKind, jobID, name string, err error) {
	if d.display == nil {
		switch kind {
		case StatusSearching:
			fmt.Println(name)
		case StatusFailed, StatusNotFound:
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

func (d *Downloader) downloadQuery(jobID, query string, retry, attempt int) (retryNeeded, rateLimited bool) {
	started := time.Now()
	name := query
	logger := d.logger.With(zap.String("job_id", jobID), zap.String("source", d.provider.Name()), zap.String("query", query), zap.Int("attempt", attempt+1))
	var generation uint64
	gate := d.searchGate
	var proxyAddress string
	onProxy := func(stage string, kind StatusKind) func(string) {
		connectionAttempt := 0
		return func(address string) {
			connectionAttempt++
			// Fallback is another network request, so it must pass the same
			// stage gate rather than bypass audio spacing or shared cooldowns.
			if connectionAttempt > 1 {
				generation = d.waitForRequest(gate, jobID, name, kind)
			}
			proxyAddress = ProxyLabel(address)
			connection := "proxy"
			if proxyAddress == "direct" {
				connection = "direct"
			}
			logger.Info("request_started", zap.String("stage", stage), zap.String("proxy", proxyAddress), zap.String("connection", connection), zap.Int("connection_attempt", connectionAttempt))
			if d.display != nil {
				d.display.Update(StatusEvent{JobID: jobID, Source: d.provider.Name(), Kind: kind, Name: name, Proxy: proxyAddress, TotalBytes: -1})
			} else {
				if connection == "direct" {
					fmt.Printf("%s: %s · напрямую\n", stage, name)
				} else {
					fmt.Printf("%s: %s · прокси: %s\n", stage, name, proxyAddress)
				}
			}
		}
	}
	fail := func(stage string, err error, details ...zap.Field) {
		fields := append(details, zap.String("stage", stage), zap.String("error", safeError(err)), zap.Duration("duration", time.Since(started)))
		if proxyAddress != "" {
			fields = append(fields, zap.String("proxy", proxyAddress))
		}
		var httpErr *HTTPError
		if errors.As(err, &httpErr) {
			fields = append(fields, zap.Int("http_status", httpErr.StatusCode), zap.String("http_stage", httpErr.Stage))
		}
		if stage == "search" && errors.Is(err, ErrNotFound) {
			logger.Warn("not_found", fields...)
			d.sendEvent(StatusNotFound, jobID, name, err)
		} else if httpErr != nil && httpErr.StatusCode == 429 {
			retryNeeded, rateLimited = true, true
			delay, changed := gate.block(generation)
			if changed {
				if d.display != nil {
					pauseStage := "download"
					if gate == d.searchGate {
						pauseStage = "search"
					}
					d.display.SetRateLimitPause(pauseStage, time.Now().Add(delay))
				}
				logger.Warn("rate_limit_pause", zap.String("stage", stage), zap.Duration("delay", delay), zap.Int("http_status", 429))
			}
			logger.Warn("retry_scheduled", append(fields, zap.Int("retry", attempt+1), zap.Duration("delay", delay))...)
			d.sendEvent(StatusRetryWaiting, jobID, name, err)
		} else if retry < maxRetries {
			retryNeeded = true
			logger.Warn("retry_scheduled", append(fields, zap.Int("retry", retry+1), zap.Duration("delay", d.retryDelay(retry)))...)
			d.sendEvent(StatusRetryWaiting, jobID, name, err)
		} else {
			logger.Error("failed", fields...)
			d.sendEvent(StatusFailed, jobID, name, err)
		}
	}
	logger.Info("searching")
	generation = d.waitForRequest(gate, jobID, query, StatusSearching)
	var track Track
	var err error
	if provider, ok := d.provider.(ProxyAwareProvider); ok {
		track, err = provider.SearchWithProxy(query, onProxy("search", StatusSearching))
	} else {
		track, err = d.provider.Search(query)
	}
	if err != nil {
		fail("search", err)
		return
	}

	name = track.Artist + " — " + track.Title
	logger = logger.With(zap.String("track_id", track.ID), zap.String("name", name))
	logger.Info("search_finished", zap.Duration("duration", time.Since(started)), zap.String("proxy", proxyAddress))
	d.sendEvent(StatusChecking, jobID, name, nil)
	filePath, err := filepath.Abs(filepath.Join(d.outputDir, audioFilename(track)))
	if err != nil {
		fail("path", err)
		return
	}
	logger = logger.With(zap.String("file_path", filePath))
	logger.Info("checking_hash")
	// Different playlist entries can resolve to the same provider track.
	// Check and download under one lock so they cannot overwrite each other.
	lock, _ := d.fileLocks.LoadOrStore(filePath, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	verified, err := d.alreadyDownloaded(track.ID, filePath)
	if err == nil && !verified {
		verified, err = d.migrateLegacyFile(track.ID, filePath, logger)
	}
	if err != nil {
		fail("check_hash", fmt.Errorf("проверка скачанного файла: %w", err))
		return
	}
	if verified {
		logger.Info("skipped", zap.Duration("duration", time.Since(started)))
		d.sendEvent(StatusSkipped, jobID, name, nil)
		return
	}
	logger.Info("preparing")
	proxyAddress = "" // A download has not selected a route while it waits for admission.
	gate = d.downloadGate
	generation = d.waitForRequest(gate, jobID, name, StatusPreparing)

	var resp DownloadResponse
	if provider, ok := d.provider.(ProgressAwareProvider); ok {
		var lastProgress time.Time
		resp, err = provider.DownloadWithProgress(track, onProxy("download", StatusDownloading), func(progress DownloadProgress) {
			// Do not flood non-interactive output; the live display redraws separately.
			now := time.Now()
			if !progress.Converting && now.Sub(lastProgress) < time.Second {
				return
			}
			lastProgress = now
			kind := StatusDownloading
			if progress.Converting {
				kind = StatusConverting
			}
			event := StatusEvent{JobID: jobID, Source: d.provider.Name(), Kind: kind, Name: name, Proxy: proxyAddress,
				DownloadedBytes: progress.DownloadedBytes, TotalBytes: progress.TotalBytes, BytesPerSecond: progress.BytesPerSecond, ETA: progress.ETA}
			if d.display != nil {
				d.display.Update(event)
			} else {
				fmt.Printf("%s · %s\n", transferStatus(event), name)
			}
		})
	} else if provider, ok := d.provider.(ProxyAwareProvider); ok {
		resp, err = provider.DownloadWithProxy(track, onProxy("download", StatusDownloading))
	} else {
		resp, err = d.provider.Download(track)
	}
	if err != nil {
		fail("prepare", err)
		return
	}

	// Only replace the destination after a complete, successfully closed copy.
	f, err := os.CreateTemp(filepath.Dir(filePath), ".download-*.tmp")
	if err != nil {
		resp.Body.Close()
		fail("create_file", err)
		return
	}
	defer os.Remove(f.Name())
	defer f.Close()

	// Шлём начальное событие старта скачивания
	d.sendEventDownloading(jobID, name, resp.ContentLength, 0, proxyAddress)
	logger.Info("downloading", zap.Int64("expected_bytes", resp.ContentLength), zap.String("proxy", proxyAddress))

	pr := newProgressReader(resp.Body, resp.ContentLength,
		func(downloaded, total int64) {
			d.sendEventDownloading(jobID, name, total, downloaded, proxyAddress)
		})

	hash := sha256.New()
	failureStage := "downloading"
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
		failureStage = "saving"
		logger.Info("saving", zap.Int64("bytes", written))
		d.sendEvent(StatusSaving, jobID, name, nil)
		err = os.Rename(f.Name(), filePath)
	}
	if err == nil && d.store != nil {
		err = d.store.SaveHash(d.provider.Name(), track.ID, filePath, fmt.Sprintf("%x", hash.Sum(nil)))
	}

	if err != nil {
		fail(failureStage, err, zap.Int64("bytes", written), zap.Int64("expected_bytes", resp.ContentLength))
		return
	}

	logger.Info("downloaded", zap.Int64("bytes", written), zap.String("sha256", fmt.Sprintf("%x", hash.Sum(nil))), zap.Duration("duration", time.Since(started)))
	d.sendEvent(StatusDone, jobID, name, nil)
	return
}

// sendEventDownloading отправляет событие скачивания с прогрессом.
func (d *Downloader) sendEventDownloading(jobID, name string, total, downloaded int64, proxy ...string) {
	if d.display == nil {
		return
	}
	address := ""
	if len(proxy) > 0 {
		address = ProxyLabel(proxy[0])
	}
	d.display.Update(StatusEvent{
		JobID:           jobID,
		Kind:            StatusDownloading,
		Source:          d.provider.Name(),
		Name:            name,
		TotalBytes:      total,
		DownloadedBytes: downloaded,
		Proxy:           address,
	})
}
