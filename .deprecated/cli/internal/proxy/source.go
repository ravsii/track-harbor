package proxy

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
	var addresses []string
	for _, protocol := range []string{"http", "socks4", "socks5"} {
		seenPages := make(map[[32]byte]bool)
		for skip := 0; ; {
			page, err := s.fetchPage(ctx, protocol, skip)
			if err != nil {
				return nil, fmt.Errorf("%s, skip=%d: %w", protocol, skip, err)
			}
			if len(page) == 0 {
				break
			}
			fingerprint := sha256.Sum256([]byte(strings.Join(page, "\n")))
			if seenPages[fingerprint] {
				return nil, fmt.Errorf("ProxyScrape repeats %s page at skip=%d", protocol, skip)
			}
			seenPages[fingerprint] = true
			for _, raw := range page {
				if !strings.Contains(raw, "://") {
					raw = protocol + "://" + raw
				}
				addresses = append(addresses, raw)
			}
			skip += len(page)
			if len(page) < PageSize {
				break
			}
		}
	}
	return usableAddresses(addresses), nil
}

const PageSize = 2000

func (s *ProxyScrape) fetchPage(ctx context.Context, protocol string, skip int) ([]string, error) {
	u, err := url.Parse(s.endpoint)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("request", "getproxies")
	q.Set("protocol", protocol)
	q.Set("ssl", "all")
	q.Set("timeout", "10000")
	q.Set("limit", strconv.Itoa(PageSize))
	q.Set("skip", strconv.Itoa(skip))
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 2<<20 {
		return nil, fmt.Errorf("proxy page exceeds size limit")
	}
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	var raw []string
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			raw = append(raw, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return raw, nil
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
	if (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks4" && u.Scheme != "socks5") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
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
	return u.Scheme + "://" + net.JoinHostPort(ip.String(), strconv.Itoa(port)), nil
}
