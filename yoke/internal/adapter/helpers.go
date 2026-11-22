package adapter

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
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

func firstEnv(names ...string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}

func RunToolMain(def string) {
	os.Exit(RunTool(InvokedAs(def), os.Args[1:], tool.Stdio{
		In:  os.Stdin,
		Out: os.Stdout,
		Err: os.Stderr,
	}))
}

func RunTool(name string, args []string, stdio tool.Stdio) int {
	StripAdapterEnv()
	t := tool.Lookup(name)
	if t == nil {
		fmt.Fprintf(stdio.Err, "bashy: %s: No such command\n", name)
		return 127
	}
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stdio.Err, "bashy: %s: %v\n", name, err)
		return 1
	}
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   os.Environ(),
		FS:    tool.NewLocalFS(),
		Stdio: stdio,
	}
	return t.Run(rc, args)
}

func AssertStandaloneDeps(t *testing.T, selfEntry string, allowedCmdPkgs ...string) {
	t.Helper()
	allow := map[string]bool{
		"github.com/qiangli/bashy/yoke/" + selfEntry:     true,
		"github.com/qiangli/bashy/yoke/internal/adapter": true,
	}
	for _, p := range allowedCmdPkgs {
		allow[p] = true
	}
	cmd := exec.Command("go", "list", "-deps", ".")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("GOWORK=off go list -deps .: %v\n%s", err, out)
	}
	pkgs := strings.Fields(string(out))
	hasWeaveLib := false
	for _, p := range pkgs {
		if p == "github.com/qiangli/yoke/pkg/weave" {
			hasWeaveLib = true
			break
		}
	}
	for _, pkg := range pkgs {
		if allow[pkg] {
			continue
		}
		if strings.HasPrefix(pkg, "github.com/qiangli/bashy/") {
			t.Errorf("%s links another bashy package: %s", selfEntry, pkg)
		}
		if strings.HasPrefix(pkg, "github.com/qiangli/yoke/cmds/") && !strings.HasPrefix(pkg, "github.com/qiangli/yoke/cmds/internal/") {
			// Frozen yoke/pkg/weave library has an internal import of yoke/cmds/browser; allow only that transitive library edge.
			if pkg == "github.com/qiangli/yoke/cmds/browser" && hasWeaveLib {
				continue
			}
			t.Errorf("%s links another yoke command package: %s", selfEntry, pkg)
		}
		if strings.HasPrefix(pkg, "github.com/qiangli/coreutils/cmds/") && !strings.HasPrefix(pkg, "github.com/qiangli/coreutils/cmds/internal/") {
			t.Errorf("%s links coreutils command package: %s", selfEntry, pkg)
		}
	}
}
