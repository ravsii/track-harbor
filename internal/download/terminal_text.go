package download

import (
	"strings"

	"github.com/clipperhouse/uax29/v2/graphemes"
	"github.com/mattn/go-runewidth"
)

// Reserve cells for emoji even on terminals which do not compose ZWJ/flag
// sequences into one glyph. This may shorten a title more than necessary, but
// avoids wrapping a live-display line into an untracked extra terminal row.
var terminalRuneWidth = func() *runewidth.Condition {
	c := runewidth.NewCondition()
	c.StrictEmojiNeutral = false
	return c
}()

func terminalClusterWidth(cluster string) int {
	width := 0
	for _, r := range cluster {
		width += terminalRuneWidth.RuneWidth(r)
	}
	// Emoji presentation and keycaps can turn a narrow base into a wide glyph.
	if strings.ContainsAny(cluster, "\ufe0f\u20e3") {
		width = max(2, width)
	}
	return width
}

func terminalTextWidth(text string) int {
	width := 0
	g := graphemes.FromString(text)
	for g.Next() {
		width += terminalClusterWidth(g.Value())
	}
	return width
}

// Cut only at grapheme boundaries: neither UTF-8 bytes nor a combining/emoji
// sequence may be split. Width is measured in terminal cells, not rune count.
func truncateTerminalText(text string, width int, tail string) string {
	if width <= 0 {
		return ""
	}
	if terminalTextWidth(text) <= width {
		return text
	}
	budget := width - terminalTextWidth(tail)
	if budget < 0 {
		return ""
	}
	used, end := 0, 0
	g := graphemes.FromString(text)
	for g.Next() {
		cells := terminalClusterWidth(g.Value())
		if used+cells > budget {
			break
		}
		used += cells
		end = g.End()
	}
	return text[:end] + tail
}
