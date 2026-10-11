package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestHTTPSCheckerActuallyTunnelsAndVerifiesTLS(t *testing.T) {
	for _, secureProxy := range []bool{false, true} {
		t.Run(fmt.Sprintf("TLSProxy=%v", secureProxy), func(t *testing.T) {
			var status atomic.Int64
			status.Store(200)
			target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(int(status.Load()))
				fmt.Fprint(w, "User-agent: *\nDisallow: /private\n")
			}))
			defer target.Close()
			var connects atomic.Int64
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodConnect {
					t.Error("probe bypassed HTTPS CONNECT")
					w.WriteHeader(400)
					return
				}
				connects.Add(1)
				upstream, err := net.Dial("tcp", target.Listener.Addr().String())
				if err != nil {
					w.WriteHeader(502)
					return
				}
				defer upstream.Close()
				client, buffer, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				defer client.Close()
				fmt.Fprint(buffer, "HTTP/1.1 200 Connection Established\r\n\r\n")
				buffer.Flush()
				go func() { io.Copy(upstream, buffer); upstream.Close() }()
				io.Copy(client, upstream)
			})
			proxyServer := httptest.NewServer(handler)
			if secureProxy {
				proxyServer.Close()
				proxyServer = httptest.NewTLSServer(handler)
			}
			defer proxyServer.Close()
			checker := NewHTTPSChecker()
			checker.target = target.URL + "/robots.txt"
			if err := checker.Check(context.Background(), proxyServer.URL); err == nil {
				t.Fatal("untrusted TLS certificate accepted")
			}
			roots := x509.NewCertPool()
			roots.AddCert(target.Certificate())
			if secureProxy {
				roots.AddCert(proxyServer.Certificate())
			}
			checker.tlsConfig = &tls.Config{RootCAs: roots}
			if err := checker.Check(context.Background(), proxyServer.URL); err != nil {
				t.Fatal(err)
			}
			status.Store(429)
			if err := checker.Check(context.Background(), proxyServer.URL); err == nil {
				t.Fatal("rate-limited proxy cached as valid")
			}
			wantConnects := int64(3)
			if secureProxy {
				wantConnects = 2
			} // Untrusted proxy TLS fails before CONNECT.
			if connects.Load() != wantConnects {
				t.Fatal("checks did not traverse the proxy")
			}
		})
	}
}
