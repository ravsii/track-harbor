package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type ProxyScrape struct {
	client   *http.Client
	endpoint string
}

func NewProxyScrape() *ProxyScrape {
	return &ProxyScrape{client: &http.Client{Timeout: 30 * time.Second}, endpoint: "https://api.proxyscrape.com/v4/free-proxy-list/get"}
}

func (s *ProxyScrape) Fetch(ctx context.Context) ([]string, error) {
	u, err := url.Parse(s.endpoint)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("request", "getproxies")
	q.Set("protocol", "http")
	q.Set("ssl", "yes")
	q.Set("timeout", "5000")
	q.Set("limit", strconv.Itoa(MaxProxies))
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	scanner := bufio.NewScanner(io.LimitReader(resp.Body, 2<<20))
	var raw []string
	for scanner.Scan() {
		raw = append(raw, strings.TrimSpace(scanner.Text()))
		if len(raw) == MaxProxies {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return usableAddresses(raw), nil
}

// Lists from the Internet must not direct probes at localhost, LANs or cloud
// metadata endpoints. Require a literal public IP, valid port, and no secrets.
func publicProxy(raw string) (string, error) {
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", fmt.Errorf("invalid public proxy")
	}
	ip, err := netip.ParseAddr(u.Hostname())
	if err != nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return "", fmt.Errorf("not a public proxy IP")
	}
	ip = ip.Unmap()
	for _, prefix := range []string{"0.0.0.0/8", "100.64.0.0/10", "198.18.0.0/15"} {
		if netip.MustParsePrefix(prefix).Contains(ip) {
			return "", fmt.Errorf("reserved proxy IP")
		}
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("invalid proxy port")
	}
	return "http://" + net.JoinHostPort(ip.String(), strconv.Itoa(port)), nil
}
