package proxy

import (
	"context"
	"net/http"
	"time"
)

// SpeedX publishes bare ip:port addresses in one file per protocol.
// Each file is an independent source so an unavailable file does not discard
// the other files' addresses.
type speedXSource struct {
	client   *http.Client
	endpoint string
	protocol string
}

func newSpeedX(protocol string) *speedXSource {
	return &speedXSource{client: &http.Client{Timeout: 30 * time.Second}, endpoint: "https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/" + protocol + ".txt", protocol: protocol}
}

func (s *speedXSource) Fetch(ctx context.Context) ([]string, error) {
	return fetchTextList(ctx, s.client, s.endpoint, s.protocol)
}
