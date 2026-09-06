// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build e2e

package agentos

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/qiangli/coreutils/pkg/reduce"
)

var (
	standaloneBashOnce sync.Once
	standaloneBashBin  string
	standaloneBashErr  error
)

func standaloneBashBinary(t *testing.T) string {
	t.Helper()
	standaloneBashOnce.Do(func() {
		root, ok := findBashySourceRoot(mustGetwd())
		if !ok {
			standaloneBashErr = os.ErrNotExist
			return
		}
		standaloneBashBin = filepath.Join(os.TempDir(), "bashy-e2e-standalone-bash")
		if runtime.GOOS == "windows" {
			standaloneBashBin += ".exe"
		}
		cmd := exec.Command("go", "build", "-o", standaloneBashBin, "./cmd/bash")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			standaloneBashErr = fmt.Errorf("build cmd/bash: %w (%d diagnostic bytes)", err, len(out))
		}
	})
	if standaloneBashErr != nil {
		t.Fatal(standaloneBashErr)
	}
	return standaloneBashBin
}

func isolatedOutputEnv(home string, entries ...string) []string {
	drop := map[string]bool{
		"BASHY_AGENTIC": true, "BASHY_HOME": true,
		"BASHY_OUTPUT_REDUCE": true, "VSC_PROFILE": true,
		"HOME": true,
	}
	env := make([]string, 0, len(os.Environ())+len(entries)+1)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !drop[name] {
			env = append(env, entry)
		}
	}
	env = append(env, "BASHY_HOME="+home)
	return append(env, entries...)
}

func TestE2EOutputStage0CanonicalizesHomeInEveryActivationMode(t *testing.T) {
	bin := bashyBinary(t)
	fixtureHome := filepath.Join(t.TempDir(), "alice")
	unsafeHome := string(filepath.Separator)
	source := `printf 'out=%s\n' "$HOME"; printf 'err=%s\n' "$HOME" >&2`
	wantOut, wantErr := []byte("out=$HOME\n"), []byte("err=$HOME\n")
	for _, tc := range []struct {
		name string
		env  []string
		args []string
	}{
		{"ordinary", nil, []string{"-c", source}},
		{"under-budget", []string{"BASHY_OUTPUT_REDUCE=on"}, []string{"-c", source}},
		{"agentic-alone", []string{"BASHY_AGENTIC=1"}, []string{"-c", source}},
		{"explicit-opt-in", []string{"BASHY_OUTPUT_REDUCE=on"}, []string{"-c", source}},
		{"rollback-off", []string{"BASHY_OUTPUT_REDUCE=off"}, []string{"-c", source}},
		{"rollback-full", []string{"BASHY_OUTPUT_REDUCE=on"}, []string{"--full", "-c", source}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := isolatedOutputEnv(t.TempDir(), append(tc.env, "HOME="+fixtureHome)...)
			out, errOut, code := runOutputBinary(t, bin, env, tc.args...)
			if code != 0 {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, out, errOut)
			}
			requireExactBytes(t, "stdout", out, wantOut)
			requireExactBytes(t, "stderr", errOut, wantErr)
		})
	}

	// The core algorithm rejects unsafe homes and leaves ordinary bytes alone.
	out, errOut, code := runOutputBinary(t, bin,
		isolatedOutputEnv(t.TempDir(), "HOME="+unsafeHome), "-c", source)
	if code != 0 {
		t.Fatalf("unsafe-home exit=%d", code)
	}
	requireExactBytes(t, "unsafe stdout", out, []byte("out=/\n"))
	if !bytes.HasSuffix(errOut, []byte("err=/\n")) || bytes.Contains(errOut, []byte("err=$HOME")) {
		t.Fatalf("unsafe HOME was unexpectedly canonicalized: %q", errOut)
	}

	out, errOut, code = runOutputBinary(t, bin,
		isolatedOutputEnv(t.TempDir(), "HOME="+fixtureHome), "-c", `printf '/Users/alice2\n'; printf 'plain\n' >&2`)
	if code != 0 {
		t.Fatalf("boundary-negative exit=%d", code)
	}
	requireExactBytes(t, "boundary-negative stdout", out, []byte("/Users/alice2\n"))
	requireExactBytes(t, "ordinary stderr", errOut, []byte("plain\n"))
}

