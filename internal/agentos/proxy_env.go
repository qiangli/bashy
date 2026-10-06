package agentos

import (
	"net/http"
	"net/url"
	"os"

	gitclient "github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"golang.org/x/net/http/httpproxy"
)

// installProxyEnvironment gives every in-process HTTP consumer one proxy
// policy. In particular, binmgr uses http.DefaultClient while go-git captures
// its own client during package initialization, so both must be wired here.
func installProxyEnvironment() {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return
	}
	// ProxyFromEnvironment intentionally ignores ALL_PROXY. Keep the standard
	// HTTP(S)_PROXY precedence while adding ALL_PROXY as the fallback, and build
	// the matcher per request so a long-running bashy observes environment
	// changes made by the shell.
	transport.Proxy = proxyFromEnvironment

	// go-git's default client captured DefaultTransport before this package's
	// init ran. Reinstall both URL schemes explicitly so bashy git clone/fetch/
	// push use the same transport as binmgr.
	gitHTTP := githttp.NewClient(&http.Client{Transport: transport})
	gitclient.InstallProtocol("http", gitHTTP)
	gitclient.InstallProtocol("https", gitHTTP)
}

func init() { installProxyEnvironment() }

func proxyFromEnvironment(req *http.Request) (*url.URL, error) {
	all := firstProxyEnv("ALL_PROXY", "all_proxy")
	httpProxy := firstProxyEnv("HTTP_PROXY", "http_proxy")
	if httpProxy == "" {
		httpProxy = all
	}
	httpsProxy := firstProxyEnv("HTTPS_PROXY", "https_proxy")
	if httpsProxy == "" {
		httpsProxy = all
	}
	cfg := &httpproxy.Config{
		HTTPProxy:  httpProxy,
		HTTPSProxy: httpsProxy,
		NoProxy:    firstProxyEnv("NO_PROXY", "no_proxy"),
		CGI:        os.Getenv("REQUEST_METHOD") != "",
	}
	return cfg.ProxyFunc()(req.URL)
}

// applyAllProxyFallback normalizes the environment inherited by subprocesses.
// The Go command already implements HTTP_PROXY, HTTPS_PROXY and NO_PROXY (and
// accepts socks5/socks5h proxy URLs), but unlike curl it does not consult
// ALL_PROXY. Supplying only missing scheme-specific variables preserves their
// normal precedence and makes `bashy go mod download` match in-process fetches.
func applyAllProxyFallback() {
	all := firstProxyEnv("ALL_PROXY", "all_proxy")
	if all == "" {
		return
	}
	if firstProxyEnv("HTTP_PROXY", "http_proxy") == "" {
		_ = os.Setenv("HTTP_PROXY", all)
	}
	if firstProxyEnv("HTTPS_PROXY", "https_proxy") == "" {
		_ = os.Setenv("HTTPS_PROXY", all)
	}
}

func firstProxyEnv(names ...string) string {
	for _, name := range names {
		if value := os.Getenv(name); value != "" {
			return value
		}
	}
	return ""
}
