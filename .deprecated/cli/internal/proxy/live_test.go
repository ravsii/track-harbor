package proxy

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLiveProxyScrapePreparation(t *testing.T) {
	if os.Getenv("TRACKHARBOR_PROXY_LIVE") != "1" {
		t.Skip("set TRACKHARBOR_PROXY_LIVE=1 for real ProxyScrape/YouTube probes")
	}
	_, store := proxyDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	pool, err := NewManager(NewSources(nil), NewHTTPSChecker(), store, nil).Prepare(ctx, func(checked, total, valid int) {
		if checked%1000 == 0 || checked == total {
			t.Logf("probed %d/%d, valid=%d", checked, total, valid)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("verified %d proxies, cached for one hour", pool.Len())
	if _, err := NewManager(nil, nil, store, nil).Prepare(ctx, nil); err != nil {
		t.Fatalf("persisted cache was not reusable: %v", err)
	}
}
