package web

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	proxyScrapePageSize     = 2000
	maxProxySourceAddresses = 500000 // Reject unexpectedly large remote lists rather than truncate them.
)

type publicProxyGroup struct {
	key        string
	candidates []string
}

func proxyHostPort(address string) string {
	u, err := url.Parse(address)
	if err != nil || u.Hostname() == "" || u.Port() == "" {
		return ""
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return ""
	}
	return net.JoinHostPort(strings.ToLower(u.Hostname()), strconv.Itoa(port))
}

// Only probe literal, globally routable public IPs from third-party lists.
func discoveredProxy(raw, protocol string) (string, string, bool) {
	if !strings.Contains(raw, "://") {
		raw = protocol + "://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", "", false
	}
	switch u.Scheme {
	case "http", "https", "socks4", "socks5":
	default:
		return "", "", false
	}
	ip, err := netip.ParseAddr(u.Hostname())
	if err != nil || ip.Zone() != "" || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return "", "", false
	}
	ip = ip.Unmap()
	for _, prefix := range []string{"0.0.0.0/8", "100.64.0.0/10", "198.18.0.0/15"} {
		if netip.MustParsePrefix(prefix).Contains(ip) {
			return "", "", false
		}
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return "", "", false
	}
	key := net.JoinHostPort(ip.String(), strconv.Itoa(port))
	return key, u.Scheme + "://" + key, true
}

func fetchProxyLines(ctx context.Context, client *http.Client, endpoint string, limit int64) ([]string, error) {
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
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

func fetchProxyScrape(ctx context.Context, client *http.Client) ([]string, error) {
	var addresses []string
	for _, protocol := range []string{"http", "socks4", "socks5"} {
		seenPages := make(map[[32]byte]bool)
		for skip := 0; ; {
			if len(addresses) >= maxProxySourceAddresses {
				return nil, fmt.Errorf("proxy source exceeds address limit")
			}
			u, _ := url.Parse("https://api.proxyscrape.com/v4/free-proxy-list/get")
			query := u.Query()
			query.Set("request", "getproxies")
			query.Set("protocol", protocol)
			query.Set("ssl", "all")
			query.Set("timeout", "10000")
			query.Set("limit", strconv.Itoa(proxyScrapePageSize))
			query.Set("skip", strconv.Itoa(skip))
			u.RawQuery = query.Encode()
			page, err := fetchProxyLines(ctx, client, u.String(), 2<<20)
			if err != nil {
				return nil, err
			}
			if len(page) == 0 {
				break
			}
			fingerprint := sha256.Sum256([]byte(strings.Join(page, "\n")))
			if seenPages[fingerprint] {
				return nil, fmt.Errorf("proxy source repeated a page")
			}
			seenPages[fingerprint] = true
			for _, address := range page {
				if len(addresses) >= maxProxySourceAddresses {
					return nil, fmt.Errorf("proxy source exceeds address limit")
				}
				if !strings.Contains(address, "://") {
					address = protocol + "://" + address
				}
				addresses = append(addresses, address)
			}
			skip += len(page)
			if len(page) < proxyScrapePageSize {
				break
			}
		}
	}
	return addresses, nil
}

func fetchPublicProxies(ctx context.Context) ([]publicProxyGroup, int, error) {
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, _ []*http.Request) error {
		if req.URL.Scheme != "https" || (req.URL.Host != "raw.githubusercontent.com" && req.URL.Host != "api.proxyscrape.com") {
			return http.ErrUseLastResponse
		}
		return nil
	}}
	sources := []struct {
		endpoint, protocol string
		paged              bool
	}{
		{"", "", true},
		{"https://raw.githubusercontent.com/proxifly/free-proxy-list/main/proxies/all/data.txt", "", false},
		{"https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/http.txt", "http", false},
		{"https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/socks4.txt", "socks4", false},
		{"https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/socks5.txt", "socks5", false},
	}
	groups := make(map[string]int)
	var ordered []publicProxyGroup
	loaded := 0
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return nil, loaded, err
		}
		var lines []string
		var err error
		if source.paged {
			lines, err = fetchProxyScrape(ctx, client)
		} else {
			lines, err = fetchProxyLines(ctx, client, source.endpoint, 16<<20)
		}
		if err != nil {
			continue // An unavailable source must not discard the other lists.
		}
		loaded++
		for _, line := range lines {
			key, address, valid := discoveredProxy(line, source.protocol)
			if !valid {
				continue
			}
			i, exists := groups[key]
			if !exists {
				if len(ordered) >= maxProxySourceAddresses {
					return nil, loaded, fmt.Errorf("proxy sources exceed address limit")
				}
				i = len(ordered)
				groups[key] = i
				ordered = append(ordered, publicProxyGroup{key: key})
			}
			seen := slices.Contains(ordered[i].candidates, address)
			if !seen {
				ordered[i].candidates = append(ordered[i].candidates, address)
			}
		}
	}
	if loaded == 0 || len(ordered) == 0 {
		return nil, loaded, fmt.Errorf("no usable public proxy candidates from sources")
	}
	return ordered, loaded, nil
}
