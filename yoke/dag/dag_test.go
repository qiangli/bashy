package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/dag"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := dag.NewDagCmd()
	dag.AddCapacityCommands(cmd, dag.CapacityServices{})
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy dag:", nil)
	if code != 0 {
		t.Fatalf("dag --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("dag --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "dag")
}

func TestBashySelfPropagatesToTargetEnvAndNoDoubleErrorPrefix(t *testing.T) {
	tmp := t.TempDir()
	fakeBashy := filepath.Join(tmp, "bashy")
	if err := os.WriteFile(fakeBashy, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dagFile := filepath.Join(tmp, "dag.md")
	md := "## Tasks\n\n### probe\n```bash\nprintf 'BASHY=%s\\nBASHY_EXE=%s\\n' \"$BASHY\" \"$BASHY_EXE\"\n```\n"
	if err := os.WriteFile(dagFile, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	origArg0 := os.Args[0]
	t.Cleanup(func() { os.Args[0] = origArg0 })
	os.Args[0] = filepath.Join(tmp, "adapters", "yoke", "dag")
	t.Setenv("BASHY_SELF", fakeBashy)

	applyBashySelfArgv0()
	cmd := dag.NewDagCmd()
	dag.AddCapacityCommands(cmd, dag.CapacityServices{})
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--no-journal", "--force", "--file", dagFile, "probe"}, "", func(err error) int {
		var dagErr *dag.Error
		if err != nil && !errors.As(err, &dagErr) {
			fmt.Fprintf(&errBuf, "bashy dag: %v\n", err)
		}
		return dag.ExitCodeOf(err)
	})
	if code != 0 {
		t.Fatalf("dag probe exited %d: stderr=%s stdout=%s", code, errBuf.String(), out.String())
	}
	combined := out.String() + errBuf.String()
	if !strings.Contains(combined, "BASHY="+fakeBashy) || !strings.Contains(combined, "BASHY_EXE="+fakeBashy) {
		t.Fatalf("expected BASHY and BASHY_EXE to resolve to %q, got:\n%s", fakeBashy, combined)
	}

	// Verify *dag.Error is not double-reported with "bashy dag:".
	cmd2 := dag.NewDagCmd()
	dag.AddCapacityCommands(cmd2, dag.CapacityServices{})
	var out2, errBuf2 bytes.Buffer
	cmd2.SetOut(&out2)
	cmd2.SetErr(&errBuf2)
	code2 := adapter.RunCobraArgs(cmd2, []string{"--no-journal", "--file", dagFile, "nosuchtarget"}, "", func(err error) int {
		var dagErr *dag.Error
		if err != nil && !errors.As(err, &dagErr) {
			fmt.Fprintf(&errBuf2, "bashy dag: %v\n", err)
		}
		return dag.ExitCodeOf(err)
	})
	if code2 == 0 {
		t.Fatal("expected unknown target to fail")
	}
	if strings.Contains(errBuf2.String(), "bashy dag:") {
		t.Fatalf("expected no duplicate 'bashy dag:' prefix on *dag.Error, got:\n%s", errBuf2.String())
	}
}

func TestStandaloneDagProvidesInProcessJQAndStripsAdapterEnv(t *testing.T) {
	tmp := t.TempDir()
	fakeBashy := filepath.Join(tmp, "bashy")
	if err := os.WriteFile(fakeBashy, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dagFile := filepath.Join(tmp, "dag.md")
	md := "## Tasks\n\n### profile-like\n```bash\nset -e\nquoted=$(M=$GENIE_MODEL_ID jq -n 'env.M')\ncobj=$(T_ID=x1 jq -cn '{id: env.T_ID, model: '\"$quoted\"'}')\nraw=$(printf '%s' \"$cobj\" | jq -r '.model')\nprintf 'QUOTED=%s\\nRAW=%s\\nBASHY=%s\\nBASHY_SELF=%s\\nBASHY_ADAPTER_SELF=%s\\nBASHY_INVOKED_AS=%s\\n' \\\n  \"$quoted\" \"$raw\" \"$BASHY\" \"${BASHY_SELF-<unset>}\" \"${BASHY_ADAPTER_SELF-<unset>}\" \"${BASHY_INVOKED_AS-<unset>}\"\n```\n"
	if err := os.WriteFile(dagFile, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	origArg0 := os.Args[0]
	t.Cleanup(func() { os.Args[0] = origArg0 })
	os.Args[0] = filepath.Join(tmp, "adapters", "yoke", "dag")
	t.Setenv("PATH", "")
	t.Setenv("GENIE_MODEL_ID", "s677-jq-profile")
	t.Setenv("BASHY_SELF", fakeBashy)
	t.Setenv("BASHY_ADAPTER_SELF", fakeBashy)
	t.Setenv("BASHY_INVOKED_AS", "dag")

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		_ = outR.Close()
		_ = outW.Close()
		t.Fatal(err)
	}
	var outBuf, errBuf bytes.Buffer
	doneOut := make(chan struct{})
	doneErr := make(chan struct{})
	go func() { _, _ = outBuf.ReadFrom(outR); close(doneOut) }()
	go func() { _, _ = errBuf.ReadFrom(errR); close(doneErr) }()

	code := runDagArgs([]string{"--no-journal", "--force", "--file", dagFile, "profile-like"}, func() (*os.File, *os.File) {
		return outW, errW
	})
	_ = outW.Close()
	_ = errW.Close()
	<-doneOut
	<-doneErr
	_ = outR.Close()
	_ = errR.Close()

	if code != 0 {
		t.Fatalf("runDagArgs exited %d: stderr=%s stdout=%s", code, errBuf.String(), outBuf.String())
	}
	combined := outBuf.String() + errBuf.String()
	for _, want := range []string{
		`QUOTED="s677-jq-profile"`,
		`RAW=s677-jq-profile`,
		"BASHY=" + fakeBashy,
		"BASHY_SELF=<unset>",
		"BASHY_ADAPTER_SELF=<unset>",
		"BASHY_INVOKED_AS=<unset>",
	} {
		if !strings.Contains(combined, want) {
			t.Errorf("missing %q in output:\n%s", want, combined)
		}
	}
}
