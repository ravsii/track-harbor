package proxy

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type HTTPSChecker struct {
	target    string
	timeout   time.Duration
	tlsConfig *tls.Config
}

func NewHTTPSChecker() *HTTPSChecker {
	return &HTTPSChecker{target: "https://www.youtube.com/robots.txt", timeout: time.Second}
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
	if u.Scheme == "socks4" {
		transport.Proxy = nil
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialSOCKS4(ctx, u.Host, address, c.timeout)
		}
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

// SOCKS4 uses an IPv4 destination, like yt-dlp's socks4:// routing. The
// handshake is bounded and cancellation closes the connection immediately.
func dialSOCKS4(ctx context.Context, proxyAddress, target string, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	host, portText, err := net.SplitHostPort(target)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip4", host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("SOCKS4 destination has no IPv4 address")
	}
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", proxyAddress)
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			conn.Close()
		}
	}()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	conn.SetDeadline(deadline)
	request := make([]byte, 9)
	request[0] = 4
	request[1] = 1
	binary.BigEndian.PutUint16(request[2:4], uint16(port))
	ip := ips[0].As4()
	copy(request[4:8], ip[:])
	if _, err := conn.Write(request); err != nil {
		return nil, err
	}
	var response [8]byte
	if _, err := io.ReadFull(conn, response[:]); err != nil {
		return nil, err
	}
	if response[0] != 0 || response[1] != 90 {
		return nil, fmt.Errorf("SOCKS4 CONNECT rejected: %d", response[1])
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !stop() {
		return nil, ctx.Err()
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return nil, err
	}
	keep = true
	return conn, nil
}
