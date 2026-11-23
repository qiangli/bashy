package desk

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The shipped installers must take the same app-add route as an external
// shaped app. A principal can then activate and undo a personal revision.
func TestDeskRegisterScriptsUsePublicShapedAppPath(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip("runs the desk installers and the product CLI")
	}
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is required for a revision round trip")
	}
	bin := productBashy(t)
	for _, name := range []string{"notepad", "form", "viewer", "chart", "calendar", "page"} {
		t.Run(name, func(t *testing.T) {
			rel := name
			if v, ok := shapedTools[name]; ok {
				rel = v
			}
			root, err := filepath.Abs(rel)
			if err != nil {
				t.Fatal(err)
			}
			f := newForkHome(t, bin, gitBin)
			f.env = append(f.env, "BASHY_BIN="+bin)
			f.run(root, "bash", "register.sh")

			var app struct {
				Name, Type, Source string
			}
			out := f.bashy("app", "show", name, "--json")
			if err := json.Unmarshal([]byte(out), &app); err != nil {
				t.Fatalf("app show: %v\n%s", err, out)
			}
			repo := filepath.Join(f.home, "src", name)
			if app.Name != name || app.Type != "shaped" || app.Source != repo {
				t.Fatalf("registered app = %+v, want shaped managed source %s", app, repo)
			}
			if list := f.bashy("app", "list"); !strings.Contains(list, name+" ") || !strings.Contains(list, "registered") {
				t.Fatalf("app list lacks registered shaped app %s:\n%s", name, list)
			}

			if name == "viewer" || name == "chart" || name == "calendar" || name == "page" {
				assertRegisteredGenie(t, f, name)
			}

			first := f.fork(name, repo, "personal-edit-1", "first personal edit")
			f.bashy("src", "activate", name, first, "--principal", alice)
			if got := f.active(name); got != first {
				t.Fatalf("active revision = %s, want %s", got, first)
			}
			f.bashy("src", "pr", "merge", f.lastPR)
			second := f.fork(name, repo, "personal-edit-2", "second personal edit")
			f.bashy("src", "activate", name, second, "--principal", alice)
			if got := f.active(name); got != second {
				t.Fatalf("active revision = %s, want %s", got, second)
			}
			f.bashy("src", "undo", name, "--principal", alice)
			if got := f.active(name); got != first {
				t.Fatalf("after undo active = %s, want %s", got, first)
			}
		})
	}
}

// assertRegisteredGenie exercises the production proxy against an isolated
// registration. The loopback-only console and its app are test subprocesses.
func assertRegisteredGenie(t *testing.T, f *forkHome, name string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	log, err := os.CreateTemp(t.TempDir(), "console-*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	cmd := exec.CommandContext(ctx, f.bin, "app", "serve", "--bind", "127.0.0.1", "--port", port)
	cmd.Dir = filepath.Dir(f.home)
	cmd.Env = f.env
	cmd.Stdout, cmd.Stderr = log, log
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
		if t.Failed() {
			output, _ := os.ReadFile(log.Name())
			t.Logf("console log: %s", output)
		}
	}()

	client := &http.Client{}
	base := "http://127.0.0.1:" + port
	health, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}
	for {
		resp, err := client.Do(health)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("test console never became ready: %v", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/"+name+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %s: %s", req.URL.Path, resp.Status, body)
	}
	for _, marker := range []string{
		`<script src="/_shape/page.js"`,
		`<script src="/_shape/genie.js?v=`,
		`data-bashy-genie`,
		`data-app="` + name + `"`,
		`data-mode="live"`,
	} {
		if !strings.Contains(string(body), marker) {
			t.Errorf("GET %s: missing Genie marker %q", req.URL.Path, marker)
		}
	}
}
