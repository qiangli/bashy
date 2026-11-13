// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Hermetic tests for examples/chrome/chrome.bsh across darwin, linux, and windows.
package chrome_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	buildOnce sync.Once
	buildBin  string
	buildErr  error
	buildDir  string
)

func TestMain(m *testing.M) {
	var err error
	buildDir, err = os.MkdirTemp("", "bashy-chrome-*")
	if err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(buildDir)
	os.Exit(code)
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "cmd", "bashy")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("bashy repository root (cmd/bashy) not found above test file")
		}
		dir = parent
	}
}

func bashyBin(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		ext := ""
		if runtime.GOOS == "windows" {
			ext = ".exe"
		}
		bin := filepath.Join(buildDir, "bashy"+ext)
		cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/bashy")
		cmd.Dir = moduleRoot(t)
		if workspace := filepath.Join(filepath.Dir(cmd.Dir), "go.work"); fileExists(workspace) {
			cmd.Env = append(os.Environ(), "GOWORK="+workspace)
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = &buildError{err, string(out)}
			return
		}
		buildBin = bin
	})
	if buildErr != nil {
		t.Fatalf("build ./cmd/bashy: %v", buildErr)
	}
	return buildBin
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }

func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }

type runner struct {
	t      *testing.T
	bin    string
	script string
	root   string
	env    []string
}

func newRunner(t *testing.T, targetOS, targetArch string) *runner {
	t.Helper()
	root := t.TempDir()
	return &runner{
		t:      t,
		bin:    bashyBin(t),
		script: filepath.Join(moduleRoot(t), "examples", "chrome", "chrome.bsh"),
		root:   root,
		env: []string{
			"CHROME=",
			"CHROME_TEST_OS=" + targetOS,
			"CHROME_TEST_ARCH=" + targetArch,
			"CHROME_TEST_ROOT=" + filepath.ToSlash(root),
			"CHROME_NO_SYSTEM_PATH=1",
			"BASHY_HINTS=off",
		},
	}
}

func (r *runner) writeFakeChrome(rel string) string {
	r.t.Helper()
	full := filepath.Join(r.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		r.t.Fatal(err)
	}
	body := "#!/bin/sh\necho 'Google Chrome 136.0.7103.92'\n"
	if err := os.WriteFile(full, []byte(body), 0o755); err != nil {
		r.t.Fatal(err)
	}
	return filepath.ToSlash(full)
}

func (r *runner) run(extraEnv []string, args ...string) (string, int) {
	r.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.bin, append([]string{"--bashsharp", r.script}, args...)...)
	cmd.Env = append(os.Environ(), r.env...)
	cmd.Env = append(cmd.Env, extraEnv...)
	cmd.Dir = r.root
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		r.t.Fatalf("run %v: %v", args, err)
	}
	return string(out), code
}

func TestSkillFence(t *testing.T) {
	r := newRunner(t, "darwin", "arm64")
	out, code := r.run(nil, "--skill")
	if code != 0 {
		t.Fatalf("--skill exit %d:\n%s", code, out)
	}
	for _, want := range []string{"name: chrome", "bashy chrome", "bashy chrome-open", "ensure", "status", "doctor", "path", "open", "chrome-browser"} {
		if !strings.Contains(out, want) {
			t.Errorf("--skill output missing %q:\n%s", want, out)
		}
	}
}

func TestEnsureAlreadyInstalledSkipsInstallAcrossOS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake chrome script")
	}
	cases := []struct {
		os   string
		arch string
		rel  string
	}{
		{"darwin", "arm64", "Applications/Google Chrome.app/Contents/MacOS/Google Chrome"},
		{"linux", "x86_64", "usr/bin/google-chrome-stable"},
		{"windows", "x86_64", "c/Program Files/Google/Chrome/Application/chrome.exe"},
	}
	for _, tc := range cases {
		t.Run(tc.os+"_"+tc.arch, func(t *testing.T) {
			r := newRunner(t, tc.os, tc.arch)
			wantPath := r.writeFakeChrome(tc.rel)
			marker := filepath.Join(r.root, "installed-called")
			failIfCalled := "CHROME_INSTALL_CMD=touch " + filepath.ToSlash(marker) + "; exit 99"

			// Default invocation (no subcommand) and explicit `ensure` / `install` must detect first and exit 0.
			for _, argv := range [][]string{nil, {"ensure"}, {"install"}} {
				out, code := r.run([]string{failIfCalled}, argv...)
				if code != 0 {
					t.Fatalf("%v exit %d:\n%s", argv, code, out)
				}
				if !strings.Contains(out, "already installed at "+wantPath) {
					t.Errorf("%v output = %q, want 'already installed at %s'", argv, out, wantPath)
				}
				if fileExists(marker) {
					t.Fatalf("%v invoked installer even though Chrome was already installed", argv)
				}
			}

			// JSON output on ensure when already installed.
			out, code := r.run([]string{failIfCalled}, "ensure", "--json")
			if code != 0 {
				t.Fatalf("ensure --json exit %d:\n%s", code, out)
			}
			var got map[string]any
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("unmarshal %q: %v", out, err)
			}
			if got["status"] != "installed" || got["installed"] != true || got["action"] != "none" || got["path"] != wantPath {
				t.Errorf("unexpected ensure --json payload: %#v", got)
			}

			// `path` returns the exact binary path.
			pOut, pCode := r.run(nil, "path")
			if pCode != 0 || strings.TrimSpace(pOut) != wantPath {
				t.Errorf("path = (%q, %d), want (%q, 0)", strings.TrimSpace(pOut), pCode, wantPath)
			}
		})
	}
}

