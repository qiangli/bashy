// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCollectContextIncludesBashyPathAndCapabilities(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "1")
	report := collectContext()
	if report.SchemaVersion != contextSchemaVersion {
		t.Fatalf("schema = %q", report.SchemaVersion)
	}
	if report.BashyPath == "" || !filepath.IsAbs(report.BashyPath) {
		t.Fatalf("bashy path should be absolute, got %q", report.BashyPath)
	}
	if !report.Mode.Agentic {
		t.Fatalf("agentic mode not detected: %#v", report.Mode)
	}
	if !report.Capabilities.DryRun || !report.Capabilities.CheckAgentJSON || !report.Capabilities.CommandFeatures {
		t.Fatalf("missing expected capabilities: %#v", report.Capabilities)
	}
	if !report.Capabilities.UnifiedInbox {
		t.Fatalf("unified inbox capability is not discoverable: %#v", report.Capabilities)
	}
	// The recommended commands must embed the absolute bashy path (cross-platform:
	// a Windows path is C:\… not /…, so check containment of BashyPath, not a
	// leading slash).
	if report.BashyPath == "" || !filepath.IsAbs(report.BashyPath) {
		t.Fatalf("bashy_path should be absolute, got %q", report.BashyPath)
	}
	var sawPathInCommand, sawInbox, sawProtocol bool
	for _, cmd := range report.RecommendedCommands {
		if strings.Contains(cmd.Command, report.BashyPath) {
			sawPathInCommand = true
		}
		if strings.HasSuffix(cmd.Command, " inbox") && strings.Contains(strings.ToLower(cmd.Purpose), "attention") {
			sawInbox = true
		}
		if strings.HasSuffix(cmd.Command, " skills show inbox") {
			sawProtocol = true
		}
	}
	if !sawPathInCommand {
		t.Fatalf("recommended commands should include the absolute bashy path %q: %#v", report.BashyPath, report.RecommendedCommands)
	}
	if !sawInbox || !sawProtocol {
		t.Fatalf("context did not route agents through unified inbox + response protocol: %#v", report.RecommendedCommands)
	}
}

func TestContextReportJSON(t *testing.T) {
	b, err := json.Marshal(collectContext())
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		SchemaVersion string `json:"schema_version"`
		BashyPath     string `json:"bashy_path"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatalf("context JSON invalid: %v\n%s", err, b)
	}
	if payload.SchemaVersion != contextSchemaVersion || payload.BashyPath == "" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestDetectProjectRootMarkerlessDeadline(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "sub", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	done := make(chan string, 1)
	go func() {
		done <- detectProjectRoot(nested)
	}()

	select {
	case <-time.After(5 * time.Second):
		t.Fatal("detectProjectRoot hung on markerless temp directory; exceeded 5s deadline")
	case res := <-done:
		// When no marker exists up to the root, detectProjectRoot returns empty string
		// (or the root if running inside an existing repository with higher-level markers).
		// Crucially, the walk must terminate without hanging.
		_ = res
	}
}

func TestDetectProjectRootHelperWindowsRoots(t *testing.T) {
	// winDir simulates filepath.Dir on Windows:
	// "C:\Users\agent\sub\dir" -> "C:\Users\agent\sub" -> "C:\Users\agent" -> "C:\Users" -> "C:\" -> "C:\"
	// "\\server\share\sub\dir" -> "\\server\share\sub" -> "\\server\share" -> "\\server\share"
	winDir := func(p string) string {
		if p == `C:\` || p == `C:` {
			return `C:\`
		}
		if p == `\\server\share` || p == `\\server\share\` {
			return `\\server\share`
		}
		p = strings.TrimRight(p, `\`)
		idx := strings.LastIndex(p, `\`)
		if idx < 0 {
			return "."
		}
		parent := p[:idx]
		if strings.HasSuffix(parent, ":") { // e.g. "C:" -> "C:\"
			parent += `\`
		}
		return parent
	}

	t.Run("markerless drive root terminates without spinning", func(t *testing.T) {
		done := make(chan string, 1)
		go func() {
			done <- detectProjectRootWith(`C:\Users\agent\sub\dir`, winDir, func(dir string) bool {
				return false
			})
		}()
		select {
		case <-time.After(5 * time.Second):
			t.Fatal("detectProjectRootWith hung on markerless Windows path; exceeded 5s deadline")
		case got := <-done:
			if got != "" {
				t.Fatalf("expected empty string for markerless path, got %q", got)
			}
		}
	})

	t.Run("marker at intermediate directory", func(t *testing.T) {
		got := detectProjectRootWith(`C:\Users\agent\sub\dir`, winDir, func(dir string) bool {
			return dir == `C:\Users\agent`
		})
		if got != `C:\Users\agent` {
			t.Fatalf("expected C:\\Users\\agent, got %q", got)
		}
	})

	t.Run("marker at drive root", func(t *testing.T) {
		got := detectProjectRootWith(`C:\Users\agent\sub\dir`, winDir, func(dir string) bool {
			return dir == `C:\`
		})
		if got != `C:\` {
			t.Fatalf("expected C:\\, got %q", got)
		}
	})

	t.Run("markerless UNC path terminates", func(t *testing.T) {
		got := detectProjectRootWith(`\\server\share\sub\dir`, winDir, func(dir string) bool {
			return false
		})
		if got != "" {
			t.Fatalf("expected empty string for markerless UNC path, got %q", got)
		}
	})

	t.Run("marker at UNC share root", func(t *testing.T) {
		got := detectProjectRootWith(`\\server\share\sub\dir`, winDir, func(dir string) bool {
			return dir == `\\server\share`
		})
		if got != `\\server\share` {
			t.Fatalf("expected \\\\server\\share, got %q", got)
		}
	})

	t.Run("empty cwd returns empty string immediately", func(t *testing.T) {
		got := detectProjectRootWith("", winDir, func(dir string) bool {
			return true
		})
		if got != "" {
			t.Fatalf("expected empty string for empty cwd, got %q", got)
		}
	})
}

func TestInspectContextRedirectedStdio(t *testing.T) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	// Drain concurrently: Windows pipes buffer ~4KB, so a reader that waits
	// for the command to return deadlocks any larger report.
	type readResult struct {
		b   []byte
		err error
	}
	read := make(chan readResult, 1)
	go func() {
		b, err := io.ReadAll(r)
		read <- readResult{b, err}
	}()

	done := make(chan int, 1)
	go func() {
		done <- dispatchContext([]string{"--json"})
	}()

	var code int
	select {
	case <-time.After(5 * time.Second):
		os.Stdout = oldStdout
		w.Close()
		r.Close()
		t.Fatal("dispatchContext --json hung with redirected stdout; exceeded 5s deadline")
	case code = <-done:
	}

	w.Close()
	os.Stdout = oldStdout
	rr := <-read
	r.Close()
	out, err := rr.b, rr.err
	if err != nil {
		t.Fatal(err)
	}

	if code != 0 {
		t.Fatalf("dispatchContext returned exit code %d", code)
	}
	var res map[string]any
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("dispatchContext emitted invalid JSON: %v\nOutput: %s", err, string(out))
	}
	if schema, ok := res["schema_version"].(string); !ok || schema != contextSchemaVersion {
		t.Fatalf("unexpected schema_version: %v", res["schema_version"])
	}
}
