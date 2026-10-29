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

// TestAppBoundaryInProcessTools: what bashy runs without a new process
// works under the boundary — awk included, which the guard alone would
// refuse as a spawner — while a genuinely external program is still
// refused, and awk's own file access and process spawning stay confined.
func TestAppBoundaryInProcessTools(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	own := filepath.Join(home, "shape", "notes", "alice", "data")
	otherApp := filepath.Join(home, "shape", "todo", "alice", "data")
	outside := filepath.Join(root, "outside")
	for _, d := range []string{own, otherApp, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(otherApp, "secret.txt"), []byte("todo-secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	policy, err := boundary.Env(boundary.Policy{App: "notes", Principal: "device:alice", Data: own, Home: home})
	if err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	q := func(p string) string { return "'" + filepath.ToSlash(p) + "'" }
	const seed = `printf 'b,2\na,1\n' > n.txt && printf '{"k":"v"}' > j.json && `

	tests := []struct {
		name   string
		body   string
		ok     bool
		stdout string // OK: exact stdout
		reason string // DENIED: substring of stderr
	}{
		{"awk notes list", `printf 'n1\n' >> "$BASHY_APP_DATA/notes.txt"; awk '{print}' "$BASHY_APP_DATA/notes.txt"`, true, "n1\n", ""},
		{"awk", seed + `awk -F, -v p=x '{print p $2}' n.txt`, true, "x2\nx1\n", ""},
		{"awk program file", seed + `printf '{print $1}\n' > p.awk && awk -F, -f p.awk n.txt`, true, "b\na\n", ""},
		{"awk getline and print redirect", seed + `awk 'BEGIN{getline l < "n.txt"; print l > "o.txt"; close("o.txt")}' && cat o.txt`, true, "b,2\n", ""},
		{"sed", seed + `sed s/b/B/ n.txt`, true, "B,2\na,1\n", ""},
		{"grep", seed + `grep a n.txt`, true, "a,1\n", ""},
		{"sort", seed + `sort n.txt`, true, "a,1\nb,2\n", ""},
		{"head", seed + `head -n 1 n.txt`, true, "b,2\n", ""},
		{"tail", seed + `tail -n 1 n.txt`, true, "a,1\n", ""},
		{"wc", seed + `wc -l < n.txt`, true, "2\n", ""},
		{"cut", seed + `cut -d, -f1 n.txt`, true, "b\na\n", ""},
		{"tr", seed + `tr ab AB < n.txt`, true, "B,2\nA,1\n", ""},
		{"jq", seed + `jq -r .k j.json`, true, "v\n", ""},
		{"date", `date -u -d @0 +%Y`, true, "1970\n", ""},
		{"external program", q(self) + " -test.run=^$", false, "", "undeclared external program"},
		{"awk other app read", "awk '{print}' " + q(filepath.Join(otherApp, "secret.txt")), false, "", "another app's data"},
		{"awk getline other app", `awk 'BEGIN{while ((getline l < "` + filepath.ToSlash(filepath.Join(otherApp, "secret.txt")) + `") > 0) print l}'`, false, "", "another app's data"},
		{"awk write outside grants", `awk 'BEGIN{print "x" > "` + filepath.ToSlash(filepath.Join(outside, "f.txt")) + `"}'`, false, "", "outside the app's own data"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := []string{boundary.EnvVar + "=" + policy, "BASHY_APP_DATA=" + own}
			stdout, stderr, code := runBashyEntry(t, own, env, "-c", tt.body, "api/"+strings.ReplaceAll(tt.name, " ", "-"))
			if tt.ok {
				if code != 0 || stdout != tt.stdout {
					t.Fatalf("want OK %q, got exit %d stdout %q stderr %q", tt.stdout, code, stdout, stderr)
				}
				return
			}
			if code == 0 || strings.Contains(stdout, "todo-secret") {
				t.Fatalf("want DENIED, got exit %d stdout %q stderr %q", code, stdout, stderr)
			}
			if !strings.Contains(stderr, "denied") || !strings.Contains(stderr, tt.reason) {
				t.Fatalf("want DENIED with reason %q, got exit %d stderr %q", tt.reason, code, stderr)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(outside, "f.txt")); err == nil {
		t.Error("denied awk write still created the file")
	}

	// awk never starts a process under the boundary: system() and pipes
	// fail instead of running the command.
	marker := filepath.Join(own, "spawned.txt")
	for _, body := range []string{
		`awk 'BEGIN{system("echo x > spawned.txt")}'`,
		`awk 'BEGIN{print "x" | "cat > spawned.txt"}'`,
		`awk 'BEGIN{"echo x > spawned.txt" | getline l}'`,
	} {
		_, stderr, code := runBashyEntry(t, own, []string{boundary.EnvVar + "=" + policy}, "-c", body, "api/awk-exec")
		if code == 0 || !strings.Contains(stderr, "NoExec") {
			t.Errorf("%s: want refusal, got exit %d stderr %q", body, code, stderr)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Fatalf("%s: awk spawned a process under the boundary", body)
		}
	}
}
