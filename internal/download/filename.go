package download

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"unicode"
)

func audioFilename(track Track) string {
	id := safeFilePart(track.ID)
	if id == "" {
		id = fmt.Sprintf("%x", sha256.Sum256([]byte(track.ID)))
	}
	id = limitFilenamePart(id, 64)
	artist, title := safeFilePart(track.Artist), safeFilePart(track.Title)
	if artist == "" {
		artist = "Неизвестный исполнитель"
	}
	if title == "" {
		title = "Без названия"
	}
	suffix := " [" + id + "].mp3"
	// Leave room for both names, the stable ID and the extension. The bound
	// is in bytes, not runes, to fit typical filesystem component limits.
	budget := 240 - len(suffix) - len(" — ")
	artist = limitFilenamePart(artist, budget/2)
	title = limitFilenamePart(title, budget-len(artist))
	return artist + " — " + title + suffix
}

func safeFilePart(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`/\:*?"<>|`, r) {
			return ' '
		}
		return r
	}, s)
	return strings.Trim(strings.Join(strings.Fields(s), " "), " .")
}

func limitFilenamePart(s string, bytes int) string {
	if len(s) <= bytes {
		return s
	}
	end := 0
	for i, r := range s {
		if i+len(string(r)) > bytes {
			break
		}
		end = i + len(string(r))
	}
	return strings.TrimRight(s[:end], " .")
}
