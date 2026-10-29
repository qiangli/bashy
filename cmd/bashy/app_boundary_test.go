//go:build !bashy_core && !bashy_cert_base

// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/shape/boundary"
)

// asBashyMainEnv set turns the test binary into bashy: TestMain hands the
// remaining arguments to the real main(), so these tests drive the same
// `bashy -c BODY NAME` entry point the console execs, on every OS.
const asBashyMainEnv = "BASHY_TEST_AS_MAIN"

func TestMain(m *testing.M) {
	if os.Getenv(asBashyMainEnv) == "1" {
		os.Args = append([]string{"bashy"}, os.Args[1:]...)
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runBashyEntry runs `bashy ARGS...` through main() in dir with env added
// to the test's environment (minus any inherited boundary).
func runBashyEntry(t *testing.T, dir string, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(self, args...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, boundary.EnvVar+"=") && !strings.HasPrefix(kv, asBashyMainEnv+"=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(append(cmd.Env, asBashyMainEnv+"=1"), env...)
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	err = cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		return out.String(), errb.String(), ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run bashy %v: %v", args, err)
	}
	return out.String(), errb.String(), 0
}

func TestAppBoundaryEntryPoint(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	shapeDir := filepath.Join(home, "shape")
	own := filepath.Join(shapeDir, "notes", "alice", "data")
	otherPrincipal := filepath.Join(shapeDir, "notes", "bob", "data")
	otherApp := filepath.Join(shapeDir, "todo", "alice", "data")
	outside := filepath.Join(root, "outside")
	for _, d := range []string{own, otherPrincipal, otherApp, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(otherApp, "secret.txt"), []byte("todo-secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The owning app's interface, reached as this run's principal through
	// the console's call broker (token-bound).
	const token = "tok-alice-notes"
	var calls []string
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(boundary.CallTokenHeader) != token {
			http.Error(w, "no access", http.StatusForbidden)
			return
		}
		calls = append(calls, r.URL.Path)
		_, _ = w.Write([]byte("todo-items\n"))
	}))
	defer broker.Close()

	policy, err := boundary.Env(boundary.Policy{
		App: "notes", Principal: "alice", Data: own, Home: home,
		Call: &boundary.Endpoint{URL: broker.URL, Token: token},
	})
	if err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	q := func(p string) string { return "'" + filepath.ToSlash(p) + "'" }

	tests := []struct {
		name   string
		body   string
		ok     bool
		stdout string // OK: exact stdout
		reason string // DENIED: substring of stderr
	}{
		{"other principal write", "echo x > " + q(filepath.Join(otherPrincipal, "f.txt")), false, "", "another principal's data"},
		{"other app read", "cat " + q(filepath.Join(otherApp, "secret.txt")), false, "", "another app's data"},
		{"other app read redirect", "read -r l < " + q(filepath.Join(otherApp, "secret.txt")), false, "", "another app's data"},
		{"other app write", "echo x > " + q(filepath.Join(otherApp, "f.txt")), false, "", "another app's data"},
		{"write outside grants", "echo x > " + q(filepath.Join(outside, "f.txt")), false, "", "outside the app's own data"},
		{"undeclared external program", q(self) + " -test.run=^$", false, "", "undeclared external program"},
		{"dev tcp", "echo x > /dev/tcp/127.0.0.1/9", false, "", "network redirection"},
		{"own data read write", "echo hi > own.txt && cat own.txt && cat " + q(filepath.Join(own, "own.txt")), true, "hi\nhi\n", ""},
		{"app call as same principal", "app call todo list", true, "todo-items\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, code := runBashyEntry(t, own, []string{boundary.EnvVar + "=" + policy}, "-c", tt.body, "api/"+strings.ReplaceAll(tt.name, " ", "-"))
			if tt.ok {
				if code != 0 || stdout != tt.stdout {
					t.Fatalf("want OK %q, got exit %d stdout %q stderr %q", tt.stdout, code, stdout, stderr)
				}
				return
			}
			if code == 0 {
				t.Fatalf("want DENIED, got exit 0 stdout %q stderr %q", stdout, stderr)
			}
			if !strings.Contains(stderr, "denied") || !strings.Contains(stderr, tt.reason) {
				t.Fatalf("want DENIED with reason %q, got exit %d stderr %q", tt.reason, code, stderr)
			}
		})
	}
	for _, f := range []string{filepath.Join(otherPrincipal, "f.txt"), filepath.Join(otherApp, "f.txt"), filepath.Join(outside, "f.txt")} {
		if _, err := os.Stat(f); err == nil {
			t.Errorf("denied write still created %s", f)
		}
	}
	if len(calls) != 1 || calls[0] != "/todo/list" {
		t.Errorf("app call reached %v, want [/todo/list]", calls)
	}
}

