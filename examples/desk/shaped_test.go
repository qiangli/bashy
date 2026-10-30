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
	"whiteboard": "../whiteboard/server",
	"brainstorm": "../brainstorm/server",
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
			bin := buildLikeConsole(t, root, m.Server.Main)
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
// command, GOWORK=off, CGO_ENABLED=0, in the app root.
func buildLikeConsole(t *testing.T, root, main string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "server")
	cmd := exec.Command("go", "build", "-trimpath", "-o", bin, "./"+main)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./%s in %s: %v\n%s", main, root, err, out)
	}
	return bin
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
