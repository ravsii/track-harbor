package download

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// DownloadStore persists verified file hashes, keyed by provider, track and path.
type DownloadStore interface {
	LookupHash(source, trackID, filePath string) (string, error)
	SaveHash(source, trackID, filePath, hash string) error
}

func (d *Downloader) WithDownloadStore(store DownloadStore) *Downloader {
	d.store = store
	return d
}

func (d *Downloader) alreadyDownloaded(trackID, filePath string) (bool, error) {
	if d.store == nil {
		return false, nil
	}
	expected, err := d.store.LookupHash(d.provider.Name(), trackID, filePath)
	if err != nil || expected == "" {
		return false, err
	}
	f, err := os.Open(filePath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return false, err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)) == expected, nil
}

// Adopt only previously tracked, hash-verified ID-named files. Never overwrite
// a target while migrating, and retain the legacy file if saving history fails.
func (d *Downloader) migrateLegacyFile(trackID, target string, logger *zap.Logger) (bool, error) {
	if d.store == nil || trackID == "" || strings.ContainsAny(trackID, `/\`) {
		return false, nil
	}
	legacy := filepath.Join(filepath.Dir(target), trackID+".mp3")
	if legacy == target {
		return false, nil
	}
	lock, _ := d.fileLocks.LoadOrStore(legacy, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	if _, err := os.Lstat(target); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	verified, err := d.alreadyDownloaded(trackID, legacy)
	if err != nil || !verified {
		return false, err
	}
	hash, err := d.store.LookupHash(d.provider.Name(), trackID, legacy)
	if err != nil {
		return false, err
	}
	// Link+unlink is a same-directory move that cannot clobber an existing file.
	if err := os.Link(legacy, target); err != nil {
		return false, err
	}
	if err := d.store.SaveHash(d.provider.Name(), trackID, target, hash); err != nil {
		return false, errors.Join(err, os.Remove(target))
	}
	if err := os.Remove(legacy); err != nil {
		return false, err
	}
	logger.Info("file_renamed", zap.String("previous_file_path", legacy), zap.String("file_path", target), zap.String("sha256", hash))
	return true, nil
}
