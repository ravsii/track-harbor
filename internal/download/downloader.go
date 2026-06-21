package download

import (
	"fmt"
	"io"
	"os"
	"path"
	"sync"
)

// Downloader — оркестратор загрузок с worker pool.
type Downloader struct {
	provider  Provider
	workers   int
	outputDir string
}

// New создаёт Downloader.
func New(provider Provider, workers int, outputDir string) *Downloader {
	return &Downloader{provider: provider, workers: workers, outputDir: outputDir}
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
}

func (d *Downloader) downloadQuery(query string) {
	fmt.Println(query)

	tracks, err := d.provider.Search(query)
	if err != nil {
		fmt.Println("err search:", err)
		return
	}

	if len(tracks) == 0 {
		fmt.Println("no results for", query)
		return
	}

	for _, track := range tracks {
		reader, err := d.provider.Download(track)
		if err != nil {
			fmt.Println("err download:", err)
			continue
		}

		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			fmt.Println("err read body:", err)
			continue
		}

		err = os.WriteFile(path.Join(d.outputDir, track.ID+".mp3"), data, 0o666)
		if err != nil {
			fmt.Println("err write:", err)
			continue
		}
	}
}