func TestMissingStatusDoctorDryRunAndInstallAcrossOS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake installer script")
	}
	cases := []struct {
		os       string
		arch     string
		rel      string
		wantHint string
	}{
		{"darwin", "arm64", "Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "brew install --cask google-chrome"},
		{"linux", "x86_64", "usr/bin/google-chrome-stable", "google-chrome-stable_current_amd64.deb"},
		{"linux", "arm64", "usr/bin/chromium", "chromium"},
		{"windows", "x86_64", "c/Program Files/Google/Chrome/Application/chrome.exe", "winget install --id Google.Chrome"},
	}
	for _, tc := range cases {
		t.Run(tc.os+"_"+tc.arch, func(t *testing.T) {
			r := newRunner(t, tc.os, tc.arch)

			// 1. status when absent exits 1.
			out, code := r.run(nil, "status")
			if code == 0 || !strings.Contains(out, tc.wantHint) {
				t.Fatalf("status when missing = (%d, %q), want non-zero with %q", code, out, tc.wantHint)
			}

			// 2. doctor --json when absent exits 1 with bashy-svc-doctor-v1.
			docOut, docCode := r.run(nil, "doctor", "--json")
			if docCode == 0 {
				t.Fatalf("doctor --json when missing succeeded:\n%s", docOut)
			}
			var doc struct {
				SchemaVersion string `json:"schema_version"`
				Available     bool   `json:"available"`
				Missing       []struct {
					Need    string `json:"need"`
					Install string `json:"install"`
				} `json:"missing"`
			}
			if err := json.Unmarshal([]byte(docOut), &doc); err != nil {
				t.Fatalf("unmarshal doctor %q: %v", docOut, err)
			}
			if doc.SchemaVersion != "bashy-svc-doctor-v1" || doc.Available || len(doc.Missing) != 1 || !strings.Contains(doc.Missing[0].Install, tc.wantHint) {
				t.Errorf("unexpected doctor payload: %+v", doc)
			}

			// 3. ensure --dry-run --json exits 0 without installing.
			dryOut, dryCode := r.run(nil, "ensure", "--dry-run", "--json")
			if dryCode != 0 {
				t.Fatalf("ensure --dry-run --json exit %d:\n%s", dryCode, dryOut)
			}
			var dry map[string]any
			if err := json.Unmarshal([]byte(dryOut), &dry); err != nil {
				t.Fatalf("unmarshal dry-run %q: %v", dryOut, err)
			}
			if dry["status"] != "would_install" || dry["installed"] != false || dry["action"] != "dry-run" {
				t.Errorf("unexpected dry-run payload: %#v", dry)
			}

			// 4. ensure installs via CHROME_INSTALL_CMD and verifies detection afterward.
			target := filepath.ToSlash(filepath.Join(r.root, filepath.FromSlash(tc.rel)))
			installCmd := "mkdir -p '" + filepath.ToSlash(filepath.Dir(filepath.Join(r.root, filepath.FromSlash(tc.rel)))) + "' && printf '#!/bin/sh\\necho \"Google Chrome 136.0\"\\n' > '" + target + "' && chmod 755 '" + target + "'"
			insOut, insCode := r.run([]string{"CHROME_INSTALL_CMD=" + installCmd}, "ensure", "--json")
			if insCode != 0 {
				t.Fatalf("ensure install exit %d:\n%s", insCode, insOut)
			}
			var ins map[string]any
			if err := json.Unmarshal([]byte(insOut), &ins); err != nil {
				t.Fatalf("unmarshal install %q: %v", insOut, err)
			}
			if ins["status"] != "installed" || ins["installed"] != true || ins["action"] != "installed" || ins["path"] != target {
				t.Errorf("unexpected install payload: %#v", ins)
			}
		})
	}
}

func TestOpenCrossPlatform(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake chrome script")
	}
	for _, tc := range []struct {
		os  string
		rel string
	}{
		{"darwin", "Applications/Google Chrome.app/Contents/MacOS/Google Chrome"},
		{"linux", "usr/bin/google-chrome-stable"},
		{"windows", "c/Program Files/Google/Chrome/Application/chrome.exe"},
	} {
		t.Run(tc.os, func(t *testing.T) {
			r := newRunner(t, tc.os, "x86_64")
			wantPath := r.writeFakeChrome(tc.rel)
			openLog := filepath.Join(r.root, "open.log")
			out, code := r.run([]string{"CHROME_OPEN_LOG=" + filepath.ToSlash(openLog)}, "open", "https://example.com", "https://example.org")
			if code != 0 {
				t.Fatalf("open exit %d:\n%s", code, out)
			}
			b, err := os.ReadFile(openLog)
			if err != nil {
				t.Fatal(err)
			}
			got := strings.TrimSpace(string(b))
			want := tc.os + "|" + wantPath + "|https://example.com\n" + tc.os + "|" + wantPath + "|https://example.org"
			if got != want {
				t.Errorf("open log =\n%s\nwant:\n%s", got, want)
			}
		})
	}
}
