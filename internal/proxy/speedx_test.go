package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSpeedXFilesAssignProtocolAndFilterUnsafeAddresses(t *testing.T) {
	for _, protocol := range []string{"http", "socks4", "socks5"} {
		t.Run(protocol, func(t *testing.T) {
			source := newSpeedX(protocol)
			if source.endpoint != "https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/"+protocol+".txt" {
				t.Fatalf("wrong file: %s", source.endpoint)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, "203.0.113.1:8080\r\n203.0.113.1:8080\n127.0.0.1:80\n10.0.0.1:80\nnot-a-proxy\n203.0.113.2:1080\n")
			}))
			defer server.Close()
			source.endpoint = server.URL
			addresses, err := source.Fetch(context.Background())
			if err != nil || len(addresses) != 2 || addresses[0] != protocol+"://203.0.113.1:8080" || addresses[1] != protocol+"://203.0.113.2:1080" {
				t.Fatalf("addresses=%v err=%v", addresses, err)
			}
		})
	}
}

func TestDefaultSourcesIncludeAllThreeSpeedXFiles(t *testing.T) {
	sources := NewSources(nil)
	found := make(map[string]bool)
	for _, entry := range sources.sources {
		found[entry.name] = true
	}
	for _, name := range []string{"proxyscrape", "proxifly", "speedx_http", "speedx_socks4", "speedx_socks5"} {
		if !found[name] {
			t.Fatalf("missing source %s", name)
		}
	}
}

func TestSpeedXFileFailureKeepsOtherFilesAndDeduplicatesExistingSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/socks4.txt" {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, "203.0.113.1:8080\n")
	}))
	defer server.Close()
	combined := &CombinedSource{sources: []namedSource{{"existing", sourceFunc(func(context.Context) ([]string, error) { return []string{"http://203.0.113.1:8080"}, nil })}}}
	for _, protocol := range []string{"http", "socks4", "socks5"} {
		source := newSpeedX(protocol)
		source.endpoint = server.URL + "/" + protocol + ".txt"
		combined.sources = append(combined.sources, namedSource{"speedx_" + protocol, source})
	}
	addresses, err := combined.Fetch(context.Background())
	if err != nil || fmt.Sprint(addresses) != "[http://203.0.113.1:8080 socks5://203.0.113.1:8080]" {
		t.Fatalf("addresses=%v err=%v", addresses, err)
	}
}

func TestSpeedXRejectsOversizedFileAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, strings.Repeat("x", (16<<20)+1)) }))
	defer server.Close()
	source := newSpeedX("socks5")
	source.endpoint = server.URL
	if _, err := source.Fetch(context.Background()); err == nil {
		t.Fatal("oversized file accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := source.Fetch(ctx); err == nil {
		t.Fatal("canceled source loaded")
	}
}

func TestCacheFromBeforeSpeedXTriggersFullDiscovery(t *testing.T) {
	db, store := proxyDB(t)
	until := time.Now().Add(time.Hour)
	if err := store.Save(context.Background(), "http://203.0.113.1:80", until); err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(context.Background(), until); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE proxy_cache_policy SET policy='proxyscrape+proxifly/1s/v1'`); err != nil {
		t.Fatal(err)
	}
	fetched := false
	_, err := NewManager(sourceFunc(func(context.Context) ([]string, error) {
		fetched = true
		return []string{"socks5://203.0.113.2:80"}, nil
	}), checkFunc(func(context.Context, string) error { return nil }), store, nil).Prepare(context.Background(), nil)
	if err != nil || !fetched {
		t.Fatalf("old cache bypassed new sources: fetched=%v err=%v", fetched, err)
	}
}
