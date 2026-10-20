// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build !bashy_engines

package agentos

import (
	"io"
	"os"
	"strings"
	"testing"
)

// runLeanDksUserlandDispatch drives the lean build's REAL `bashy dks` early
// dispatch in-process — like runLeanDksDispatch in the federation test, but
// capturing BOTH streams: the userland verbs print their context to stdout
// and their failures to stderr, and both matter to the assertions. Returns
// the exit status, stdout and stderr.
func runLeanDksUserlandDispatch(t *testing.T, argv []string) (code int, stdout, stderr string) {
	t.Helper()
	oldArgs := os.Args
	os.Args = argv
	t.Cleanup(func() { os.Args = oldArgs })
	frontDoorObserving.Store(true)
	defer frontDoorObserving.Store(false)

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	outDone, errDone := make(chan string), make(chan string)
	go func() { b, _ := io.ReadAll(outR); outDone <- string(b) }()
	go func() { b, _ := io.ReadAll(errR); errDone <- string(b) }()

	func() {
		defer func() {
			os.Stdout, os.Stderr = oldOut, oldErr
			_ = outW.Close()
			_ = errW.Close()
			v := recover()
			if v == nil {
				t.Error("dispatchEngine returned without exiting")
				return
			}
			c, ok := v.(frontDoorExit)
			if !ok {
				panic(v)
			}
			code = int(c)
		}()
		dispatchEngine("dks")
	}()

	stdout, stderr = <-outDone, <-errDone
	_ = outR.Close()
	_ = errR.Close()
	return code, stdout, stderr
}

// TestLeanDksUserlandReachesHandler pins the lean routing of the
// cluster-userland client verbs: `bashy dks use` (no selection) must reach
// yoke's userland dispatcher and print the local-context message — NOT the
// lean build's VM refusal. State goes to a throwaway BASHY_HOME so the test
// never touches the real per-client context store.
func TestLeanDksUserlandReachesHandler(t *testing.T) {
	t.Setenv("BASHY_HOME", t.TempDir())

	code, stdout, stderr := runLeanDksUserlandDispatch(t, []string{"bashy", "dks", "use"})
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr:\n%s", code, stderr)
	}
	if !strings.Contains(stdout, "local") {
		t.Errorf("use did not print its local-context message; stdout:\n%s", stdout)
	}
	if strings.Contains(stderr, "not available in this build") {
		t.Errorf("userland hit the lean engine refusal; stderr:\n%s", stderr)
	}
}

// TestLeanDksUserlandStubFailsClosed pins the fail-closed half on the lean
// build. Every userland verb is real since yoke cc9b96e3 (S481.6 logs/attach),
// so the check is the verb's own refusal: `dks logs` without a selected
// workspace exits non-zero having run NOTHING locally, and it is not the
// engine-build refusal.
func TestLeanDksUserlandStubFailsClosed(t *testing.T) {
	t.Setenv("BASHY_HOME", t.TempDir())

	code, stdout, stderr := runLeanDksUserlandDispatch(t, []string{"bashy", "dks", "logs", "--", "echo", "hi"})
	if code == 0 {
		t.Fatalf("exit = 0, want non-zero without a selected workspace; stderr:\n%s", stderr)
	}
	if strings.Contains(stdout, "hi") {
		t.Errorf("logs ran the local command; stdout:\n%s", stdout)
	}
	if strings.Contains(stderr, "not available in this build") {
		t.Errorf("logs fell through to the engine refusal; stderr:\n%s", stderr)
	}
}

// TestLeanDksUserlandRunWithoutContextNeverRunsLocally pins the real `dks run`
// (S481.3) on the lean build: with no cluster workspace selected it exits
// non-zero having run NOTHING locally, and it is not the engine refusal.
func TestLeanDksUserlandRunWithoutContextNeverRunsLocally(t *testing.T) {
	t.Setenv("BASHY_HOME", t.TempDir())

	code, stdout, stderr := runLeanDksUserlandDispatch(t, []string{"bashy", "dks", "run", "--", "echo", "hi"})
	if code == 0 {
		t.Fatalf("exit = 0, want non-zero without a selected workspace; stderr:\n%s", stderr)
	}
	if strings.Contains(stdout, "hi") {
		t.Errorf("run executed the command locally; stdout:\n%s", stdout)
	}
	if strings.Contains(stderr, "not available in this build") {
		t.Errorf("run fell through to the engine refusal; stderr:\n%s", stderr)
	}
}
