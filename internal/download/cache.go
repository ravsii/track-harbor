package download

import (
	"errors"
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

// QueryDownloadStore links playlist queries to successful downloads so repeat
// runs can skip both provider search and transfer. Older stores stay compatible.
type QueryDownloadStore interface {
	LookupQueryPaths(source, query string) ([]string, error)
	SaveQuery(source, query, trackID, filePath string) error
}

func (d *Downloader) downloadedQuery(query string) (string, error) {
	store, ok := d.store.(QueryDownloadStore)
	if !ok {
		return "", nil
	}
	return DownloadedQueryPath(store, d.provider.Name(), query, d.outputDir)
}

// DownloadedQueryPath checks history locally without initializing a provider.
func DownloadedQueryPath(store QueryDownloadStore, source, query, outputDir string) (string, error) {
	paths, err := store.LookupQueryPaths(source, query)
	if err != nil {
		return "", err
	}
	directory, err := filepath.Abs(outputDir)
	if err != nil {
		return "", err
	}
	for _, path := range paths {
		// History from another output directory must not suppress this download.
		if filepath.Dir(path) != directory {
			continue
		}
		present, err := nonemptyFile(path)
		if err != nil {
			return "", err
		}
		if present {
			return path, nil
		}
	}
	return "", nil
}

func (d *Downloader) saveQuery(query, trackID, filePath string) error {
	if store, ok := d.store.(QueryDownloadStore); ok {
		return store.SaveQuery(d.provider.Name(), query, trackID, filePath)
	}
	return nil
}

func (d *Downloader) WithDownloadStore(store DownloadStore) *Downloader {
	d.store = store
	return d
}

func (d *Downloader) alreadyDownloaded(trackID, filePath string) (bool, error) {
	return nonemptyFile(filePath)
}

func nonemptyFile(filePath string) (bool, error) {
	info, err := os.Stat(filePath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.Mode().IsRegular() && info.Size() > 0, nil
}

// Adopt only previously tracked, non-empty ID-named files. Never overwrite
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
	if hash == "" {
		return false, nil
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
