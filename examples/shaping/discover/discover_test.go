// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Script tests for the discover Bash# command (Sprint 693 S2a). They run
// under the installed bashy (`bashy --bashsharp`; skipped when bashy is
// not on PATH) against the bundled testdata fixture only: `serve` starts
// the request-logging fixture server on a free port, `matrix` runs the
// non-agent clients plus a fake agent CLI (a PATH shim that curls the URL
// with a distinctive User-Agent), and the assertions read the matrix's
// own JSON/Markdown output and the server request log. No network beyond
// loopback, no real agent CLI, no browser.
package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func bashyBin(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fixtures are POSIX shell scripts")
	}
	bin, err := exec.LookPath("bashy")
	if err != nil {
		t.Skip("bashy is not on PATH")
	}
	return bin
}

func scriptPath(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "discover.bsh")
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// startFixture serves the bundled testdata on a free port and waits until
// robots.txt answers. It returns the origin and the request-log path.
func startFixture(t *testing.T) (origin, logPath string) {
	t.Helper()
	bin := bashyBin(t)
	port := freePort(t)
	logPath = filepath.Join(t.TempDir(), "requests.log")
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, "--bashsharp", scriptPath(t),
		"serve", "--log", logPath, "--port", fmt.Sprint(port))
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = cmd.Wait() })
	origin = fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		out, err := exec.Command("curl", "-fsS", "-m", "2", origin+"/robots.txt").Output()
		if err == nil && strings.Contains(string(out), "Allow: /") {
			return origin, logPath
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("fixture server did not answer /robots.txt")
	return "", ""
}

func runMatrix(t *testing.T, env []string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bashyBin(t),
		append([]string{"--bashsharp", scriptPath(t), "matrix"}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("matrix %v: %v", args, err)
	}
	return string(out)
}

type row struct {
	Client    string `json:"client"`
	Status    string `json:"status"`
	Method    string `json:"method"`
	Accept    string `json:"accept"`
	UserAgent string `json:"user_agent"`
	CT        string `json:"content_type"`
	ETag      string `json:"etag"`
	Vary      string `json:"vary"`
	Sections  string `json:"sections"`
	Links     string `json:"links"`
	Discovery string `json:"discovery"`
	LLMS      string `json:"llms_txt"`
	Alternate string `json:"followed_alternate"`
}

