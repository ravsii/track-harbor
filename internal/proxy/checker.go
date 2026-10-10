package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HTTPSChecker struct {
	target    string
	timeout   time.Duration
	tlsConfig *tls.Config
}

func NewHTTPSChecker() *HTTPSChecker {
	return &HTTPSChecker{target: "https://www.youtube.com/robots.txt", timeout: 8 * time.Second}
}

func (c *HTTPSChecker) Check(ctx context.Context, address string) error {
	u, err := url.Parse(address)
	if err != nil {
		return err
	}
	transport := &http.Transport{
		Proxy: http.ProxyURL(u), DialContext: (&net.Dialer{Timeout: c.timeout}).DialContext,
		TLSClientConfig: c.tlsConfig, TLSHandshakeTimeout: c.timeout, ResponseHeaderTimeout: c.timeout,
		DisableKeepAlives: true,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: c.timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.target, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("probe HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return err
	}
	if !strings.Contains(strings.ToLower(string(body)), "user-agent:") {
		return fmt.Errorf("unexpected YouTube robots response")
	}
	return nil
}
