package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestProxyScrapePaginatesAllProtocolsWithoutHTTPSFilter(t *testing.T) {
	requests := make(map[string][]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for key, want := range map[string]string{"request": "getproxies", "ssl": "all", "limit": "2000", "timeout": "10000"} {
			if got := r.URL.Query().Get(key); got != want {
				t.Errorf("%s=%s, want %s", key, got, want)
			}
		}
		protocol := r.URL.Query().Get("protocol")
		skip, _ := strconv.Atoi(r.URL.Query().Get("skip"))
		requests[protocol] = append(requests[protocol], skip)
		if skip == 0 {
			for i := 0; i < 2000; i++ {
				fmt.Fprintf(w, "203.0.113.1:%d\n", 10000+i)
			}
			return
		}
		if skip != 2000 {
			t.Errorf("unexpected offset %d", skip)
		}
		fmt.Fprint(w, "203.0.113.1:10000\n127.0.0.1:80\n10.0.0.1:80\n169.254.169.254:80\nnot-a-proxy\n203.0.113.2:8080\n")
	}))
	defer server.Close()
	source := NewProxyScrape()
	source.endpoint = server.URL
	urls, err := source.Fetch(context.Background())
	if err != nil || len(urls) != 6003 || urls[0] != "http://203.0.113.1:10000" {
		t.Fatalf("wrong list: %d addresses, err=%v", len(urls), err)
	}
	for _, protocol := range []string{"http", "socks4", "socks5"} {
		if fmt.Sprint(requests[protocol]) != "[0 2000]" {
			t.Fatalf("missing pages: %v", requests)
		}
	}
	if !strings.Contains(strings.Join(urls, "\n"), "socks5://203.0.113.2:8080") {
		t.Fatal("SOCKS addresses lost")
	}
}

func TestProxyScrapeStopsIfServerIgnoresPagination(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		for i := 0; i < 2000; i++ {
			fmt.Fprintf(w, "203.0.113.1:%d\n", 10000+i)
		}
	}))
	defer server.Close()
	source := NewProxyScrape()
	source.endpoint = server.URL
	if _, err := source.Fetch(context.Background()); err == nil {
		t.Fatal("repeating pages silently accepted")
	}
	if count != 2 {
		t.Fatalf("pagination did not stop: %d", count)
	}
}

func TestProxyScrapeRejectsHTTPErrorPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500); fmt.Fprint(w, "203.0.113.1:80") }))
	defer server.Close()
	source := NewProxyScrape()
	source.endpoint = server.URL
	if _, err := source.Fetch(context.Background()); err == nil {
		t.Fatal("HTTP error page accepted")
	}
}
