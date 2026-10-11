package youtube

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

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

// RandomProxy uses the discovered public pool; its selection remains LRU.
type RandomProxy struct{ ProxyOnly }

func (RandomProxy) Name() string { return "random_proxy" }

// ConfigProxy tries personal proxies in configuration order. These addresses
// are trusted user input and are neither publicly discovered nor pre-probed.
type ConfigProxy struct{ Addresses []string }

func (ConfigProxy) Name() string { return "config_proxy" }

func (s ConfigProxy) Execute(ctx context.Context, request func(string) error) error {
	if len(s.Addresses) == 0 {
		return fmt.Errorf("config_proxy: no personal proxies configured")
	}
	var lastErr error
	for _, address := range s.Addresses {
		if err := ctx.Err(); err != nil {
			return err
		}
		if address == "" {
			return fmt.Errorf("config_proxy: empty proxy; direct connection forbidden")
		}
		lastErr = request(address)
		if lastErr == nil || errors.Is(lastErr, download.ErrNotFound) || errors.Is(lastErr, context.Canceled) {
			return lastErr
		}
	}
	return fmt.Errorf("config_proxy: personal proxies exhausted: %w", lastErr)
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

// Priority tries each configured route in order for every operation.
// The last error remains unwrap-able for the downloader's retry policy.
type Priority struct {
	Strategies []DownloadStrategy
	OnFallback func(from, to string, err error)
}

func (s Priority) Name() string {
	names := make([]string, 0, len(s.Strategies))
	for _, strategy := range s.Strategies {
		names = append(names, strategy.Name())
	}
	return strings.Join(names, " -> ")
}

func (s Priority) Execute(ctx context.Context, request func(string) error) error {
	if len(s.Strategies) == 0 {
		return fmt.Errorf("no download strategies configured")
	}
	var lastErr error
	for i, strategy := range s.Strategies {
		if err := ctx.Err(); err != nil {
			return err
		}
		if i > 0 && s.OnFallback != nil {
			s.OnFallback(s.Strategies[i-1].Name(), strategy.Name(), lastErr)
		}
		lastErr = strategy.Execute(ctx, request)
		if lastErr == nil || errors.Is(lastErr, download.ErrNotFound) || errors.Is(lastErr, context.Canceled) {
			return lastErr
		}
	}
	return fmt.Errorf("strategies %s exhausted: %w", s.Name(), lastErr)
}
