package httpproxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestClientThroughProxy(t *testing.T) {
	for _, secure := range []bool{false, true} {
		for _, auth := range []string{"", "user:pass:word"} {
			for _, credentials := range []string{"", "user:wrong", "user:pass:word"} {
				t.Run(strings.Join([]string{map[bool]string{false: "http", true: "https"}[secure], auth, credentials}, "/"), func(t *testing.T) {
					target := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Header.Get("Proxy-Authorization") != "" || r.Header.Get("Proxy-Connection") != "" || r.Header.Get("X-Hop") != "" {
							t.Error("proxy headers leaked to origin")
						}
						if r.Method != "POST" || r.URL.RequestURI() != "/path?q=value" {
							t.Errorf("request changed: %s %s", r.Method, r.URL)
						}
						w.Header().Set("X-Origin", "yes")
						io.Copy(w, r.Body)
					}))
					if secure {
						target.StartTLS()
					} else {
						target.Start()
					}
					defer target.Close()
					h, err := New(Config{Auth: auth})
					if err != nil {
						t.Fatal(err)
					}
					defer h.Close()
					proxy := httptest.NewServer(h)
					defer proxy.Close()
					proxyURL, _ := url.Parse(proxy.URL)
					if credentials != "" {
						u, p, _ := strings.Cut(credentials, ":")
						proxyURL.User = url.UserPassword(u, p)
					}
					roots := x509.NewCertPool()
					if secure {
						roots.AddCert(target.Certificate())
					}
					tr := &http.Transport{Proxy: http.ProxyURL(proxyURL), TLSClientConfig: &tls.Config{RootCAs: roots}}
					defer tr.CloseIdleConnections()
					client := &http.Client{Transport: tr, Timeout: 3 * time.Second}
					req, _ := http.NewRequest("POST", target.URL+"/path?q=value", strings.NewReader("forwarded body"))
					if !secure {
						req.Header.Set("Proxy-Connection", "keep-alive")
						req.Header.Set("Connection", "X-Hop")
						req.Header.Set("X-Hop", "private")
					}
					resp, err := client.Do(req)
					denied := auth != "" && credentials != auth
					if denied && secure {
						if err == nil {
							resp.Body.Close()
							t.Fatal("CONNECT accepted bad auth")
						}
						if !strings.Contains(err.Error(), "Proxy Authentication Required") {
							t.Fatalf("want 407, got %v", err)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					defer resp.Body.Close()
					if denied {
						if resp.StatusCode != 407 || resp.Header.Get("Proxy-Authenticate") == "" {
							t.Fatalf("want auth challenge, got %s", resp.Status)
						}
						return
					}
					body, _ := io.ReadAll(resp.Body)
					if resp.StatusCode != 200 || string(body) != "forwarded body" || resp.Header.Get("X-Origin") != "yes" {
						t.Fatalf("response: %s %q", resp.Status, body)
					}
				})
			}
		}
	}
}

func TestServeCancellation(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, l, Config{}) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not stop")
	}
}

// A pipelined payload must survive Hijack's read buffer, and a client half-close
// must still allow the target's final response back through the tunnel.
func TestTunnelBufferedBytesAndHalfClose(t *testing.T) {
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	targetDone := make(chan error, 1)
	go func() {
		c, err := target.Accept()
		if err != nil {
			targetDone <- err
			return
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(3 * time.Second))
		body, err := io.ReadAll(c)
		if err == nil {
			_, err = c.Write(append([]byte("echo:"), body...))
		}
		targetDone <- err
	}()
	h, _ := New(Config{})
	defer h.Close()
	proxy := httptest.NewServer(h)
	defer proxy.Close()
	c, err := net.Dial("tcp", strings.TrimPrefix(proxy.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\npipelined", target.Addr(), target.Addr())
	r := bufio.NewReader(c)
	resp, err := http.ReadResponse(r, &http.Request{Method: "CONNECT"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatal(resp.Status)
	}
	c.(*net.TCPConn).CloseWrite()
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "echo:pipelined" {
		t.Fatalf("lost tunneled data: %q", body)
	}
	if err := <-targetDone; err != nil {
		t.Fatal(err)
	}
}

func TestRejectBeforeDial(t *testing.T) {
	for _, tc := range []struct {
		name, method, target, auth string
		status                     int
	}{
		{"auth", "CONNECT", "example.com:443", "user:pass", 407},
		{"missing port", "CONNECT", "example.com", "", 400},
		{"origin form", "GET", "/", "", 400},
		{"unsupported scheme", "GET", "ftp://example.com/file", "", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := New(Config{Auth: tc.auth, DialContext: func(context.Context, string, string) (net.Conn, error) {
				t.Error("unexpected dial")
				return nil, fmt.Errorf("unexpected dial")
			}})
			defer h.Close()
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.target, nil))
			if w.Code != tc.status {
				t.Fatalf("status %d, want %d", w.Code, tc.status)
			}
		})
	}
}

func TestCancelActiveTunnel(t *testing.T) {
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	targetDone := make(chan error, 1)
	go func() {
		c, err := target.Accept()
		if err != nil {
			targetDone <- err
			return
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(3 * time.Second))
		_, err = io.Copy(io.Discard, c)
		targetDone <- err
	}()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, l, Config{}) }()
	c, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target.Addr(), target.Addr())
	r := bufio.NewReader(c)
	resp, err := http.ReadResponse(r, &http.Request{Method: "CONNECT"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatal(resp.Status)
	}
	cancel()
	if _, err := r.ReadByte(); err != io.EOF {
		t.Fatalf("tunnel not closed: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-targetDone; err != nil {
		t.Fatal(err)
	}
}

func TestDialFailure(t *testing.T) {
	for _, tc := range []struct{ method, target string }{{"GET", "http://example.invalid/"}, {"CONNECT", "example.invalid:443"}} {
		t.Run(tc.method, func(t *testing.T) {
			h, _ := New(Config{DialContext: func(context.Context, string, string) (net.Conn, error) { return nil, fmt.Errorf("unreachable") }})
			defer h.Close()
			proxy := httptest.NewServer(h)
			defer proxy.Close()
			c, err := net.Dial("tcp", strings.TrimPrefix(proxy.URL, "http://"))
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			c.SetDeadline(time.Now().Add(3 * time.Second))
			fmt.Fprintf(c, "%s %s HTTP/1.1\r\nHost: example.invalid:443\r\n\r\n", tc.method, tc.target)
			resp, err := http.ReadResponse(bufio.NewReader(c), &http.Request{Method: tc.method})
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 502 {
				t.Fatalf("got %s", resp.Status)
			}
		})
	}
}
