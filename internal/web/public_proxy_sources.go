package web

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const maxProxySourceAddresses = 500000

func publicProxyIP(ip netip.Addr) bool {
	if ip.Zone() != "" {
		return false
	}
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range []string{"0.0.0.0/8", "100.64.0.0/10", "198.18.0.0/15"} {
		if netip.MustParsePrefix(prefix).Contains(ip) {
			return false
		}
	}
	return true
}

// Only probe literal public IPs from third-party lists, never local services.
func discoveredProxy(raw, protocol string) (string, bool) {
	if !strings.Contains(raw, "://") {
		raw = protocol + "://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", false
	}
	switch u.Scheme {
	case "http", "https", "socks4", "socks4a", "socks5", "socks5h":
	default:
		return "", false
	}
	ip, err := netip.ParseAddr(u.Hostname())
	if err != nil || !publicProxyIP(ip) {
		return "", false
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return "", false
	}
	return u.Scheme + "://" + net.JoinHostPort(ip.Unmap().String(), strconv.Itoa(port)), true
}

func validateProxySource(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || len(endpoint) > 2048 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("use HTTPS links to public text files, without credentials or fragments")
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("source URL has an invalid port")
		}
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !publicProxyIP(ip) {
		return fmt.Errorf("proxy sources must use public network addresses")
	}
	return nil
}

// Validate the actual dial destination as well as the URL, including redirects
// and DNS changes. Source downloads must never reach loopback/private services.
func dialProxySource(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if !publicProxyIP(ip) {
			continue
		}
		conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
	}
	return nil, fmt.Errorf("public proxy source unavailable")
}

// Explicit proxy URLs retain their protocol. Bare IP:PORT entries use the
// protocol in the source query/filename, falling back to SOCKS5 for other files.
func proxySourceProtocol(endpoint string) string {
	u, _ := url.Parse(endpoint)
	protocol := u.Query().Get("protocol")
	if protocol == "" {
		protocol = strings.TrimSuffix(strings.ToLower(path.Base(u.Path)), ".txt")
	}
	switch protocol {
	case "http", "https", "socks4", "socks4a", "socks5", "socks5h":
		return protocol
	default:
		return "socks5"
	}
}

func fetchProxyLines(ctx context.Context, client *http.Client, endpoint string, limit int64) ([]string, error) {
	if err := validateProxySource(endpoint); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("proxy source returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, fmt.Errorf("proxy source unavailable or too large")
	}
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	var lines []string
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" && !strings.HasPrefix(line, "#") {
			if len(lines) >= maxProxySourceAddresses {
				return nil, fmt.Errorf("proxy source exceeds address limit")
			}
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

func fetchPublicProxies(ctx context.Context, sources []string) ([]string, int, error) {
	transport := &http.Transport{DialContext: dialProxySource, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many proxy source redirects")
		}
		return validateProxySource(req.URL.String())
	}}
	seen := make(map[string]bool)
	var addresses []string
	loaded := 0
	for _, endpoint := range sources {
		if err := ctx.Err(); err != nil {
			break
		}
		lines, err := fetchProxyLines(ctx, client, endpoint, 32<<20)
		if err != nil {
			continue // One unavailable source must not discard the other lists.
		}
		loaded++
		protocol := proxySourceProtocol(endpoint)
		for _, line := range lines {
			address, valid := discoveredProxy(line, protocol)
			if !valid || seen[address] {
				continue
			}
			if len(addresses) >= maxProxySourceAddresses {
				return nil, loaded, fmt.Errorf("proxy sources exceed address limit")
			}
			seen[address] = true
			addresses = append(addresses, address)
		}
	}
	if len(addresses) == 0 {
		return nil, loaded, fmt.Errorf("no valid public proxies found in source files")
	}
	return addresses, loaded, nil
}
