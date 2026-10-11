package cli

import (
	"fmt"
	"os"
)

func prepareDownloadDirectory(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("создание папки скачивания %q: %w", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("проверка папки скачивания %q: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("путь скачивания %q не является папкой", path)
	}
	// Actually attempt a write: permission bits alone are unreliable across OSes.
	f, err := os.CreateTemp(path, ".meshvault-write-check-*")
	if err != nil {
		return fmt.Errorf("папка скачивания %q недоступна для записи: %w", path, err)
	}
	name := f.Name()
	closeErr := f.Close()
	removeErr := os.Remove(name)
	if closeErr != nil {
		return fmt.Errorf("проверка записи в %q: %w", path, closeErr)
	}
	if removeErr != nil {
		return fmt.Errorf("очистка проверки записи в %q: %w", path, removeErr)
	}
	return nil
}