func TestE2EOutputStage0CanonicalizesBeforeStage1SpillAndDiagnostics(t *testing.T) {
	bin := bashyBinary(t)
	fixtureHome := filepath.Join(t.TempDir(), "alice")
	home := t.TempDir()
	// The recursive bashy is external; cat is Bashy's in-process coreutils.
	// Its missing-file diagnostic exercises stderr hygiene on the failure path.
	source := `BASHY_OUTPUT_REDUCE=off "$BASHY_E2E_SELF" -c 'i=0; while ((i < 6000)); do printf "%s/out-%04d\n" "$HOME" "$i"; printf "%s/err-%04d\n" "$HOME" "$i" >&2; ((i++)); done'`
	env := isolatedOutputEnv(home, "HOME="+fixtureHome, "BASHY_OUTPUT_REDUCE=on", "BASHY_E2E_SELF="+bin)
	out, errOut, code := runOutputBinary(t, bin, env, "-c", source)
	if code != 0 {
		t.Fatalf("external spill exit=%d", code)
	}
	for stream, marker := range map[string][]byte{"stdout": out, "stderr": errOut} {
		_ = markerHandle(t, marker)
		if bytes.Contains(marker, []byte(fixtureHome)) {
			t.Fatalf("raw HOME escaped %s marker", stream)
		}
		recovered := recoverE2E(t, bin, home, marker)
		if bytes.Contains(recovered, []byte(fixtureHome)) || !bytes.Contains(recovered, []byte("$HOME/")) {
			t.Fatalf("%s recovery was not canonicalized", stream)
		}
	}

	file := filepath.Join(t.TempDir(), "fixture")
	if err := os.WriteFile(file, []byte(fixtureHome+"/from-coreutils\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code = runOutputBinary(t, bin, append(isolatedOutputEnv(t.TempDir(), "HOME="+fixtureHome), "FIXTURE="+file), "-c", `cat "$FIXTURE"; cat "$HOME/missing"`)
	if code == 0 {
		t.Fatal("expected in-process cat diagnostic failure")
	}
	if bytes.Contains(out, []byte(fixtureHome)) || bytes.Contains(errOut, []byte(fixtureHome)) {
		t.Fatal("raw HOME escaped in-process output or diagnostic")
	}
	if !bytes.Contains(out, []byte("$HOME/from-coreutils")) || !bytes.Contains(errOut, []byte("$HOME/missing")) {
		t.Fatalf("in-process outputs were not canonicalized: stdout=%q stderr=%q", out, errOut)
	}
}

func TestE2EOutputStage01SuppressesOnlyExactTelemetryHintsByDefault(t *testing.T) {
	bin := bashyBinary(t)
	fixtureHome := filepath.Join(t.TempDir(), "alice")
	home := t.TempDir()
	const secret = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	script := `hint="bashy: telemetry on → $HOME/.agents/otel/spool/spans.jsonl?token=$GITHUB_TOKEN (service=bashy)"; similar="bashy: telemetry on → $HOME/.agents/otel/spool/other.jsonl?token=$GITHUB_TOKEN (service=bashy)"; printf '%s\n%s\nordinary duplicate\n%s\nordinary duplicate\n%s\n' "$hint" "$hint" "$hint" "$similar" >&2`
	env := isolatedOutputEnv(home,
		"HOME="+fixtureHome,
		"GITHUB_TOKEN="+secret,
		"BASHY_E2E_SELF="+bin,
		"BASHY_E2E_DUPLICATE_SCRIPT="+script)
	out, errOut, code := runOutputBinary(t, bin, env, "-c", `BASHY_OUTPUT_REDUCE=off "$BASHY_E2E_SELF" -c "$BASHY_E2E_DUPLICATE_SCRIPT"`)
	if code != 0 || len(out) != 0 {
		t.Fatalf("duplicate fixture failed: exit=%d stdout=%d stderr=%d", code, len(out), len(errOut))
	}
	if bytes.Contains(errOut, []byte(fixtureHome)) || bytes.Contains(errOut, []byte(secret)) {
		t.Fatal("private HOME or synthetic secret escaped inline")
	}
	if got := bytes.Count(errOut, []byte("bashy: telemetry on → ")); got != 2 {
		t.Fatalf("inline telemetry hints=%d, want first exact hint plus distinct hint: %q", got, errOut)
	}
	if bytes.Count(errOut, []byte("ordinary duplicate")) != 2 ||
		!bytes.Contains(errOut, []byte("2 duplicate telemetry hints suppressed")) {
		t.Fatalf("nontelemetry or annotation mismatch: %q", errOut)
	}

	recovered := recoverE2E(t, bin, home, errOut)
	if bytes.Contains(recovered, []byte(fixtureHome)) || bytes.Contains(recovered, []byte(secret)) {
		t.Fatal("private HOME or synthetic secret escaped recovery")
	}
	if bytes.Count(recovered, []byte("bashy: telemetry on → ")) != 4 ||
		bytes.Count(recovered, []byte("ordinary duplicate")) != 2 ||
		!bytes.Contains(recovered, []byte("$HOME/.agents/otel/spool/")) ||
		!bytes.Contains(recovered, []byte("[redacted:github-token]")) {
		t.Fatalf("recovery did not preserve canonicalized/redacted source: %q", recovered)
	}
	inlineTokens := e2eTokenCount(t, bin, home, errOut)
	fullTokens := e2eTokenCount(t, bin, home, recovered)
	if inlineTokens >= fullTokens {
		t.Fatalf("real-BPE token reduction absent: inline=%d full=%d", inlineTokens, fullTokens)
	}
	t.Logf("Stage 0.1 cl100k_base tokens: inline=%d full=%d", inlineTokens, fullTokens)
}

func e2eTokenCount(t *testing.T, bin, home string, input []byte) int {
	t.Helper()
	cmd := exec.Command(bin, "tokens", "--encoding", "cl100k_base")
	cmd.Env = isolatedOutputEnv(home)
	cmd.Stdin = bytes.NewReader(input)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		t.Fatalf("exact token counter failed: %v (%d diagnostic bytes)", err, errOut.Len())
	}
	fields := strings.Fields(out.String())
	if len(fields) == 0 {
		t.Fatalf("exact token counter returned no count: %q", out.String())
	}
	n, err := strconv.Atoi(fields[0])
	if err != nil {
		t.Fatalf("parse exact token count %q: %v", fields[0], err)
	}
	return n
}

func runOutputBinary(t *testing.T, bin string, env []string, args ...string) (stdout, stderr []byte, code int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader("")
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("run actual entrypoint: %v", err)
	}
	return out.Bytes(), errOut.Bytes(), code
}

