//go:build unix

// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"io"
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
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer ptmx.Close()
	captured := make(chan []byte, 1)
	go func() {
		got, _ := io.ReadAll(ptmx) // PTYs commonly end with EIO after the slave closes.
		captured <- got
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
	if err := tty.Close(); err != nil {
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
