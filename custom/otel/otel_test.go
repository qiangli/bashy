package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/bashy/custom/internal/adapter"
)

func TestOtelHelpServeAndDeps(t *testing.T) {
	cmd := newOtelCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("otel --help: %v (stderr=%q)", err, errBuf.String())
	}
	if !strings.Contains(out.String(), "serve") {
		t.Fatalf("otel --help missing serve subcommand:\n%s", out.String())
	}
	if runtime.GOOS != "windows" {
		fake := filepath.Join(t.TempDir(), "fake-otel")
		if err := os.WriteFile(fake, []byte("#!/bin/sh\n[ \"$1\" = serve ] && [ \"$2\" = --check ] && exit 0 || exit 9\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("BASHY_OTEL_BIN", fake)
		if code := run([]string{"serve", "--check"}); code != 0 {
			t.Fatalf("otel serve --check with BASHY_OTEL_BIN exited %d, want 0", code)
		}
	}
	adapter.AssertStandaloneDeps(t, "otel")
}