func requireExactBytes(t *testing.T, stream string, got, want []byte) {
	t.Helper()
	if !bytes.Equal(got, want) {
		t.Fatalf("%s bytes differ: got=%d want=%d", stream, len(got), len(want))
	}
}

func markerHandle(t *testing.T, marker []byte) string {
	t.Helper()
	match := recoveryHandleRE.FindSubmatch(marker)
	if match == nil {
		t.Fatalf("reduced output lacks a recovery handle (%d bytes)", len(marker))
	}
	return string(match[1])
}

func recoverE2E(t *testing.T, bin, home string, marker []byte) []byte {
	t.Helper()
	out, errOut, code := runOutputBinary(t, bin,
		isolatedOutputEnv(home, "BASHY_AGENTIC=1"), "out", markerHandle(t, marker))
	if code != 0 || len(errOut) != 0 {
		t.Fatalf("bashy out failed: exit=%d stdout=%d bytes stderr=%d bytes", code, len(out), len(errOut))
	}
	return out
}

func TestE2EOutputReductionStage1DefaultIsByteExact(t *testing.T) {
	bin := bashyBinary(t)
	want := sequenceBytes(20000)
	for _, agentic := range []string{"0", "1"} {
		t.Run("agentic="+agentic, func(t *testing.T) {
			out, errOut, code := runOutputBinary(t, bin,
				isolatedOutputEnv(t.TempDir(), "BASHY_AGENTIC="+agentic),
				"-c", "seq 1 20000")
			if code != 0 || len(errOut) != 0 {
				t.Fatalf("entrypoint failed: exit=%d stdout=%d bytes stderr=%d bytes", code, len(out), len(errOut))
			}
			requireExactBytes(t, "stdout", out, want)
		})
	}

	home := t.TempDir()
	const fixtureValue = "plain-fixture-value"
	wantOut, wantErr := externalFixtureBytes(fixtureValue)
	out, errOut, code := runOutputBinary(t, bin,
		isolatedOutputEnv(home, "BASHY_AGENTIC=1", "BASHY_E2E_SELF="+bin, "OUTPUT_SECRET="+fixtureValue),
		"-c", externalFixtureScript())
	if code != 0 {
		t.Fatalf("default external entrypoint failed: exit=%d stdout=%d bytes stderr=%d bytes", code, len(out), len(errOut))
	}
	requireExactBytes(t, "default external stdout", out, wantOut)
	requireExactBytes(t, "default external stderr", errOut, wantErr)
}

