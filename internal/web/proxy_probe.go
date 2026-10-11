package web

import (
	"context"
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

// probePersonalProxy verifies HTTPS access through the supplied proxy, not
// direct network access. Never include transport errors in responses or logs:
// proxy authentication data can appear in their text.
func probePersonalProxy(ctx context.Context, address string) error {
	return probeYouTube(ctx, address, 6*time.Second)
}

func probeYouTube(ctx context.Context, address string, timeout time.Duration) error {
	u, err := url.Parse(address)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	transport := &http.Transport{
		Proxy:                 http.ProxyURL(u),
		DialContext:           (&net.Dialer{Timeout: timeout}).DialContext,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		DisableKeepAlives:     true,
	}
	if u.Scheme == "socks4" || u.Scheme == "socks4a" {
		transport.Proxy = nil
		transport.DialContext = func(ctx context.Context, _, target string) (net.Conn, error) {
			return dialSOCKS4(ctx, u, target)
		}
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.youtube.com/robots.txt", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("YouTube returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return err
	}
	if !strings.Contains(strings.ToLower(string(body)), "user-agent:") {
		return fmt.Errorf("unexpected YouTube response")
	}
	return nil
}

func dialSOCKS4(ctx context.Context, proxy *url.URL, target string) (net.Conn, error) {
	host, portText, err := net.SplitHostPort(target)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid destination port")
	}
	var ip [4]byte
	if proxy.Scheme == "socks4a" {
		ip[3] = 1
	} else {
		resolved, err := net.DefaultResolver.LookupNetIP(ctx, "ip4", host)
		if err != nil || len(resolved) == 0 {
			return nil, fmt.Errorf("destination lookup failed")
		}
		ip = resolved[0].As4()
	}
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", proxy.Host)
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}
	request := []byte{4, 1, 0, 0, ip[0], ip[1], ip[2], ip[3]}
	binary.BigEndian.PutUint16(request[2:4], uint16(port))
	if proxy.User != nil {
		request = append(request, []byte(proxy.User.Username())...)
	}
	request = append(request, 0)
	if proxy.Scheme == "socks4a" {
		request = append(request, []byte(host)...)
		request = append(request, 0)
	}
	if _, err := conn.Write(request); err != nil {
		return nil, err
	}
	var response [8]byte
	if _, err := io.ReadFull(conn, response[:]); err != nil {
		return nil, err
	}
	if response[1] != 90 {
		return nil, fmt.Errorf("SOCKS4 connection rejected")
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
	result := conn
	conn = nil
	return result, nil
}
