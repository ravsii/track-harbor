package youtube

import (
	"context"
	"errors"
	"fmt"
	"net"

	"meshvault/internal/download"
)

// DownloadStrategy controls the connection used by a search or audio request.
// An empty proxy URL means an explicitly direct connection.
type DownloadStrategy interface {
	Name() string
	Execute(context.Context, func(proxyURL string) error) error
}

type Direct struct{}

func (Direct) Name() string { return "direct" }
func (Direct) Execute(ctx context.Context, request func(string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return request("")
}

type ProxyOnly struct {
	Pool        ProxySelector
	Unavailable error
}

func (ProxyOnly) Name() string { return "proxy" }
func (s ProxyOnly) Execute(ctx context.Context, request func(string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.Unavailable != nil {
		return fmt.Errorf("proxy pool unavailable: %w", s.Unavailable)
	}
	if s.Pool == nil {
		return fmt.Errorf("proxy pool unavailable; direct connection forbidden")
	}
	var address string
	var err error
	if pool, ok := s.Pool.(interface {
		NextContext(context.Context) (string, error)
	}); ok {
		address, err = pool.NextContext(ctx)
	} else {
		address, err = s.Pool.Next()
	}
	if err != nil {
		return fmt.Errorf("selecting proxy: %w", err)
	}
	if address == "" {
		return fmt.Errorf("empty proxy; direct connection forbidden")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	err = request(address)
	var httpErr *download.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == 429 {
		if pool, ok := s.Pool.(interface{ ReportRateLimit(string) }); ok {
			pool.ReportRateLimit(address)
		}
	}
	return err
}

type DirectWithProxyFallback struct {
	Proxy      DownloadStrategy
	OnFallback func(error)
}

func (DirectWithProxyFallback) Name() string { return "fallback" }
func (s DirectWithProxyFallback) Execute(ctx context.Context, request func(string) error) error {
	err := (Direct{}).Execute(ctx, request)
	if err == nil || ctx.Err() != nil || !canFallback(err) {
		return err
	}
	if s.OnFallback != nil {
		s.OnFallback(err)
	}
	if s.Proxy == nil {
		return fmt.Errorf("proxy fallback unavailable: %w", err)
	}
	if proxyErr := s.Proxy.Execute(ctx, request); proxyErr != nil {
		return fmt.Errorf("proxy fallback after %v: %w", err, proxyErr)
	}
	return nil
}

func canFallback(err error) bool {
	if errors.Is(err, download.ErrNotFound) || errors.Is(err, context.Canceled) {
		return false
	}
	var httpErr *download.HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == 403 || httpErr.StatusCode == 429
	}
	var networkErr net.Error
	return errors.Is(err, context.DeadlineExceeded) || errors.As(err, &networkErr) && networkErr.Timeout()
}