func TestE2EOutputReductionCoreutilsIsBoundedDeterministicAndRecoverable(t *testing.T) {
	bin := bashyBinary(t)
	home := t.TempDir()
	env := isolatedOutputEnv(home, "BASHY_AGENTIC=1")
	want := sequenceBytes(20000)
	var first []byte
	for i := 0; i < 2; i++ {
		out, errOut, code := runOutputBinary(t, bin, env, "--reduce", "-c", "seq 1 20000")
		if code != 0 || len(errOut) != 0 || len(out) > reduce.DefaultBudgetBytes {
			t.Fatalf("bounded coreutils run failed: exit=%d stdout=%d bytes stderr=%d bytes", code, len(out), len(errOut))
		}
		_ = markerHandle(t, out)
		if i == 0 {
			first = append([]byte(nil), out...)
		} else {
			requireExactBytes(t, "deterministic marker", out, first)
		}
	}
	requireExactBytes(t, "recovered coreutils stdout", recoverE2E(t, bin, home, first), want)

	scopedHome := t.TempDir()
	scoped, scopedErr, code := runOutputBinary(t, bin,
		isolatedOutputEnv(scopedHome, "BASHY_AGENTIC=1", "BASHY_OUTPUT_REDUCE=profile:sprint-123"),
		"-c", "seq 1 20000")
	if code != 0 || len(scopedErr) != 0 || len(scoped) > reduce.DefaultBudgetBytes {
		t.Fatalf("scoped opt-in failed: exit=%d stdout=%d bytes stderr=%d bytes", code, len(scoped), len(scopedErr))
	}
	_ = markerHandle(t, scoped)
}

func externalFixtureScript() string {
	return `BASHY_OUTPUT_REDUCE=off "$BASHY_E2E_SELF" -c 'i=0; while ((i < 2400)); do printf "stdout-%04d-%s\n" "$i" "$OUTPUT_SECRET"; printf "stderr-%04d-%s\n" "$i" "$OUTPUT_SECRET" >&2; ((i++)); done'`
}

func externalFixtureBytes(secret string) (stdout, stderr []byte) {
	var out, errOut bytes.Buffer
	for i := 0; i < 2400; i++ {
		fmt.Fprintf(&out, "stdout-%04d-%s\n", i, secret)
		fmt.Fprintf(&errOut, "stderr-%04d-%s\n", i, secret)
	}
	return out.Bytes(), errOut.Bytes()
}

func TestE2EOutputReductionExternalStreamsRedactBeforeSpill(t *testing.T) {
	bin := bashyBinary(t)
	home := t.TempDir()
	const secret = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	env := isolatedOutputEnv(home,
		"BASHY_AGENTIC=1", "BASHY_OUTPUT_REDUCE=on",
		"BASHY_E2E_SELF="+bin, "OUTPUT_SECRET="+secret)
	wantOut, wantErr := externalFixtureBytes(secret)
	mask := []byte("[redacted:github-token]")
	wantOut = bytes.ReplaceAll(wantOut, []byte(secret), mask)
	wantErr = bytes.ReplaceAll(wantErr, []byte(secret), mask)

	out1, err1, code := runOutputBinary(t, bin, env, "-c", externalFixtureScript())
	if code != 0 || len(out1) > reduce.DefaultBudgetBytes || len(err1) > reduce.DefaultBudgetBytes {
		t.Fatalf("bounded external run failed: exit=%d stdout=%d bytes stderr=%d bytes", code, len(out1), len(err1))
	}
	for name, stream := range map[string][]byte{"stdout": out1, "stderr": err1} {
		_ = markerHandle(t, stream)
		if bytes.Contains(stream, []byte(secret)) {
			t.Fatalf("synthetic secret appeared inline on %s", name)
		}
	}
	recoveredOut := recoverE2E(t, bin, home, out1)
	recoveredErr := recoverE2E(t, bin, home, err1)
	for name, stream := range map[string][]byte{"stdout": recoveredOut, "stderr": recoveredErr} {
		if bytes.Contains(stream, []byte(secret)) {
			t.Fatalf("synthetic secret appeared in recovered %s artifact", name)
		}
	}
	requireExactBytes(t, "recovered redacted stdout", recoveredOut, wantOut)
	requireExactBytes(t, "recovered redacted stderr", recoveredErr, wantErr)

	out2, err2, code := runOutputBinary(t, bin, env, "-c", externalFixtureScript())
	if code != 0 {
		t.Fatalf("repeat external run failed: exit=%d", code)
	}
	requireExactBytes(t, "deterministic external stdout marker", out2, out1)
	requireExactBytes(t, "deterministic external stderr marker", err2, err1)
}

