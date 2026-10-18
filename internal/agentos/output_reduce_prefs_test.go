// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/interp"
)

// TestReaderPrefsBudget wires the launcher's BASHY_PREFS JSON budget_bytes
// (yoke/pkg/fleet.ReaderPrefs) into the Stage 1 output reduction sink.
func TestReaderPrefsBudget(t *testing.T) {
	isolateOutputReduction(t)

	const bigLines = 200
	bigBody := strings.Repeat("0123456789abcdef\n", bigLines) // 17*200 = 3400 B

	t.Run("custom budget bounds stage1 view and spills before emit", func(t *testing.T) {
		isolateOutputReduction(t)
		t.Setenv("BASHY_OUTPUT_REDUCE", "on")
		t.Setenv("BASHY_PREFS", `{"budget_bytes":2048}`)
		out, errOut, runErr := runReducedSource(t, "seq 1 20000", false, nil)
		if runErr != nil || errOut != "" {
			t.Fatalf("seq failed: run=%v stderr=%d bytes", runErr, len(errOut))
		}
		if len(out) > 2048 {
			t.Fatalf("custom budget not applied: view is %d bytes, want <= 2048", len(out))
		}
		if !strings.Contains(out, "bashy out ") {
			t.Fatalf("budgeted view lacks recovery command: %q", out)
		}
		if recovered := recoverMarker(t, out); !bytes.Equal(recovered, sequenceBytes(20000)) {
			t.Fatalf("recovered %d bytes, want complete %d", len(recovered), len(sequenceBytes(20000)))
		}
	})

	t.Run("budget below default proves wiring", func(t *testing.T) {
		isolateOutputReduction(t)
		t.Setenv("BASHY_OUTPUT_REDUCE", "on")
		t.Setenv("BASHY_PREFS", `{"budget_bytes":1024}`)
		fx := fixtureExec(map[string][2]string{"dump big": {bigBody, ""}}, nil)
		out, errOut, runErr := runReducedSource(t, "dump big", false, fx)
		if runErr != nil || errOut != "" {
			t.Fatalf("fixture failed: run=%v stderr=%d bytes", runErr, len(errOut))
		}
		// 3400 B fits the 40 KiB default ceiling, so a complete view here
		// would mean the prefs budget never reached the sink.
		if !strings.Contains(out, "bashy out ") || len(out) > 1024 {
			t.Fatalf("prefs budget not applied: %d bytes, marker=%v", len(out), strings.Contains(out, "bashy out "))
		}
		if recovered := recoverMarker(t, out); string(recovered) != bigBody {
			t.Fatalf("recovered %d bytes, want complete %d", len(recovered), len(bigBody))
		}
	})

	t.Run("zero budget selects default ceiling", func(t *testing.T) {
		isolateOutputReduction(t)
		t.Setenv("BASHY_OUTPUT_REDUCE", "on")
		t.Setenv("BASHY_PREFS", `{"budget_bytes":0}`)
		fx := fixtureExec(map[string][2]string{"dump big": {bigBody, ""}}, nil)
		out, errOut, runErr := runReducedSource(t, "dump big", false, fx)
		if runErr != nil || errOut != "" {
			t.Fatalf("fixture failed: run=%v stderr=%d bytes", runErr, len(errOut))
		}
		if out != bigBody {
			t.Fatalf("zero budget changed output: %d bytes, want complete %d", len(out), len(bigBody))
		}
	})

	t.Run("invalid prefs fail closed with clear construction error", func(t *testing.T) {
		isolateOutputReduction(t)
		t.Setenv("BASHY_OUTPUT_REDUCE", "on")
		t.Setenv("BASHY_PREFS", `{"budget_bytes":`)
		_, err := newShellOutputReducer(io.Discard, io.Discard, os.Environ())
		if err == nil || !strings.Contains(err.Error(), "BASHY_PREFS") {
			t.Fatalf("construction error = %v, want a clear BASHY_PREFS error", err)
		}
		// End to end the bad projection must not silently reshape a view:
		// output passes through complete, with no recovery marker.
		out, _, runErr := runReducedSource(t, "seq 1 20000", false, nil)
		if runErr != nil {
			t.Fatal(runErr)
		}
		if want := sequenceBytes(20000); out != string(want) {
			t.Fatalf("invalid prefs reshaped output: got %d bytes, want complete %d", len(out), len(want))
		}
		if strings.Contains(out, "bashy out ") {
			t.Fatal("invalid prefs produced a reduced view")
		}
	})

	t.Run("budget ignored without stage1", func(t *testing.T) {
		isolateOutputReduction(t)
		t.Setenv("BASHY_PREFS", `{"budget_bytes":1024}`)
		fx := fixtureExec(map[string][2]string{"dump big": {bigBody, ""}}, nil)
		out, errOut, runErr := runReducedSource(t, "dump big", false, fx)
		if runErr != nil || errOut != "" {
			t.Fatalf("fixture failed: run=%v stderr=%d bytes", runErr, len(errOut))
		}
		if out != bigBody {
			t.Fatalf("budget applied without Stage 1: %d bytes, want complete %d", len(out), len(bigBody))
		}
	})

	t.Run("budget ignored under full escape", func(t *testing.T) {
		isolateOutputReduction(t)
		t.Setenv("BASHY_OUTPUT_REDUCE", "on")
		t.Setenv("BASHY_PREFS", `{"budget_bytes":1024}`)
		noElideFlag = true
		fx := fixtureExec(map[string][2]string{"dump big": {bigBody, ""}}, nil)
		out, _, runErr := runReducedSource(t, "dump big", false, fx)
		if runErr != nil {
			t.Fatal(runErr)
		}
		if out != bigBody {
			t.Fatalf("--full did not escape the budget: %d bytes, want complete %d", len(out), len(bigBody))
		}
	})

	t.Run("budget ignored under cert profile", func(t *testing.T) {
		isolateOutputReduction(t)
		t.Setenv("BASHY_OUTPUT_REDUCE", "on")
		t.Setenv("BASHY_PREFS", `{"budget_bytes":1024}`)
		t.Setenv("VSC_PROFILE", "cert")
		fx := fixtureExec(map[string][2]string{"dump big": {bigBody, ""}}, nil)
		out, _, runErr := runReducedSource(t, "dump big", false, fx)
		if runErr != nil {
			t.Fatal(runErr)
		}
		if out != bigBody {
			t.Fatalf("cert profile did not escape the budget: %d bytes, want complete %d", len(out), len(bigBody))
		}
	})

	t.Run("failure evidence preserved under budget", func(t *testing.T) {
		isolateOutputReduction(t)
		t.Setenv("BASHY_OUTPUT_REDUCE", "on")
		t.Setenv("BASHY_PREFS", `{"budget_bytes":1024}`)
		stdout := strings.Repeat("context\n", 7000)
		stderr := strings.Repeat("FAIL file.go:42\n", 4000)
		fx := fixtureExec(map[string][2]string{"go test": {stdout, stderr}}, map[string]int{"go test": 2})
		out, errOut, runErr := runReducedSource(t, "go test", false, fx)
		var status interp.ExitStatus
		if !errors.As(runErr, &status) || int(status) != 2 {
			t.Fatalf("exit = %v, want 2", runErr)
		}
		if out != stdout || errOut != stderr {
			t.Fatalf("budget compressed failure evidence: stdout %d/%d stderr %d/%d", len(out), len(stdout), len(errOut), len(stderr))
		}
	})

	t.Run("redaction and canonicalization hold under budget", func(t *testing.T) {
		isolateOutputReduction(t)
		t.Setenv("BASHY_OUTPUT_REDUCE", "on")
		t.Setenv("BASHY_PREFS", `{"budget_bytes":2048}`)
		home := os.Getenv("HOME")
		// Synthetic github-token-shaped value: shape masking replaces it
		// with [redacted:github-token] without needing vault env.
		credential := "ghp_" + strings.Repeat("x", 36)
		var full strings.Builder
		for i := 0; i < 150; i++ {
			full.WriteString(home + "/data/file\n")
			full.WriteString("token " + credential + "\n")
		}
		fx := fixtureExec(map[string][2]string{"dump mixed": {full.String(), ""}}, nil)
		out, errOut, runErr := runReducedSource(t, "dump mixed", false, fx)
		if runErr != nil || errOut != "" {
			t.Fatalf("fixture failed: run=%v stderr=%d bytes", runErr, len(errOut))
		}
		if len(out) > 2048 {
			t.Fatalf("budgeted view is %d bytes, want <= 2048", len(out))
		}
		for name, got := range map[string]string{"view": out, "artifact": string(recoverMarker(t, out))} {
			if strings.Contains(got, home) || strings.Contains(got, credential) {
				t.Fatalf("%s leaked raw home or credential", name)
			}
			if !strings.Contains(got, "$HOME") || !strings.Contains(got, "[redacted:github-token]") {
				t.Fatalf("%s lacks canonical/redacted form", name)
			}
		}
	})
}
