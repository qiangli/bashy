package adapter

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/net/http/httpproxy"
)

func init() {
	if transport, ok := http.DefaultTransport.(*http.Transport); ok {
		transport.Proxy = proxyFromEnvironment
	}
}

func proxyFromEnvironment(req *http.Request) (*url.URL, error) {
	all := firstEnv("ALL_PROXY", "all_proxy")
	httpP := firstEnv("HTTP_PROXY", "http_proxy")
	if httpP == "" {
		httpP = all
	}
	httpsP := firstEnv("HTTPS_PROXY", "https_proxy")
	if httpsP == "" {
		httpsP = all
	}
	cfg := &httpproxy.Config{
		HTTPProxy:  httpP,
		HTTPSProxy: httpsP,
		NoProxy:    firstEnv("NO_PROXY", "no_proxy"),
		CGI:        os.Getenv("REQUEST_METHOD") != "",
	}
	return cfg.ProxyFunc()(req.URL)
}

func ApplyAllProxyFallback() {
	all := firstEnv("ALL_PROXY", "all_proxy")
	if all == "" {
		return
	}
	if firstEnv("HTTP_PROXY", "HTTP_PROXY") == "" && firstEnv("http_proxy") == "" {
		_ = os.Setenv("HTTP_PROXY", all)
	}
	if firstEnv("HTTPS_PROXY", "https_proxy") == "" {
		_ = os.Setenv("HTTPS_PROXY", all)
	}
}

func firstEnv(names ...string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}

func ChildExitStatus(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		return ee.ExitCode()
	}
	return 1
}

func AssertStandaloneDeps(t *testing.T, selfEntry string, allowedExtPkgs ...string) {
	t.Helper()
	allow := map[string]bool{
		"github.com/qiangli/bashy/toolchain/" + selfEntry:     true,
		"github.com/qiangli/bashy/toolchain/internal/adapter": true,
	}
	for _, p := range allowedExtPkgs {
		allow[p] = true
	}
	cmd := exec.Command("go", "list", "-deps", ".")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("GOWORK=off go list -deps .: %v\n%s", err, out)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if allow[pkg] {
			continue
		}
		if strings.HasPrefix(pkg, "github.com/qiangli/bashy/") {
			t.Errorf("%s links another bashy package: %s", selfEntry, pkg)
		}
		if strings.HasPrefix(pkg, "github.com/qiangli/yoke/cmds/") && !strings.HasPrefix(pkg, "github.com/qiangli/yoke/cmds/internal/") {
			t.Errorf("%s links yoke command package: %s", selfEntry, pkg)
		}
		if strings.HasPrefix(pkg, "github.com/qiangli/coreutils/cmds/") && !strings.HasPrefix(pkg, "github.com/qiangli/coreutils/cmds/internal/") {
			t.Errorf("%s links coreutils command package: %s", selfEntry, pkg)
		}
		if strings.HasPrefix(pkg, "github.com/qiangli/yoke/external/") && !strings.HasPrefix(pkg, "github.com/qiangli/yoke/external/internal/") {
			// external/go-billy, external/multierror, external/errwrap, external/securejoin, external/runewidth are fork replace libraries

			isSharedLib := strings.HasPrefix(pkg, "github.com/qiangli/yoke/external/notrack") ||
				strings.HasPrefix(pkg, "github.com/qiangli/yoke/external/go-billy") ||
				strings.HasPrefix(pkg, "github.com/qiangli/yoke/external/multierror") ||
				strings.HasPrefix(pkg, "github.com/qiangli/yoke/external/errwrap") ||
				strings.HasPrefix(pkg, "github.com/qiangli/yoke/external/securejoin") ||
				strings.HasPrefix(pkg, "github.com/qiangli/yoke/external/runewidth")
			if !isSharedLib {
				t.Errorf("%s links another external command package: %s", selfEntry, pkg)
			}
		}
	}
}
