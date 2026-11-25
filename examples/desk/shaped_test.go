// Package desk holds no code: each desk tool is its own Go module. This
// test lives in the bashy module because it reads every tool's shaped.json
// with yoke's own loader — the one `bashy app add --type shaped` and the
// console use — so a manifest that drifts from yoke fails here first.
package desk

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/shape/appserver"
)

// shapedTools maps each desk tool to its app root: the directory passed to
// `bashy app add NAME --type shaped --source DIR`, which holds shaped.json
// and the go.mod the console builds from.
var shapedTools = map[string]string{
	"notepad":    "notepad",
	"chart":      "chart",
	"form":       "form",
	"viewer":     "viewer",
	"whiteboard": "../whiteboard",
	"brainstorm": "../brainstorm/server",
	"session":    "../session",
	"calendar":   "../calendar",
	"page":       "page",
}

func TestConsumerPolishedToolsDeclareGenieExamples(t *testing.T) {
	for _, name := range []string{"calendar", "form", "notepad", "page", "whiteboard"} {
		t.Run(name, func(t *testing.T) {
			root := shapedTools[name]
			b, err := os.ReadFile(filepath.Join(root, appserver.File))
			if err != nil {
				t.Fatalf("%s: read %s: %v", name, appserver.File, err)
			}
			m, err := appserver.Parse(b)
			if err != nil {
				t.Fatalf("%s: parse %s: %v", name, appserver.File, err)
			}
			if m.Genie == nil || len(m.Genie.Examples) < 2 {
				t.Fatalf("%s: %s must declare at least 2 genie.examples; got %+v", name, appserver.File, m.Genie)
			}
		})
	}
}

func TestFormShapewareCoreDigest(t *testing.T) {
	shapedBytes, err := os.ReadFile(filepath.Join("form", appserver.File))
	if err != nil {
		t.Fatalf("read form/shaped.json: %v", err)
	}
	m, err := appserver.Parse(shapedBytes)
	if err != nil {
		t.Fatalf("appserver.Parse(form/shaped.json): %v", err)
	}
	if m.Server == nil || !strings.HasPrefix(m.Server.Digest, "sha256:") || len(m.Server.Digest) != len("sha256:")+64 {
		t.Fatalf("form shaped.json server.digest = %q, want sha256:<64 hex>", m.Server.Digest)
	}
}

func TestDeskToolsShipShapedServer(t *testing.T) {
	for name, root := range shapedTools {
		t.Run(name, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join(root, appserver.File))
			if err != nil {
				t.Fatalf("%s must ship %s: %v", name, appserver.File, err)
			}
			m, err := appserver.Parse(b)
			if err != nil {
				t.Fatal(err)
			}
			if m.Server == nil {
				t.Fatalf("%s: %s declares no server", name, appserver.File)
			}
			// The console runs `go build ./MAIN` in the app root, so the
			// module must be rooted there, not above or beside it.
			if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
				t.Fatalf("%s: app root %s has no go.mod; the console cannot build ./%s", name, root, m.Server.Main)
			}
			mainDir := filepath.Join(root, filepath.FromSlash(m.Server.Main))
			if !hasMainFunc(t, mainDir) {
				t.Fatalf("%s: server.main %q is not a main package", name, m.Server.Main)
			}
			if testing.Short() {
				return
			}
			bin := buildLikeConsole(t, extractAlone(t, root), m.Server.Main, deskProxy(t))
			runLikeConsole(t, bin, *m.Server)
		})
	}
}

func hasMainFunc(t *testing.T, dir string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || filepath.Ext(n) != ".go" || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, n), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		if f.Name.Name == "main" && f.Scope.Lookup("main") != nil {
			return true
		}
	}
	return false
}

// buildLikeConsole mirrors yoke pkg/webconsole's server build: the fixed
// command, GOWORK=off, CGO_ENABLED=0, in the app root, shared modules from
// the local module proxy (gomod.BuildEnv). The module cache starts empty, so
// every module comes from a proxy: the desk proxy, then the host's own
// download cache (offline when warm), then the inherited GOPROXY.
func buildLikeConsole(t *testing.T, root, main, proxy string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "server")
	hostCache, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err != nil {
		t.Fatal(err)
	}
	upstream := os.Getenv("GOPROXY")
	if upstream == "" {
		upstream = "https://proxy.golang.org,direct"
	}
	cmd := exec.Command("go", "build", "-trimpath", "-o", bin, "./"+main)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0", "GOFLAGS=-modcacherw",
		"GOMODCACHE="+filepath.Join(t.TempDir(), "mod"),
		"GOPROXY="+fileURL(proxy)+","+fileURL(filepath.Join(strings.TrimSpace(string(hostCache)), "cache", "download"))+","+upstream,
		"GONOSUMDB=github.com/qiangli/bashy/examples")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./%s in %s: %v\n%s", main, root, err, out)
	}
	return bin
}

func fileURL(dir string) string {
	p := filepath.ToSlash(dir)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return "file://" + p
}

// extractAlone copies the files git tracks under root, and nothing else, to
// a fresh directory: the console builds a revision's tree with no sibling
// directory beside it.
func extractAlone(t *testing.T, root string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "ls-files", "-z", ".").Output()
	if err != nil {
		t.Fatalf("git ls-files %s: %v", root, err)
	}
	dst := filepath.Join(t.TempDir(), "src")
	for _, rel := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dst
}

var (
	proxyOnce sync.Once
	proxyDir  string
	proxyErr  string
)

// deskProxy publishes the shared desk modules (register-modules.sh, through
// the candidate bashy) into a proxy directory private to this test run.
func deskProxy(t *testing.T) string {
	t.Helper()
	proxyOnce.Do(func() {
		dir, err := os.MkdirTemp("", "desk-gomod-proxy-")
		if err != nil {
			proxyErr = err.Error()
			return
		}
		bin := os.Getenv("BASHY_BIN")
		if bin == "" {
			bin = productBashy(t)
		}
		cmd := exec.Command("bash", "register-modules.sh")
		cmd.Env = append(os.Environ(), "BASHY_BIN="+bin, "BASHY_GOMOD_PROXY="+dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			proxyErr = fmt.Sprintf("register-modules.sh: %v\n%s", err, out)
			return
		}
		proxyDir = dir
	})
	if proxyErr != "" {
		t.Fatal(proxyErr)
	}
	return proxyDir
}

// runLikeConsole starts the server with the manifest's argv on a loopback
// port and waits for the desk health probe.
func runLikeConsole(t *testing.T, bin string, srv appserver.Server) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, srv.Argv(addr, port, t.TempDir())...)
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = cmd.Wait() })
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := client.Get("http://" + addr + "/healthz"); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("server %v never answered /healthz on %s", srv.Args, addr)
}
