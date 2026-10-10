package download

import (
	"net/url"
	"strings"
)

// ProxyLabel never exposes proxy credentials, query parameters or URL paths.
func ProxyLabel(address string) string {
	if address == "" {
		return ""
	}
	if !strings.Contains(address, "://") {
		address = "http://" + address
	}
	u, err := url.Parse(address)
	if err != nil || u.Hostname() == "" {
		return "неизвестный"
	}
	switch u.Scheme {
	case "http", "https", "socks4", "socks5", "socks5h":
	default:
		return "неизвестный"
	}
	return u.Scheme + "://" + u.Host
}
