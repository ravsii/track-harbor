package mp3party

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"meshvault/internal/download"
)

func TestProviderUsesHTTP1ForParallelTransfers(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Protocol", r.Proto)
		io.WriteString(w, "audio")
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	p := NewProvider()
	defer p.downloadClient.CloseIdleConnections()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	transport := p.downloadClient.Transport.(*http.Transport)
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	}
	transport.TLSClientConfig.RootCAs = roots
	// A custom TLS config otherwise suppresses automatic HTTP/2 negotiation.
	// Preserve negotiation so this test checks the provider's protocol policy.
	p.downloadClient.Transport.(*http.Transport).ForceAttemptHTTP2 = true
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := p.downloadClient.Get(server.URL)
			if err != nil {
				t.Error(err)
				return
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil || string(body) != "audio" {
				t.Errorf("incomplete transfer: %q, %v", body, err)
			}
			if resp.ProtoMajor != 1 || resp.Header.Get("X-Protocol") != "HTTP/1.1" {
				t.Errorf("parallel transfers must not share an HTTP/2 connection, got %s", resp.Proto)
			}
		}()
	}
	wg.Wait()
}

type routedTransport func(*http.Request) (*http.Response, error)

func (f routedTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestDownloadRejectsHTTPErrorPages(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNotFound, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/music/track":
					io.WriteString(w, `<meta name="csrf-token" content="test">`)
				case "/song_permissions/":
					w.WriteHeader(http.StatusNoContent)
				case "/download/track":
					w.WriteHeader(status)
					io.WriteString(w, "response body")
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			local, err := url.Parse(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			route := routedTransport(func(req *http.Request) (*http.Response, error) {
				req = req.Clone(req.Context())
				req.URL.Scheme, req.URL.Host = local.Scheme, local.Host
				return http.DefaultTransport.RoundTrip(req)
			})
			p := NewProvider()
			p.client.Transport, p.downloadClient.Transport = route, route
			response, err := p.Download(download.Track{ID: "track"})
			if response.Body != nil {
				defer response.Body.Close()
			}
			if status == http.StatusOK {
				if err != nil || response.Body == nil {
					t.Fatalf("valid audio response rejected: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), fmt.Sprint(status)) || response.Body != nil {
				t.Fatalf("HTTP error could be saved and hashed as audio: status=%d err=%v body=%v", status, err, response.Body)
			}
		})
	}
}

func TestProviderAllowsLongerFileTransfers(t *testing.T) {
	p := NewProvider()
	defer p.client.CloseIdleConnections()
	if p.downloadClient.Timeout < 5*time.Minute || p.downloadClient.Timeout > 10*time.Minute {
		t.Fatalf("file transfer needs a bounded timeout of 5–10 minutes, got %v", p.downloadClient.Timeout)
	}
	if p.client.Timeout != 30*time.Second {
		t.Fatalf("metadata requests must retain their short timeout, got %v", p.client.Timeout)
	}
	if p.client.Jar != p.downloadClient.Jar {
		t.Fatal("metadata and downloads must share permission cookies")
	}
}
