package logger

import (
	"os"
	"path/filepath"
	"time"
)

// dailyWriter keeps the file selected by the local date at initialization.
// It does not switch files while the process is running.
type dailyWriter struct {
	file *os.File
}

func newWriter(directory string) (dailyWriter, error) {
	name := time.Now().Format("2006-01-02")
	file, err := os.OpenFile(filepath.Join(directory, name+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return dailyWriter{}, err
	}

	return dailyWriter{file: file}, nil
}

func (w *dailyWriter) Write(data []byte) (int, error) {
	return w.file.Write(data)
}

func (w *dailyWriter) Sync() error {
	return w.file.Sync()
}

func (w *dailyWriter) Close() error {
	return w.file.Close()
}
