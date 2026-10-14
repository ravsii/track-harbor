package download

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var sourceName = regexp.MustCompile(`^[a-z0-9_-]+$`)

// FilePath is shared by migration, downloading and playback. Encoding preserves
// case-sensitive keys on case-insensitive filesystems and prevents path traversal.
func FilePath(root, source, key string) (string, error) {
	if !sourceName.MatchString(source) || key == "" || len(key) > 512 {
		return "", fmt.Errorf("invalid track source or key")
	}
	name := hex.EncodeToString([]byte(key))
	if source == "vk" && vkAudioID.MatchString(key) {
		name = key
	}
	return filepath.Join(root, source, name+".mp3"), nil
}

func FileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}