func TestAppBoundaryFailsClosed(t *testing.T) {
	dir := t.TempDir()
	valid, err := boundary.Env(boundary.Policy{App: "notes", Data: dir})
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "ran.txt")
	body := "echo ran > " + "'" + filepath.ToSlash(marker) + "'"
	tests := []struct {
		name  string
		value string
		args  []string
	}{
		{"malformed json", "{not json", []string{"-c", body}},
		{"empty value", "", []string{"-c", body}},
		{"relative data dir", `{"app":"notes","data":"rel/data"}`, []string{"-c", body}},
		{"option before -c", valid, []string{"-e", "-c", body}},
		{"bundled -ec", valid, []string{"-ec", body}},
		{"dropped positional args", valid, []string{"-c", body, "name", "arg1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, stderr, code := runBashyEntry(t, dir, []string{boundary.EnvVar + "=" + tt.value}, tt.args...)
			if code != boundary.DeniedStatus || !strings.Contains(stderr, "refusing to run") {
				t.Fatalf("want refusal (exit %d), got exit %d stderr %q", boundary.DeniedStatus, code, stderr)
			}
			if _, err := os.Stat(marker); err == nil {
				t.Fatal("script ran despite the refusal")
			}
		})
	}
}

func TestAppBoundaryUnsetIsUnchanged(t *testing.T) {
	t.Setenv(boundary.EnvVar, "")
	os.Unsetenv(boundary.EnvVar)
	for _, args := range [][]string{{"bashy", "-c", "echo hi"}, {"bashy", "-c", "echo hi", "name"}, {"bashy"}} {
		if _, handled := appBoundaryMain(args, nil, nil, nil); handled {
			t.Errorf("%v handled without %s", args, boundary.EnvVar)
		}
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "outside.txt")
	stdout, stderr, code := runBashyEntry(t, dir, nil, "-c", "echo unbounded > '"+filepath.ToSlash(marker)+"' && echo ok")
	if code != 0 || strings.TrimSpace(stdout) != "ok" {
		t.Fatalf("unset boundary: exit %d stdout %q stderr %q", code, stdout, stderr)
	}
}

func TestAppBoundaryCommand(t *testing.T) {
	tests := []struct {
		args       []string
		body, name string
		isC, err   bool
	}{
		{[]string{"bashy", "-c", "echo"}, "echo", "bashy", true, false},
		{[]string{"bashy", "-c", "echo", "api/x"}, "echo", "api/x", true, false},
		{[]string{"bashy", "-c"}, "", "", true, true},
		{[]string{"bashy", "-o", "pipefail", "-c", "echo"}, "", "", true, true},
		{[]string{"bashy", "script.sh"}, "", "", false, false},
		{[]string{"bashy", "kb", "search", "x"}, "", "", false, false},
		{[]string{"bashy", "--norc"}, "", "", false, false},
		{[]string{"bashy"}, "", "", false, false},
	}
	for _, tt := range tests {
		body, name, isC, err := appBoundaryCommand(tt.args)
		if body != tt.body || name != tt.name || isC != tt.isC || (err != nil) != tt.err {
			t.Errorf("appBoundaryCommand(%q) = %q, %q, %v, %v", tt.args, body, name, isC, err)
		}
	}
}
