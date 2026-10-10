package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProxiflyLoadsFullListAndKeepsHTTPSProxies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 3000; i++ {
			fmt.Fprintf(w, "socks5://203.0.113.1:%d\n", 10000+i)
		}
		fmt.Fprint(w, "https://203.0.113.2:443\nhttp://203.0.113.3:80\nsocks4://203.0.113.4:80\nhttp://127.0.0.1:80\nsocks5://203.0.113.1:10000\n")
	}))
	defer server.Close()
	source := NewProxifly()
	source.endpoint = server.URL
	addresses, err := source.Fetch(context.Background())
	if err != nil || len(addresses) != 3003 {
		t.Fatalf("addresses=%d err=%v", len(addresses), err)
	}
	if addresses[3000] != "https://203.0.113.2:443" {
		t.Fatal("HTTPS proxy scheme discarded")
	}
}

func TestProxiflyRejectsHTTPErrorAndOversizedList(t *testing.T) {
	for _, oversized := range []bool{false, true} {
		t.Run(fmt.Sprint(oversized), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !oversized {
					w.WriteHeader(500)
					return
				}
				fmt.Fprint(w, strings.Repeat("x", (16<<20)+1))
			}))
			defer server.Close()
			source := NewProxifly()
			source.endpoint = server.URL
			if _, err := source.Fetch(context.Background()); err == nil {
				t.Fatal("invalid response accepted")
			}
		})
	}
}

func TestCombinedSourcesMergeAndDeduplicate(t *testing.T) {
	source := &CombinedSource{sources: []namedSource{
		{"first", sourceFunc(func(context.Context) ([]string, error) {
			return []string{"http://203.0.113.1:80", "socks5://203.0.113.2:80"}, nil
		})},
		{"second", sourceFunc(func(context.Context) ([]string, error) {
			return []string{"http://203.0.113.1:80", "https://203.0.113.3:443"}, nil
		})},
	}}
	addresses, err := source.Fetch(context.Background())
	if err != nil || len(addresses) != 3 {
		t.Fatalf("addresses=%v err=%v", addresses, err)
	}
}

func TestCombinedSourcesKeepWorkingSourceWhenAnotherFails(t *testing.T) {
	source := &CombinedSource{sources: []namedSource{
		{"offline", sourceFunc(func(context.Context) ([]string, error) { return nil, errors.New("offline") })},
		{"working", sourceFunc(func(context.Context) ([]string, error) { return []string{"http://203.0.113.1:80"}, nil })},
	}}
	if addresses, err := source.Fetch(context.Background()); err != nil || len(addresses) != 1 {
		t.Fatalf("addresses=%v err=%v", addresses, err)
	}
	source.sources = source.sources[:1]
	if _, err := source.Fetch(context.Background()); err == nil {
		t.Fatal("all sources offline accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := source.Fetch(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestProbeTimeoutIsOneSecondForEntireRequest(t *testing.T) {
	checker := NewHTTPSChecker()
	if checker.timeout != time.Second {
		t.Fatalf("timeout=%v, want 1s", checker.timeout)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1500 * time.Millisecond)
		fmt.Fprint(w, "User-agent: *")
	}))
	defer server.Close()
	checker.target = "http://www.youtube.com/robots.txt"
	start := time.Now()
	if err := checker.Check(context.Background(), server.URL); err == nil {
		t.Fatal("slow proxy accepted")
	}
	if elapsed := time.Since(start); elapsed > 1400*time.Millisecond {
		t.Fatalf("probe exceeded deadline: %v", elapsed)
	}
}

func TestCacheWithoutCurrentSourcePolicyTriggersDiscovery(t *testing.T) {
	db, store := proxyDB(t)
	if err := store.Save(context.Background(), "http://203.0.113.1:80", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO proxy_cache_state (id,full_valid_until) VALUES (1,?)`, time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	fetched := false
	_, err := NewManager(sourceFunc(func(context.Context) ([]string, error) { fetched = true; return []string{"http://203.0.113.2:80"}, nil }), checkFunc(func(context.Context, string) error { return nil }), store, nil).Prepare(context.Background(), nil)
	if err != nil || !fetched {
		t.Fatalf("old source policy bypassed discovery: %v", err)
	}
}
