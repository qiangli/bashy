package agentos

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	outgit "github.com/qiangli/yoke/git"
	"github.com/qiangli/yoke/pkg/binmgr"

	"github.com/qiangli/bashy/internal/httpproxy"
)

func TestBinmgrHonorsProxyEnvironment(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("proxied artifact"))
	}))
	defer origin.Close()
	originAddr := origin.Listener.Addr().String()
	wantSHA := fmt.Sprintf("%x", sha256.Sum256([]byte("proxied artifact")))

	for _, scheme := range []string{"http", "socks5", "socks5h"} {
		t.Run(scheme, func(t *testing.T) {
			proxyAddr, seen, closeProxy := startStoryProxy(t, scheme, originAddr)
			defer closeProxy()
			clearProxyEnvironment(t)
			t.Setenv("ALL_PROXY", scheme+"://"+proxyAddr)
			t.Setenv("BASHY_BIN_CACHE", t.TempDir())

			tool := binmgr.Tool{
				Name: "proxy-fixture", Version: scheme,
				Assets: map[string]binmgr.Asset{
					binmgr.Platform(): {URL: "http://artifact.invalid/tool", SHA256: wantSHA},
				},
			}
			got, err := binmgr.Ensure(context.Background(), tool)
			if err != nil {
				t.Fatal(err)
			}
			if body, err := os.ReadFile(got); err != nil || string(body) != "proxied artifact" {
				t.Fatalf("downloaded body = %q, %v", body, err)
			}
			select {
			case target := <-seen:
				if target != "artifact.invalid:80" {
					t.Fatalf("proxy target = %q", target)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("binmgr bypassed the configured proxy")
			}
		})
	}
}

func TestNativeGitHonorsSOCKS5HProxy(t *testing.T) {
	origin := httptest.NewServer(http.NotFoundHandler())
	defer origin.Close()
	proxyAddr, seen, closeProxy := startStoryProxy(t, "socks5h", origin.Listener.Addr().String())
	defer closeProxy()
	clearProxyEnvironment(t)
	t.Setenv("ALL_PROXY", "socks5h://"+proxyAddr)

	_, err := outgit.Clone(outgit.CloneOptions{
		URL:  "http://git.invalid/repo.git",
		Path: filepath.Join(t.TempDir(), "clone"),
	})
	if err == nil {
		t.Fatal("clone unexpectedly succeeded against a 404 fixture")
	}
	select {
	case target := <-seen:
		if target != "git.invalid:80" {
			t.Fatalf("proxy target = %q", target)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("native git bypassed the configured SOCKS5H proxy")
	}
}

func TestProxyEnvironmentPrecedenceAndNoProxy(t *testing.T) {
	clearProxyEnvironment(t)
	t.Setenv("ALL_PROXY", "socks5h://all.example:1080")
	t.Setenv("HTTPS_PROXY", "http://secure.example:8443")
	t.Setenv("NO_PROXY", ".internal.example,metadata.example:80")

	for _, tc := range []struct {
		url, want string
	}{
		{"http://public.example/file", "socks5h://all.example:1080"},
		{"https://public.example/file", "http://secure.example:8443"},
		{"http://api.internal.example/file", ""},
		{"http://metadata.example/file", ""},
		{"https://metadata.example/file", "http://secure.example:8443"},
	} {
		req, err := http.NewRequest(http.MethodGet, tc.url, nil)
		if err != nil {
			t.Fatal(err)
		}
		got, err := proxyFromEnvironment(req)
		if err != nil {
			t.Fatalf("%s: %v", tc.url, err)
		}
		gotURL := ""
		if got != nil {
			gotURL = got.String()
		}
		if gotURL != tc.want {
			t.Errorf("%s: proxy = %q, want %q", tc.url, gotURL, tc.want)
		}
	}
}

func TestAllProxyFallbackForGoModuleSubprocess(t *testing.T) {
	clearProxyEnvironment(t)
	t.Setenv("ALL_PROXY", "socks5://proxy.example:1080")
	applyAllProxyFallback()
	if got := os.Getenv("HTTP_PROXY"); got != "socks5://proxy.example:1080" {
		t.Fatalf("HTTP_PROXY = %q", got)
	}
	if got := os.Getenv("HTTPS_PROXY"); got != "socks5://proxy.example:1080" {
		t.Fatalf("HTTPS_PROXY = %q", got)
	}
}

func clearProxyEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{"ALL_PROXY", "all_proxy", "HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "NO_PROXY", "no_proxy", "REQUEST_METHOD"} {
		t.Setenv(name, "")
	}
}

func startStoryProxy(t *testing.T, scheme, originAddr string) (string, <-chan string, func()) {
	t.Helper()
	seen := make(chan string, 8)
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		seen <- address
		return (&net.Dialer{}).DialContext(ctx, network, originAddr)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	switch scheme {
	case "http":
		handler, err := httpproxy.New(httpproxy.Config{DialContext: dial})
		if err != nil {
			t.Fatal(err)
		}
		server := &http.Server{Handler: handler}
		go func() { done <- server.Serve(listener) }()
		return listener.Addr().String(), seen, func() {
			cancel()
			handler.Close()
			_ = server.Close()
			<-done
		}
	case "socks5", "socks5h":
		go func() { done <- serveSOCKS5(ctx, listener, "", "", dial, nil) }()
		return listener.Addr().String(), seen, func() {
			cancel()
			<-done
		}
	default:
		listener.Close()
		cancel()
		t.Fatalf("unsupported test proxy scheme %q", scheme)
		return "", nil, func() {}
	}
}
