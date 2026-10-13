package spotify

import (
	"regexp"
	"strings"
	"unicode"

	"go.uber.org/zap"
)

var diagnosticSecrets = regexp.MustCompile(`(?i)(?:https?://|socks[45]h?://)[^\s]+|bearer\s+[^\s,;]+|(?:authorization|client-token|cookie|access_token|refresh_token|password)\s*[:=].*|[a-z0-9_-]{24,}\.[a-z0-9_-]+\.[a-z0-9_-]+|[a-z0-9+/=_-]{48,}|[^\s@]+@[^\s@]+`)

func spotifyLogger(loggers []*zap.Logger) *zap.Logger {
	if len(loggers) > 0 && loggers[0] != nil {
		return loggers[0]
	}
	return zap.L()
}

// Redact the exact request tokens before shortening or normalizing text. Also
// mask URLs, credential assignments and opaque token-like strings in messages.
func diagnosticText(text string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[redacted]")
		}
		if token := strings.TrimPrefix(secret, "Bearer "); token != secret && token != "" {
			text = strings.ReplaceAll(text, token, "[redacted]")
		}
	}
	text = diagnosticSecrets.ReplaceAllString(text, "[redacted]")
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.ToValidUTF8(text, "�"))
	runes := []rune(text)
	if len(runes) > 500 {
		text = string(runes[:500]) + "…"
	}
	return text
}
