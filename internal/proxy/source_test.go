package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxyScrapeRequests2000HTTPSCapableHTTPProxies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for key, want := range map[string]string{"request": "getproxies", "protocol": "http", "ssl": "yes", "limit": "2000"} {
			if got := r.URL.Query().Get(key); got != want {
				t.Errorf("%s=%s, want %s", key, got, want)
			}
		}
		fmt.Fprint(w, "203.0.113.1:8080\n203.0.113.1:8080\n127.0.0.1:80\n10.0.0.1:80\n169.254.169.254:80\nnot-a-proxy\n203.0.113.2:8080\n")
	}))
	defer server.Close()
	source := NewProxyScrape()
	source.endpoint = server.URL
	urls, err := source.Fetch(context.Background())
	if err != nil || len(urls) != 2 || urls[0] != "http://203.0.113.1:8080" {
		t.Fatalf("unsafe list accepted: %v %v", urls, err)
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
