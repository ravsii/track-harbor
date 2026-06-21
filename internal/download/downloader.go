package download

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sync"
	"time"
)

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

	reader, err := d.provider.Download(track)
	if err != nil {
		d.sendEvent(StatusFailed, name, err)
		return
	}

	data, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		d.sendEvent(StatusFailed, name, err)
		return
	}

	err = os.WriteFile(path.Join(d.outputDir, track.ID+".mp3"), data, 0o666)
	if err != nil {
		d.sendEvent(StatusFailed, name, err)
		return
	}

	d.sendEvent(StatusDone, name, nil)
}
