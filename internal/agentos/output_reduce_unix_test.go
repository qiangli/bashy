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
