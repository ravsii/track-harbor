package download

import (
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
		time.Sleep(100 * time.Millisecond) // даём последним событиям долететь
		d.display.Stop()
	}
}

func (d *Downloader) sendEvent(kind StatusKind, name string, err error) {
	if d.display == nil {
		// fallback: обычный лог
		switch kind {
		case StatusSearching:
			fmt.Println(name)
		case StatusDownloading:
			// ничего не пишем в простом режиме
		case StatusDone:
			// ничего
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
	source := d.provider.Name()

	if d.display == nil {
		fmt.Println(query)
	} else {
		d.display.Update(StatusEvent{Kind: StatusSearching, Source: source, Name: query})
	}

	tracks, err := d.provider.Search(query)
	if err != nil {
		if d.display == nil {
			fmt.Println("err search:", err)
		} else {
			d.display.Update(StatusEvent{Kind: StatusFailed, Source: source, Name: query, Err: err})
		}
		return
	}

	if len(tracks) == 0 {
		if d.display == nil {
			fmt.Println("no results for", query)
		} else {
			d.display.Update(StatusEvent{Kind: StatusFailed, Source: source, Name: query, Err: fmt.Errorf("no results")})
		}
		return
	}

	for _, track := range tracks {
		name := track.Artist + " — " + track.Title

		if d.display == nil {
			// в простом режиме пропускаем
		} else {
			d.display.Update(StatusEvent{Kind: StatusDownloading, Source: source, Name: name})
		}

		reader, err := d.provider.Download(track)
		if err != nil {
			d.sendEvent(StatusFailed, name, err)
			continue
		}

		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			d.sendEvent(StatusFailed, name, err)
			continue
		}

		err = os.WriteFile(path.Join(d.outputDir, track.ID+".mp3"), data, 0o666)
		if err != nil {
			d.sendEvent(StatusFailed, name, err)
			continue
		}

		d.sendEvent(StatusDone, name, nil)
	}
}
