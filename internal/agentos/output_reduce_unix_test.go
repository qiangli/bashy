//go:build unix

// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty/v2"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

func TestShellOutputReductionReturnsThroughPTY(t *testing.T) {
	isolateOutputReduction(t)
	t.Setenv("BASHY_OUTPUT_REDUCE", "on")
	if !outputReductionEnabled(os.Environ()) {
		t.Fatalf("explicit output reduction is disabled: env=%q no-elide=%v reduce=%+v", os.Getenv("BASHY_OUTPUT_REDUCE"), noElideFlag, reduceFlag)
	}
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer ptmx.Close()
	defer tty.Close()
	captured := make(chan []byte, 1)
	go func() {
		var got bytes.Buffer
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				_, _ = got.Write(buf[:n])
				if bytes.Contains(got.Bytes(), []byte("bashy out ")) {
					captured <- got.Bytes()
					return
				}
			}
			if err != nil {
				captured <- got.Bytes() // PTYs commonly end with EIO after the slave closes.
				return
			}
		}
	}()

	var errOut bytes.Buffer
	opts := []interp.RunnerOption{
		interp.Lang(syntax.LangBash),
		interp.Env(expand.ListEnviron("PATH=" + os.Getenv("PATH"))),
		interp.Dir(t.TempDir()),
	}
	opts = WireExec(opts, false, os.Environ(), strings.NewReader(""), tty, &errOut)
	runner, err := interp.New(opts...)
	if err != nil {
		t.Fatal(err)
	}
	file, err := syntax.NewParser().Parse(strings.NewReader("seq 1 20000"), "output-reduce-pty-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Run(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	var got []byte
	select {
	case got = <-captured:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out reading reduced PTY output")
	}
	if !bytes.Contains(got, []byte("bashy out ")) {
		t.Fatalf("reduced output did not reach PTY: %q (stderr %q)", got, errOut.Bytes())
	}
}

// readPTYUntil drains ptmx until marker appears (or the slave closes).
func readPTYUntil(ptmx *os.File, marker string) <-chan []byte {
	captured := make(chan []byte, 1)
	go func() {
		var got bytes.Buffer
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				_, _ = got.Write(buf[:n])
				if bytes.Contains(got.Bytes(), []byte(marker)) {
					captured <- got.Bytes()
					return
				}
			}
			if err != nil {
				captured <- got.Bytes()
				return
			}
		}
	}()
	return captured
}

func runThroughPTY(t *testing.T, script string, marker string) []byte {
	t.Helper()
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer ptmx.Close()
	defer tty.Close()
	captured := readPTYUntil(ptmx, marker)
	opts := []interp.RunnerOption{
		interp.Lang(syntax.LangBash),
		interp.Env(expand.ListEnviron("PATH=" + os.Getenv("PATH"))),
		interp.Dir(t.TempDir()),
	}
	// Both sinks are the terminal, as in an interactive session or `bashy -c`
	// on a tty (cli/main.go hands WireExec os.Stdout and os.Stderr).
	opts = WireExec(opts, false, os.Environ(), strings.NewReader(""), tty, tty)
	runner, err := interp.New(opts...)
	if err != nil {
		t.Fatal(err)
	}
	file, err := syntax.NewParser().Parse(strings.NewReader(script), "child-tty-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Run(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-captured:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("timed out reading PTY output")
		return nil
	}
}

// Story 05c51640 (sprint 220): a foreground external command launched from a
// bashy session on a terminal must inherit that terminal on stdout/stderr.
// Stage 0 used to wrap the tty sink for every run, so os/exec piped the child
// and codex/claude refused to start ("stdout is not a terminal").
func TestTerminalSinkInheritedByForegroundChild(t *testing.T) {
	isolateOutputReduction(t)
	t.Setenv("BASHY_OUTPUT_REDUCE", "off")
	const script = `test -t 1 && test -t 2 && echo builtin=tty || echo builtin=notty
sh -c '[ -t 1 ] && [ -t 2 ] && echo child=tty || echo child=notty'
sh -c '[ -t 1 ] && echo piped=tty || echo piped=notty' | cat
echo END`
	got := string(runThroughPTY(t, script, "END"))
	for _, want := range []string{"builtin=tty", "child=tty", "piped=notty"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in PTY output:\n%s", want, got)
		}
	}
}

// The explicit opt-in is the one case where a terminal sink IS captured: the
// operator asked for reduction, and TestShellOutputReductionReturnsThroughPTY
// already proves the reduced form reaches the tty. Pin the child's view too.
func TestTerminalSinkCapturedOnlyWhenReductionRequested(t *testing.T) {
	isolateOutputReduction(t)
	t.Setenv("BASHY_OUTPUT_REDUCE", "on")
	got := string(runThroughPTY(t, `sh -c '[ -t 1 ] && echo child=tty || echo child=notty'; echo END`, "END"))
	if !strings.Contains(got, "child=notty") {
		t.Fatalf("explicit reduction must capture the child's stdout:\n%s", got)
	}
}
