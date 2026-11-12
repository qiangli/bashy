// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// The serve adoption: vlog.sh registers the built site with the bashy
// console (`app add NAME --type site --source DIR`) instead of running a
// server of its own. The test builds the sample site, registers it through
// the real CLI in a sandboxed home (isolated command and app rings), and
// reads the record back: type site, source = the site's public/ directory.
// No server is started anywhere: serve writes a record and prints the
// console URL, pointing CONSOLE_PORT at a port nothing listens on so the
// "start the console" hint is exercised deterministically. Live serving is
// the conductor's S499.4 re-run.
package vlog

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
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

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }

func TestMain(m *testing.M) {
	var err error
	if buildDir, err = os.MkdirTemp("", "vlog-*"); err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(buildDir)
	os.Exit(code)
}

// moduleRoot is the bashy repository root (the directory holding cmd/bashy).
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

// bashyBin builds the CLI once. The binary must be named bashy: vlog.sh
// calls `bashy app ...`, resolved through PATH below.
func bashyBin(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		bin := filepath.Join(buildDir, "bashy")
		cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/bashy")
		cmd.Dir = moduleRoot(t)
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

// freePort is a port nothing listens on: the console is down, so serve
// must print how to start it instead of finding one running.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestServeRegistersSiteApp(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the bashy binary")
	}
	bin := bashyBin(t)
	root := moduleRoot(t)
	example := filepath.Join(root, "examples", "vlog")

	home := t.TempDir()
	port := freePort(t)
	env := append(os.Environ(),
		"HOME="+home,
		"BASHY_HOME="+filepath.Join(home, ".bashy"),
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"BASHY_COMMANDS_DIR="+filepath.Join(home, "commands"),
		"BASHY_APPS_DIR="+filepath.Join(home, "apps"),
		"BASHY_APPS_PATH=",
		"CONSOLE_PORT="+strconv.Itoa(port),
		"PATH="+filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Dir = example
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bashy %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}

	// Build the bundled sample site, then serve it through vlog.sh.
	run("vlog.sh", "build", "sample")
	out := run("vlog.sh", "serve", "sample")
	if want := "http://127.0.0.1:" + strconv.Itoa(port) + "/vlog/"; !strings.Contains(out, want) {
		t.Fatalf("serve output lacks the console URL %s:\n%s", want, out)
	}
	if !strings.Contains(out, "bashy app serve") {
		t.Fatalf("serve output lacks the console start hint:\n%s", out)
	}

	// The record: type site, source = the built public/ directory.
	var app struct{ Name, Type, Source string }
	if err := json.Unmarshal([]byte(run("app", "show", "vlog", "--json")), &app); err != nil {
		t.Fatalf("app show --json: %v", err)
	}
	want := filepath.ToSlash(filepath.Join(example, "sample", "public"))
	if app.Name != "vlog" || app.Type != "site" || filepath.ToSlash(app.Source) != want {
		t.Fatalf("app show = %+v, want vlog site %s", app, want)
	}

	// Re-running serve is idempotent: still one record, same source.
	run("vlog.sh", "serve", "sample")
	entries, err := os.ReadDir(filepath.Join(home, "apps"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("app ring after re-serve = %v (err %v), want exactly one record", entries, err)
	}
	var again struct{ Type, Source string }
	if err := json.Unmarshal([]byte(run("app", "show", "vlog", "--json")), &again); err != nil {
		t.Fatalf("app show --json after re-serve: %v", err)
	}
	if again.Type != "site" || filepath.ToSlash(again.Source) != want {
		t.Fatalf("app show after re-serve = %+v, want site %s", again, want)
	}
}
