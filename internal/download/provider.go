package download

import (
	"errors"
	"io"
)

// ErrNotFound — провайдер не нашёл подходящий трек.
var ErrNotFound = errors.New("track not found")

// Track — общий результат поиска, понятный любому провайдеру загрузки.
type Track struct {
	ID       string
	Artist   string
	Title    string
	Duration string
	Size     string
	Bitrate  string
}

// DownloadResponse — результат скачивания трека.
type DownloadResponse struct {
	Body          io.ReadCloser
	ContentLength int64 // -1, если неизвестен
}

// Provider — интерфейс, который должна реализовать каждая площадка-источник.
type Provider interface {
	// Name — уникальное имя провайдера (например "mp3party").
	Name() string

	// Search — поиск лучшего трека по строке запроса.
	// Возвращает ErrNotFound, если ничего не найдено.
	Search(query string) (Track, error)

	// Download — скачивание трека, возвращает DownloadResponse с телом и размером.
	Download(track Track) (DownloadResponse, error)
}
