package cli

import (
	"fmt"
	"os"
)

func prepareDownloadDirectory(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("create download directory %q: %w", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("check download directory %q: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("download path %q is not a directory", path)
	}
	// Actually attempt a write: permission bits alone are unreliable across OSes.
	f, err := os.CreateTemp(path, ".trackharbor-write-check-*")
	if err != nil {
		return fmt.Errorf("download directory %q is not writable: %w", path, err)
	}
	name := f.Name()
	closeErr := f.Close()
	removeErr := os.Remove(name)
	if closeErr != nil {
		return fmt.Errorf("check write access to %q: %w", path, closeErr)
	}
	if removeErr != nil {
		return fmt.Errorf("clean up write check in %q: %w", path, removeErr)
	}
	return nil
}
