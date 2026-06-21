package download

import "io"

// Track — общий результат поиска, понятный любому провайдеру загрузки.
type Track struct {
	ID       string
	Artist   string
	Title    string
	Duration string
	Size     string
	Bitrate  string
}

// Provider — интерфейс, который должна реализовать каждая площадка-источник.
type Provider interface {
	// Name — уникальное имя провайдера (например "mp3party").
	Name() string

	// Search — поиск треков по строке запроса.
	Search(query string) ([]Track, error)

	// Download — скачивание трека, возвращает io.ReadCloser аудио-данных.
	// Вызывающий закрывает ReadCloser после использования.
	Download(track Track) (io.ReadCloser, error)
}
