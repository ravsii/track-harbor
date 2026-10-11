package download

import (
	"errors"
	"io"
	"time"
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
	// Name — уникальное имя провайдера (например "youtube").
	Name() string

	// Search — поиск лучшего трека по строке запроса.
	// Возвращает ErrNotFound, если ничего не найдено.
	Search(query string) (Track, error)

	// Download — скачивание трека, возвращает DownloadResponse с телом и размером.
	Download(track Track) (DownloadResponse, error)
}

// ProxyAwareProvider reports the selected route synchronously for this exact
// invocation, before starting the connection. Provider stays compatible with
// implementations that do not expose connection diagnostics.
type ProxyAwareProvider interface {
	SearchWithProxy(query string, onProxy func(string)) (Track, error)
	DownloadWithProxy(track Track, onProxy func(string)) (DownloadResponse, error)
}

// DownloadProgress describes network transfer, not the later local file copy.
type DownloadProgress struct {
	DownloadedBytes int64
	TotalBytes      int64 // May be an estimate; -1 means unknown.
	BytesPerSecond  float64
	ETA             time.Duration
	Converting      bool
}

// ProgressAwareProvider streams progress for this invocation only.
type ProgressAwareProvider interface {
	DownloadWithProgress(Track, func(string), func(DownloadProgress)) (DownloadResponse, error)
}
