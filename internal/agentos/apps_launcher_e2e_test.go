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
)

func TestE2ECustomLauncher(t *testing.T) {
	bin := bashyBinary(t)
	env := appsEnv(t)
	dir, err := filepath.Abs(filepath.Join("..", "..", "examples", "launcher"))
	if err != nil {
		t.Fatal(err)
	}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "example upstream "+r.URL.Path) }))
	defer up.Close()
	_, upPort, _ := net.SplitHostPort(up.Listener.Addr().String())
	port := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := exec.CommandContext(ctx, bin, "app", "serve", "--port", strconv.Itoa(port), "--launcher", dir)
	srv.Env = append(append(srv.Environ(), "BASHY_AGENTIC=1"), env...)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = srv.Wait() }()
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	waitHealthy(t, base+"/healthz")
	if body := getBody(t, base+"/"); !strings.Contains(body, "Example bashy launcher") || !strings.Contains(body, `<base href="/">`) {
		t.Fatalf("example page = %s", body)
	}
	_ = getBody(t, base+"/api/apps") // prime the cache before adding
	request := func(method, path, body string, want int) {
		t.Helper()
		r, err := http.NewRequest(method, base+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		b, _ := io.ReadAll(response.Body)
		if response.StatusCode != want {
			t.Fatalf("%s %s = %d: %s", method, path, response.StatusCode, b)
		}
	}
	request("POST", "/api/apps/registered", `{"name":"exampleapp","port":`+upPort+`,"label":"Example"}`, 201)
	var apps struct {
		Apps []struct{ Name, Source, Status string } `json:"apps"`
	}
	getJSON(t, base+"/api/apps", &apps)
	found := false
	for _, app := range apps.Apps {
		if app.Name == "exampleapp" {
			found = app.Source == "registered" && app.Status == "ready"
		}
	}
	if !found {
		t.Fatalf("live registered row missing: %+v", apps.Apps)
	}
	stdout, stderr, code := runBashyStdEnv(bin, env, "app", "list", "--json")
	if code != 0 || json.Unmarshal([]byte(stdout), &apps) != nil {
		t.Fatalf("app list --json: %d %s %s", code, stdout, stderr)
	}
	found = false
	for _, app := range apps.Apps {
		if app.Name == "exampleapp" {
			found = true
		}
	}
	if !found {
		t.Fatal("CLI JSON lacks API registration")
	}
	if body := getBody(t, base+"/exampleapp/x"); body != "example upstream /x" {
		t.Fatalf("immediate proxy = %q", body)
	}
	request("DELETE", "/api/apps/registered/exampleapp", "", 204)
	getJSON(t, base+"/api/apps", &apps)
	for _, app := range apps.Apps {
		if app.Name == "exampleapp" {
			t.Fatal("deleted app still listed")
		}
	}
	response, err := http.Get(base + "/exampleapp/x")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if strings.Contains(string(body), "example upstream") {
		t.Fatal("deleted app still proxied")
	}
}
