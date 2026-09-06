// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/bashy/internal/cli"
)

func TestWarmSessionReturnsReducedOutputToRequestWriter(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "0")
	t.Setenv("BASHY_OUTPUT_REDUCE", "on")
	t.Setenv("VSC_PROFILE", "")
	home := t.TempDir()
	var stdout, stderr bytes.Buffer
	status := cli.RunSessionCommand(context.Background(), cli.SessionIO{
		Command: "seq 1 20000",
		Dir:     t.TempDir(),
		Env: []string{
			"BASHY_AGENTIC=1",
			"BASHY_OUTPUT_REDUCE=on",
			"BASHY_HOME=" + home,
			"PATH=" + os.Getenv("PATH"),
		},
		Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr,
	})
	if status != 0 || stderr.Len() != 0 {
		t.Fatalf("warm session status=%d stderr=%q", status, stderr.String())
	}
	if !strings.Contains(stdout.String(), "bashy out ") {
		t.Fatalf("warm request writer did not receive reduced output (%d bytes)", stdout.Len())
	}
	entries, err := os.ReadDir(filepath.Join(home, "exec", "output"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("warm request did not use its artifact root: entries=%d err=%v", len(entries), err)
	}
}
