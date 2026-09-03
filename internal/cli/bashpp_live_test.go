package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

func TestBashPPLiveStatementReselection(t *testing.T) {
	for _, src := range []string{
		"set -o bashpp; type T int; echo enabled",
		"set -o bashpp\ntype T int\necho enabled\n",
	} {
		t.Run(src, func(t *testing.T) {
			var out bytes.Buffer
			r, err := interp.New(interp.Lang(syntax.LangBash), interp.StdIO(nil, &out, io.Discard), interp.Env(expand.ListEnviron()))
			if err != nil {
				t.Fatal(err)
			}
			if err := runStatementStream(context.Background(), r, []byte(src), syntax.LangBash, "bash"); err != nil {
				t.Fatal(err)
			}
			if got := out.String(); got != "enabled\n" {
				t.Fatalf("output = %q", got)
			}
		})
	}
}

func TestBashPPLiveDisableFallsBackToClassic(t *testing.T) {
	var out, stderr bytes.Buffer
	r, err := interp.New(interp.Lang(syntax.LangBashPP), interp.StdIO(nil, &out, &stderr), interp.Env(expand.ListEnviron()))
	if err != nil {
		t.Fatal(err)
	}
	if err := runStatementStream(context.Background(), r, []byte("set +o bashpp; type T int; echo done"), syntax.LangBashPP, "bash"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "type: T: not found") || out.String() != "done\n" {
		t.Fatalf("stdout=%q stderr=%q", out.String(), stderr.String())
	}
}

func TestRunSessionCommandBashPPLive(t *testing.T) {
	var out, stderr bytes.Buffer
	status := RunSessionCommand(context.Background(), SessionIO{
		Command: "set +o bashpp; set -o bashpp; type T int; echo session",
		Env:     []string{"PATH="}, Stdout: &out, Stderr: &stderr,
	})
	if status != 0 || out.String() != "session\n" {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, out.String(), stderr.String())
	}
}
