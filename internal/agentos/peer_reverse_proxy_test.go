// Copyright (c) 2026 qiangli
// See LICENSE for licensing information.

package agentos

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/bashy/internal/httpproxy"
	"github.com/qiangli/outpost/pkg/sshclient"
)

func testPeerReverseProxy(t *testing.T, channel *PeerChannel) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "operator-egress") }))
	defer origin.Close()
	for _, scheme := range []string{"http", "socks5h"} {
		t.Run(scheme, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			// Only the operator proxy knows how to reach this deliberately unresolvable name.
			dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
				if addr != "operator-only.invalid:80" {
					t.Errorf("unexpected proxy target %s", addr)
				}
				return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(origin.URL, "http://"))
			}
			if scheme == "http" {
				go httpproxy.Serve(ctx, ln, httpproxy.Config{Auth: "operator:p'ass", DialContext: dial})
			} else {
				go serveSOCKS5(ctx, ln, "operator", "p'ass", dial, nil)
			}
			operatorURL := &url.URL{Scheme: scheme, Host: ln.Addr().String(), User: url.UserPassword("operator", "p'ass")}
			p, err := channel.ReverseProxy(ctx, operatorURL.String(), "127.0.0.1:0", "localhost,127.0.0.1,::1")
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			u, err := url.Parse(p.URL())
			if err != nil {
				t.Fatal(err)
			}
			if !net.ParseIP(u.Hostname()).IsLoopback() {
				t.Fatalf("non-loopback %s", u.Host)
			}
			tr := &http.Transport{Proxy: http.ProxyURL(u)}
			defer tr.CloseIdleConnections()
			client := &http.Client{Transport: tr, Timeout: 3 * time.Second}
			resp, err := client.Get("http://operator-only.invalid/")
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || string(body) != "operator-egress" {
				t.Fatalf("body %q err %v", body, err)
			}
			result, err := p.Exec(ctx, sshclient.ExecOptions{Command: `printf '%s\n' "$ALL_PROXY" "$HTTPS_PROXY" "$HTTP_PROXY" "$NO_PROXY" "$all_proxy" "$https_proxy" "$http_proxy" "$no_proxy"`})
			want := strings.Repeat(p.URL()+"\n", 3) + "localhost,127.0.0.1,::1\n"
			if err != nil || result.ExitCode != 0 || string(result.Stdout) != want+want {
				t.Fatalf("env result %+v err %v want %q", result, err, want+want)
			}
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			fetched, err := p.Exec(ctx, sshclient.ExecOptions{Command: "BASHY_TEST_REVERSE_FETCH=1 " + shellQuote(exe) + " -test.run=^TestPeerReverseProxyFetchProcess$"})
			if err != nil || fetched.ExitCode != 0 || !strings.Contains(string(fetched.Stdout), "operator-egress") {
				t.Fatalf("remote fetch: %+v err %v", fetched, err)
			}
			p.Close()
			if _, err := p.Exec(ctx, sshclient.ExecOptions{Command: "printf must-not-run"}); err == nil {
				t.Fatal("Exec after Close succeeded")
			}
			deadline := time.Now().Add(2 * time.Second)
			for {
				c, err := net.DialTimeout("tcp", u.Host, 100*time.Millisecond)
				if err != nil {
					break
				}
				c.Close()
				if time.Now().After(deadline) {
					t.Fatal("remote listener survived Close")
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
	if _, err := channel.RemoteForward(context.Background(), "0.0.0.0:0", "127.0.0.1:8080"); err == nil {
		t.Fatal("wildcard remote accepted")
	}
}

func TestReverseProxyRejectsInvalidConfiguration(t *testing.T) {
	for _, raw := range []string{"", "ftp://127.0.0.1:8080", "http://0.0.0.0:8080", "http://example.com:8080", "http://127.0.0.1", "http://127.0.0.1:0", "http://127.0.0.1:8080/path", "http://127.0.0.1:8080?q=x"} {
		if _, err := parseOperatorProxy(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}

func TestReverseProxyURLValidation(t *testing.T) {
	for _, raw := range []string{"http://localhost:8080", "http://[::1]:8080", "socks5://user:p%27ass@127.0.0.1:1080"} {
		u, err := parseOperatorProxy(raw)
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if strings.HasPrefix(raw, "socks5:") && u.Scheme != "socks5h" {
			t.Fatal("SOCKS must resolve at operator")
		}
	}
	if _, err := (*PeerChannel)(nil).RemoteForward(context.Background(), "127.0.0.1:0", "127.0.0.1:8080"); err == nil {
		t.Fatal("nil channel accepted")
	}
	if _, err := (*PeerChannel)(nil).ReverseProxy(nil, "http://127.0.0.1:8080", "", ""); err == nil {
		t.Fatal("nil context accepted")
	}
}

// Invoked as an actual remote SSH command, in a fresh process so net/http reads
// the exported proxy environment rather than the test runner's cached settings.
func TestPeerReverseProxyFetchProcess(t *testing.T) {
	if os.Getenv("BASHY_TEST_REVERSE_FETCH") != "1" {
		return
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://operator-only.invalid/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || string(body) != "operator-egress" {
		t.Fatalf("remote body %q err %v", body, err)
	}
	os.Stdout.Write(body)
}
