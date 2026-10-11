package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
)

// HTTPError is a failed request, not a successful search with no matches.
type HTTPError struct {
	Stage      string
	StatusCode int
	// ProxyCooldown means the failed proxy was paused in the shared pool.
	// Other proxies may still accept requests; do not pause all workers.
	ProxyCooldown bool
}

func (e *HTTPError) Error() string { return fmt.Sprintf("%s: HTTP %d", e.Stage, e.StatusCode) }

func safeError(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Sprintf("%s: %s", urlErr.Op, safeHTTPFailure(urlErr.Err))
	}
	return err.Error()
}

// HTTP client errors may contain an untrusted redirect URL or server bytes.
func safeHTTPFailure(err error) string {
	var networkErr net.Error
	var dnsErr *net.DNSError
	var opErr *net.OpError
	switch {
	case errors.Is(err, context.Canceled):
		return "request canceled"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &networkErr) && networkErr.Timeout():
		return "request timeout"
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return "unexpected EOF"
	case errors.As(err, &dnsErr):
		return "DNS lookup failed"
	case errors.As(err, &opErr):
		return fmt.Sprintf("network %s failed", opErr.Op)
	case strings.Contains(err.Error(), "failed to parse Location header"):
		return "invalid redirect Location"
	case strings.Contains(err.Error(), "malformed HTTP response"):
		return "malformed HTTP response"
	default:
		return fmt.Sprintf("HTTP client error (%T)", err)
	}
}
