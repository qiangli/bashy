// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build e2e

// End-to-end: build the real bashy binary and confirm every command that
// `bashy commands` advertises actually dispatches — no "No such command" / "not in
// this build" / "No such file". Runs identically on macOS and Windows via
//
//	go test -tags e2e ./internal/agentos/
package agentos

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// unsupportedSignals are substrings that mean a command did NOT dispatch to a real
// handler (the failure modes this goal forbids). Checked on verb `--help` output
// (cobra help is clean, so any of these means a broken/absent handler) and on the
// coreutils execution smoke.
var unsupportedSignals = []string{
	"no such command",           // bashy tool dispatch, unregistered
	"not in this build",         // engine stub, old behavior
	"rebuild with -tags",        // engine stub, old behavior
	"no such file or directory", // verb fell through to exec its own name (the docker bug)
	"command not found",         // verb/tool fell through to PATH and wasn't found
}

var (
	buildOnce sync.Once
	builtBin  string
	buildErr  error
)

func bashyBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		// Escape hatch: a prebuilt binary (e.g. cross-compiled bashy.exe shipped
		// to a Windows host that has no Go toolchain/source tree). Lets the same
		// dispatch e2e run on macOS and Windows identically.
		if p := strings.TrimSpace(os.Getenv("BASHY_E2E_BIN")); p != "" {
			builtBin = p
			return
		}
		root, ok := findBashySourceRoot(mustGetwd())
		if !ok {
			buildErr = os.ErrNotExist
			return
		}
		out := filepath.Join(os.TempDir(), "bashy-e2e-cmds")
		if runtime.GOOS == "windows" {
			out += ".exe"
		}
		cmd := exec.Command("go", "build", "-o", out, "./cmd/bashy")
		cmd.Dir = root
		if b, err := cmd.CombinedOutput(); err != nil {
			buildErr = err
			t.Logf("build output:\n%s", b)
			return
		}
		builtBin = out
	})
	if buildErr != nil {
		t.Fatalf("build bashy: %v", buildErr)
	}
	return builtBin
}

func runBashy(bin string, args ...string) (string, int) {
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "BASHY_AGENTIC=1")
	cmd.Stdin = strings.NewReader("") // empty stdin so stdin-reading tools never hang
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	}
	return string(out), code
}

func unsupportedSignal(out string) string {
	lo := strings.ToLower(out)
	for _, s := range unsupportedSignals {
		if strings.Contains(lo, s) {
			return s
		}
	}
	return ""
}

func firstLineOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func featureAvailable(bin, name string) (bool, string) {
	o, _ := runBashy(bin, "commands", name, "--features", "--json")
	var info map[string]any
	if err := json.Unmarshal([]byte(o), &info); err != nil {
		return false, "no feature report"
	}
	class, _ := info["class"].(string)
	return info["available"] == true && class != "not-found", class
}

// TestE2EAllListedCommandsDispatch runs the real binary and checks every command
// `bashy commands` prints:
//   - coreutils tools: recognized+available per the binary (all of them), plus a
//     live execution smoke proves the in-process userland actually runs;
//   - native + engine front-door verbs: really invoked (`--help`), asserting no
//     unsupported signal (the docker/podman regression class);
//   - download/passthrough verbs: dispatch recognition (so we don't pull hundreds
//     of MB of upstream tools).
func TestE2EAllListedCommandsDispatch(t *testing.T) {
	bin := bashyBinary(t)

	out, code := runBashy(bin, "commands", "--json")
	if code != 0 {
		t.Fatalf("`bashy commands --json` exited %d:\n%s", code, out)
	}
	var cat struct {
		Builtins  []string `json:"builtins"`
		Coreutils []string `json:"coreutils"`
		Verbs     []string `json:"verbs"`
	}
	if err := json.Unmarshal([]byte(out), &cat); err != nil {
		t.Fatalf("decode commands json: %v\n%s", err, out)
	}
	if len(cat.Coreutils) == 0 || len(cat.Verbs) == 0 {
		t.Fatalf("empty catalog: %d coreutils, %d verbs", len(cat.Coreutils), len(cat.Verbs))
	}

	// Builtins: a sample resolves as a builtin.
	for _, b := range []string{"cd", "export", "echo", "read"} {
		if !contains(cat.Builtins, b) {
			continue
		}
		if o, _ := runBashy(bin, "-c", "type -t "+b); strings.TrimSpace(o) != "builtin" {
			t.Errorf("builtin %q did not resolve as a builtin: %q", b, strings.TrimSpace(o))
		}
	}

	// coreutils: every listed tool is recognized+available per the binary. (--help
	// is not universal across tools, and stdin-readers would hang, so per-tool we
	// check dispatch recognition, not execution.)
	for _, tool := range cat.Coreutils {
		if ok, class := featureAvailable(bin, tool); !ok {
			t.Errorf("coreutils %q is listed but not available per dispatch (class=%s)", tool, class)
		}
	}
	// ...and a live smoke proves the in-process userland actually runs.
	smoke, code := runBashy(bin, "-c", `printf 'a\nb\n' | grep b | tr a-z A-Z | wc -l`)
	if s := unsupportedSignal(smoke); s != "" || code != 0 {
		t.Errorf("coreutils execution smoke failed (%q, exit %d): %s", s, code, firstLineOf(smoke))
	}

	// Native front-door verbs (safe, side-effect-free `--help`) + engine verbs
	// (docker/podman/ollama — the regression class) are really invoked.
	native := set("weave", "sprint", "chat", "agent", "sdlc", "web", "dag",
		"schedule", "secrets", "skills", "run", "commands", "context", "doctor",
		"self", "check", "verify", "git")
	engine := set("podman", "ollama", "docker")

	for _, v := range cat.Verbs {
		switch {
		case engine[v], native[v]:
			o, _ := runBashy(bin, v, "--help")
			if s := unsupportedSignal(o); s != "" {
				t.Errorf("verb %q is listed but unsupported (%q): %s", v, s, firstLineOf(o))
			}
		default:
			// Download/passthrough verb (gh/act/rclone/loom/zot/seaweedfs/kopia/
			// mirror/go/cmake/clang): confirm dispatch recognition without pulling
			// the upstream tool.
			if ok, class := featureAvailable(bin, v); !ok {
				t.Errorf("verb %q is listed but dispatch does not recognize it (class=%s)", v, class)
			}
		}
	}
}

func set(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