func TestMatrixAgainstFixture(t *testing.T) {
	origin, logPath := startFixture(t)
	out := runMatrix(t, nil, origin,
		"--clients", "curl,curl-cc,bashy-fetch,bashy-fetch-md",
		"--log", logPath, "--json")
	var doc struct {
		Origin  string `json:"origin"`
		Page    string `json:"page"`
		Clients []row  `json:"clients"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("matrix --json does not parse: %v\n%s", err, out)
	}
	if doc.Page != origin+"/about" {
		t.Errorf("page = %q, want %q", doc.Page, origin+"/about")
	}
	if len(doc.Clients) != 4 {
		t.Fatalf("clients = %d, want 4\n%s", len(doc.Clients), out)
	}
	by := map[string]row{}
	for _, r := range doc.Clients {
		by[r.Client] = r
	}
	for _, id := range []string{"curl", "curl-cc", "bashy-fetch", "bashy-fetch-md"} {
		if by[id].Status != "ok" {
			t.Errorf("%s status=%q, want ok", id, by[id].Status)
		}
		if !strings.Contains(by[id].Sections, "team") {
			t.Errorf("%s sections=%q, want team", id, by[id].Sections)
		}
		if !strings.Contains(by[id].Links, "/about") {
			t.Errorf("%s links=%q, want /about", id, by[id].Links)
		}
		if by[id].LLMS != "no" || by[id].Alternate != "no" {
			t.Errorf("%s llms/alternate=%q/%q, want no/no (log-recorded)",
				id, by[id].LLMS, by[id].Alternate)
		}
	}
	c := by["curl"]
	if !strings.Contains(c.CT, "text/html") || c.Discovery != "twin" {
		t.Errorf("curl ct=%q discovery=%q, want text/html + twin", c.CT, c.Discovery)
	}
	if c.Accept != "*/*" || !strings.HasPrefix(c.UserAgent, "curl/") {
		t.Errorf("curl request accept=%q ua=%q, want */* + curl/* (from log)", c.Accept, c.UserAgent)
	}
	cc := by["curl-cc"]
	if !strings.Contains(cc.CT, "text/markdown") || cc.Discovery != "self" {
		t.Errorf("curl-cc ct=%q discovery=%q, want markdown + self", cc.CT, cc.Discovery)
	}
	if !strings.Contains(cc.Accept, "text/markdown") {
		t.Errorf("curl-cc accept=%q, want the Claude Code header", cc.Accept)
	}
	for _, id := range []string{"curl", "curl-cc", "bashy-fetch"} {
		if !strings.Contains(by[id].ETag, "fixture-rev-1") || by[id].Vary != "Accept" {
			t.Errorf("%s etag=%q vary=%q, want fixture-rev-1 + Accept", id, by[id].ETag, by[id].Vary)
		}
	}
	if by["bashy-fetch-md"].Discovery != "self-md" {
		t.Errorf("bashy-fetch-md discovery=%q, want self-md", by["bashy-fetch-md"].Discovery)
	}
	// Markdown shape: one table, paste-ready for the evidence doc.
	md := runMatrix(t, nil, origin, "--clients", "curl", "--log", logPath, "--md")
	if !strings.Contains(md, "| Client | Status |") || !strings.Contains(md, "| curl | ok |") {
		t.Errorf("markdown table missing curl row:\n%s", md)
	}
}

// The agent leg: a fake CLI on PATH curls the page with a distinctive
// User-Agent; the row must carry those headers FROM the server log.
func TestAgentLegHeadersFromLog(t *testing.T) {
	origin, logPath := startFixture(t)
	binDir := t.TempDir()
	fake := "#!/bin/sh\n" +
		"url=$(printf '%s' \"$*\" | grep -o 'http://[^ ]*' | head -n 1)\n" +
		"curl -sS -A 'FakeClaude/1.0' -H 'Accept: text/html' \"$url\" >/dev/null\n" +
		"echo 'CONTENT-TYPE: text/html; charset=utf-8'\n" +
		"echo 'SECTIONS: about,team,contact'\n" +
		"echo 'LINKS: /, /about'\n" +
		"echo 'DISCOVERY: twin'\n" +
		"echo 'FETCHED-LLMS: no'\n"
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	out := runMatrix(t, []string{"PATH=" + binDir + ":" + os.Getenv("PATH")},
		origin, "--clients", "claude,gemini", "--log", logPath, "--json")
	var doc struct {
		Clients []row `json:"clients"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("agent matrix --json does not parse: %v\n%s", err, out)
	}
	by := map[string]row{}
	for _, r := range doc.Clients {
		by[r.Client] = r
	}
	cl := by["claude"]
	if cl.Status != "ok" {
		t.Fatalf("fake claude status=%q, want ok\n%s", cl.Status, out)
	}
	if cl.UserAgent != "FakeClaude/1.0" || cl.Accept != "text/html" {
		t.Errorf("claude headers ua=%q accept=%q, want FakeClaude/1.0 + text/html (from log)",
			cl.UserAgent, cl.Accept)
	}
	if !strings.Contains(cl.Sections, "team") || cl.Discovery != "twin" {
		t.Errorf("claude sections=%q discovery=%q, want team + twin", cl.Sections, cl.Discovery)
	}
	if by["gemini"].Status != "skipped" {
		t.Errorf("absent gemini status=%q, want skipped (never a failure)", by["gemini"].Status)
	}
}

func TestDryRunAndUsage(t *testing.T) {
	bin := bashyBin(t)
	out, err := exec.Command(bin, "--bashsharp", scriptPath(t),
		"matrix", "http://127.0.0.1:9", "--clients", "curl,browser", "--dry-run").CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "would run:") || !strings.Contains(string(out), "| curl | dry-run |") {
		t.Errorf("dry-run output missing would-run rows:\n%s", out)
	}
	if err := exec.Command(bin, "--bashsharp", scriptPath(t)).Run(); err == nil {
		t.Error("no-verb invocation must fail with usage")
	}
}
