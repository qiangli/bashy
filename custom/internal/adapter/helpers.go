// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package adapter

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/qiangli/bash/pathconv"
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

// ChildExitStatus extracts the child's exit code (128+sig on signal death).
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

// AdaptKubeEnvironment converts KUBECONFIG and related plane path variables
// from MSYS/POSIX drive forms (/c/...) to native Windows path lists (C:\...).
func AdaptKubeEnvironment() {
	for _, key := range []string{
		"KUBECONFIG",
		"OUTPOST_KUBECONFIG_PATH",
		"DKS_PEER_KUBECONFIG",
		"DKS_FEDERATION_KUBECONFIG",
	} {
		if val := os.Getenv(key); val != "" {
			_ = os.Setenv(key, pathconv.NativePathList(val))
		}
	}
}

// NativeArgsOnWindows converts POSIX-form drive paths on Windows only.
func NativeArgsOnWindows(args []string) []string {
	if runtime.GOOS != "windows" {
		return args
	}
	return pathconv.NativeArgs(args)
}

// ForwardSignals relays interrupt/termination signals to a child process until
// the returned stop function is invoked.
func ForwardSignals(p *os.Process) func() {
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, forwardTermSignals()...)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case s := <-signals:
				_ = p.Signal(s)
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(signals)
		close(done)
	}
}

// AssertStandaloneDeps proves that the standalone command package builds under
// GOWORK=off and links no other command packages (neither other bashy packages,
// yoke/cmds/*, coreutils/cmds/*, nor unrelated yoke/external/* command packages).
func AssertStandaloneDeps(t *testing.T, selfEntry string, allowedExtPkgs ...string) {
	t.Helper()
	allow := map[string]bool{
		"github.com/qiangli/bashy/custom/" + selfEntry:     true,
		"github.com/qiangli/bashy/custom/internal/adapter": true,
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
