//go:build e2e

package agentos

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// appsEnv is scratchStoreEnv plus an isolated registered-app ring.
func appsEnv(t *testing.T) []string {
	t.Helper()
	return append(scratchStoreEnv(t), "BASHY_APPS_DIR="+filepath.Join(t.TempDir(), "apps"), "BASHY_APPS_PATH=")
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// The registered-app rod through the built binary: `app add` persists a
// record, `app show`/`app list` read it back, a stock mount is refused, a
// served console tiles it as ready and proxies /<name>/ to its port, and
// `app rm` takes it away again.
func TestE2ERegisteredAppFlow(t *testing.T) {
	bin := bashyBinary(t)
	env := appsEnv(t)

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "upstream "+r.URL.Path)
	}))
	defer up.Close()
	_, upPort, _ := net.SplitHostPort(up.Listener.Addr().String())

	if _, stderr, code := runBashyStdEnv(bin, env, "app", "add", "pyhttp", "--port", upPort, "--label", "Py HTTP"); code != 0 {
		t.Fatalf("app add (exit %d): %s", code, stderr)
	}
	stdout, stderr, code := runBashyStdEnv(bin, env, "app", "show", "pyhttp", "--json")
	var rec struct {
		Name, Kind, Label string
		Port              int
	}
	if code != 0 || json.Unmarshal([]byte(stdout), &rec) != nil {
		t.Fatalf("app show --json (exit %d): %s %s", code, stdout, stderr)
	}
	if rec.Name != "pyhttp" || rec.Kind != "app" || rec.Label != "Py HTTP" || strconv.Itoa(rec.Port) != upPort {
		t.Errorf("record = %+v", rec)
	}
	stdout, _, _ = runBashyStdEnv(bin, env, "app", "list")
	if !listHas(stdout, "pyhttp", "registered") {
		t.Errorf("app list lacks the registered row:\n%s", stdout)
	}
	// A registered app never takes a mount bashy ships.
	if _, stderr, code := runBashyStdEnv(bin, env, "app", "add", "files", "--port", upPort); code == 0 || !strings.Contains(stderr, "files") {
		t.Errorf("stock mount accepted (exit %d): %s", code, stderr)
	}

	// Serve and go through the console exactly as a browser would.
	port := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := exec.CommandContext(ctx, bin, "app", "serve", "--port", strconv.Itoa(port))
	srv.Env = append(append(srv.Environ(), "BASHY_AGENTIC=1"), env...)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = srv.Wait() }()
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	waitHealthy(t, base+"/healthz")

	var apps struct {
		Apps []struct{ Name, Source, Status string } `json:"apps"`
	}
	getJSON(t, base+"/api/apps", &apps)
	found := false
	for _, a := range apps.Apps {
		if a.Name == "pyhttp" {
			found = true
			if a.Source != "registered" || a.Status != "ready" {
				t.Errorf("/api/apps pyhttp = %+v", a)
			}
		}
	}
	if !found {
		t.Errorf("/api/apps lacks pyhttp: %+v", apps.Apps)
	}
	if body := getBody(t, base+"/pyhttp/x"); body != "upstream /x" {
		t.Errorf("proxy body = %q", body)
	}

	if _, stderr, code := runBashyStdEnv(bin, env, "app", "rm", "pyhttp"); code != 0 {
		t.Fatalf("app rm (exit %d): %s", code, stderr)
	}
	stdout, _, _ = runBashyStdEnv(bin, env, "app", "list")
	if strings.Contains(stdout, "pyhttp") {
		t.Errorf("app list still shows pyhttp after rm:\n%s", stdout)
	}
}

func listHas(table, name, source string) bool {
	for _, line := range strings.Split(table, "\n") {
		f := strings.Fields(line)
		if len(f) >= 5 && f[0] == name && f[4] == source {
			return true
		}
	}
	return false
}

func waitHealthy(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := http.Get(url); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("console never became healthy at %s", url)
}

func getBody(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", url, resp.StatusCode, b)
	}
	return string(b)
}

func getJSON(t *testing.T, url string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(getBody(t, url)), v); err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
}
