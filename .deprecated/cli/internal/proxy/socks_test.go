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
	"testing"
	"time"
)

func TestHTTPSCheckerUsesSOCKS4AndSOCKS5(t *testing.T) {
	for _, protocol := range []string{"socks4", "socks5"} {
		t.Run(protocol, func(t *testing.T) {
			target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "User-agent: *\n") }))
			defer target.Close()
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			done := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					done <- err
					return
				}
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(3 * time.Second))
				if protocol == "socks4" {
					var request [9]byte
					if _, err = io.ReadFull(conn, request[:]); err != nil {
						done <- err
						return
					}
					if request[0] != 4 || request[1] != 1 || request[8] != 0 {
						done <- fmt.Errorf("invalid SOCKS4 request %v", request)
						return
					}
					_, err = conn.Write([]byte{0, 90, 0, 0, 0, 0, 0, 0})
				} else {
					var greeting [3]byte
					if _, err = io.ReadFull(conn, greeting[:]); err != nil {
						done <- err
						return
					}
					if greeting != [3]byte{5, 1, 0} {
						done <- fmt.Errorf("invalid SOCKS5 greeting")
						return
					}
					conn.Write([]byte{5, 0})
					var header [4]byte
					if _, err = io.ReadFull(conn, header[:]); err != nil {
						done <- err
						return
					}
					size := 4
					if header[3] == 3 {
						var n [1]byte
						io.ReadFull(conn, n[:])
						size = int(n[0])
					}
					if _, err = io.ReadFull(conn, make([]byte, size+2)); err != nil {
						done <- err
						return
					}
					_, err = conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
				}
				if err != nil {
					done <- err
					return
				}
				upstream, err := net.Dial("tcp", target.Listener.Addr().String())
				if err != nil {
					done <- err
					return
				}
				defer upstream.Close()
				go func() { io.Copy(upstream, conn); upstream.Close() }()
				io.Copy(conn, upstream)
				done <- nil
			}()
			roots := x509.NewCertPool()
			roots.AddCert(target.Certificate())
			checker := NewHTTPSChecker()
			checker.target = target.URL
			checker.tlsConfig = &tls.Config{RootCAs: roots}
			if err := checker.Check(context.Background(), protocol+"://"+listener.Addr().String()); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}