func TestE2EOutputReductionRollbackAndDataPathsStayExact(t *testing.T) {
	bin := bashyBinary(t)
	want := sequenceBytes(20000)
	tests := []struct {
		name string
		env  []string
		args []string
	}{
		{"no-elide", []string{"BASHY_AGENTIC=1"}, []string{"--reduce", "--no-elide", "-c", "seq 1 20000"}},
		{"full-flag", []string{"BASHY_AGENTIC=1"}, []string{"--reduce", "--full", "-c", "seq 1 20000"}},
		{"off-beats-flag", []string{"BASHY_AGENTIC=1", "BASHY_OUTPUT_REDUCE=off"}, []string{"--reduce", "-c", "seq 1 20000"}},
		{"full-verb", []string{"BASHY_AGENTIC=1", "BASHY_OUTPUT_REDUCE=on"}, []string{"full", "--", "seq", "1", "20000"}},
		{"posix-default", []string{"BASHY_AGENTIC=1"}, []string{"--posix", "-c", "seq 1 20000"}},
		{"posix-cert", []string{"BASHY_AGENTIC=1", "BASHY_OUTPUT_REDUCE=on", "VSC_PROFILE=cert"}, []string{"--reduce", "--posix", "-c", "seq 1 20000"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runOutputBinary(t, bin, isolatedOutputEnv(t.TempDir(), tc.env...), tc.args...)
			if code != 0 || len(errOut) != 0 {
				t.Fatalf("rollback run failed: exit=%d stdout=%d bytes stderr=%d bytes", code, len(out), len(errOut))
			}
			requireExactBytes(t, "stdout", out, want)
		})
	}

	home := t.TempDir()
	env := isolatedOutputEnv(home, "BASHY_AGENTIC=1", "BASHY_OUTPUT_REDUCE=on")
	out, errOut, code := runOutputBinary(t, bin, env, "-c", "seq 1 20000 | wc -c")
	if code != 0 || len(errOut) != 0 {
		t.Fatalf("pipe run failed: exit=%d stdout=%d bytes stderr=%d bytes", code, len(out), len(errOut))
	}
	requireExactBytes(t, "pipe", out, []byte(fmt.Sprintf("%d\n", len(want))))

	redirected := filepath.Join(t.TempDir(), "complete.txt")
	env = append(env, "OUTPUT_REDIRECT="+redirected)
	out, errOut, code = runOutputBinary(t, bin, env, "-c", `seq 1 20000 > "$OUTPUT_REDIRECT"`)
	if code != 0 || len(out) != 0 || len(errOut) != 0 {
		t.Fatalf("redirect run failed: exit=%d stdout=%d bytes stderr=%d bytes", code, len(out), len(errOut))
	}
	redirectedBytes, err := os.ReadFile(redirected)
	if err != nil {
		t.Fatal(err)
	}
	requireExactBytes(t, "redirect", redirectedBytes, want)

	out, errOut, code = runOutputBinary(t, bin, env, "-c", `x=$(seq 1 20000); printf '%s\n' "${#x}"`)
	if code != 0 || len(errOut) != 0 {
		t.Fatalf("command substitution failed: exit=%d stdout=%d bytes stderr=%d bytes", code, len(out), len(errOut))
	}
	requireExactBytes(t, "command substitution", out, []byte(fmt.Sprintf("%d\n", len(want)-1)))
}

func TestE2EStandaloneBashNeverLinksOutputReduction(t *testing.T) {
	bin := standaloneBashBinary(t)
	want := sequenceBytes(20000)
	out, errOut, code := runOutputBinary(t, bin,
		isolatedOutputEnv(t.TempDir(), "BASHY_AGENTIC=1", "BASHY_OUTPUT_REDUCE=on"),
		"-c", `i=1; while ((i <= 20000)); do printf '%s\n' "$i"; ((i++)); done`)
	if code != 0 || len(errOut) != 0 {
		t.Fatalf("standalone cmd/bash failed: exit=%d stdout=%d bytes stderr=%d bytes", code, len(out), len(errOut))
	}
	requireExactBytes(t, "standalone cmd/bash stdout", out, want)
}
