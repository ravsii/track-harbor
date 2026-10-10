package download

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
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
