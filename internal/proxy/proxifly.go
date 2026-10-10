package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

type Proxifly struct {
	client   *http.Client
	endpoint string
}

func NewProxifly() *Proxifly {
	return &Proxifly{client: &http.Client{Timeout: 30 * time.Second}, endpoint: "https://raw.githubusercontent.com/proxifly/free-proxy-list/main/proxies/all/data.txt"}
}

func (s *Proxifly) Fetch(ctx context.Context) ([]string, error) {
	return fetchTextList(ctx, s.client, s.endpoint, "")
}

func fetchTextList(ctx context.Context, client *http.Client, endpoint, protocol string) ([]string, error) {
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
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	const maxBytes = 16 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBytes {
		return nil, fmt.Errorf("proxy list exceeds size limit")
	}
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	var addresses []string
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			if protocol != "" && !strings.Contains(line, "://") {
				line = protocol + "://" + line
			}
			addresses = append(addresses, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return usableAddresses(addresses), nil
}

type namedSource struct {
	name   string
	source Source
}

type CombinedSource struct {
	sources []namedSource
	logger  *zap.Logger
}

func NewSources(logger *zap.Logger) *CombinedSource {
	return &CombinedSource{sources: []namedSource{
		{"proxyscrape", NewProxyScrape()}, {"proxifly", NewProxifly()},
		{"speedx_http", newSpeedX("http")},
		{"speedx_socks4", newSpeedX("socks4")},
		{"speedx_socks5", newSpeedX("socks5")},
	}, logger: logger}
}

// One unavailable source does not prevent using the other source's list.
func (s *CombinedSource) Fetch(ctx context.Context) ([]string, error) {
	logger := s.logger
	if logger == nil {
		logger = zap.NewNop()
	}
	var addresses []string
	for _, entry := range s.sources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		list, err := entry.source.Fetch(ctx)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			logger.Warn("proxy_source_failed", zap.String("source", entry.name), zap.Error(err))
			continue
		}
		logger.Info("proxy_source_loaded", zap.String("source", entry.name), zap.Int("count", len(list)))
		addresses = append(addresses, list...)
	}
	addresses = usableAddresses(addresses)
	if len(addresses) == 0 {
		return nil, fmt.Errorf("ни один источник не вернул пригодных адресов прокси (ProxyScrape/Proxifly/TheSpeedX)")
	}
	return addresses, nil
}
