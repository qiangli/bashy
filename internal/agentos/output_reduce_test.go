// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/reduce"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

func isolateOutputReduction(t *testing.T) {
	t.Helper()
	oldNoElide, oldReduce := noElideFlag, reduceFlag
	oldDryRun := *dryRunFlag
	t.Cleanup(func() {
		noElideFlag, reduceFlag = oldNoElide, oldReduce
		*dryRunFlag = oldDryRun
	})
	noElideFlag = false
	reduceFlag = optionalBool{}
	*dryRunFlag = false
	t.Setenv("BASHY_HOME", t.TempDir())
	t.Setenv("BASHY_AGENTIC", "1")
	t.Setenv("BASHY_OUTPUT_REDUCE", "")
	t.Setenv("VSC_PROFILE", "")
}

func TestOutputReductionActivationPrecedence(t *testing.T) {
	isolateOutputReduction(t)

	if outputReductionEnabled(os.Environ()) {
		t.Fatal("BASHY_AGENTIC alone enabled Stage 1 reduction")
	}
	t.Setenv("BASHY_AGENTIC", "0")
	if outputReductionEnabled(os.Environ()) {
		t.Fatal("ordinary shell enabled reduction")
	}
	for _, value := range []string{"on", "true", "1", "yes"} {
		t.Setenv("BASHY_OUTPUT_REDUCE", value)
		if !outputReductionEnabled(os.Environ()) {
			t.Fatalf("BASHY_OUTPUT_REDUCE=%s did not enable reduction", value)
		}
	}
	t.Setenv("BASHY_AGENTIC", "1")
	t.Setenv("BASHY_OUTPUT_REDUCE", "off")
	if outputReductionEnabled(os.Environ()) {
		t.Fatal("BASHY_OUTPUT_REDUCE=off did not beat agent mode")
	}
	reduceFlag = optionalBool{set: true, value: true}
	if outputReductionEnabled(os.Environ()) {
		t.Fatal("BASHY_OUTPUT_REDUCE=off did not beat --reduce")
	}
	t.Setenv("BASHY_OUTPUT_REDUCE", "")
	if !outputReductionEnabled(os.Environ()) {
		t.Fatal("--reduce did not enable reduction")
	}
	reduceFlag = optionalBool{}
	t.Setenv("BASHY_OUTPUT_REDUCE", "profile:sprint-123")
	if !outputReductionEnabled(os.Environ()) {
		t.Fatal("documented profile prefix did not enable reduction")
	}
	t.Setenv("BASHY_OUTPUT_REDUCE", "profile:")
	if outputReductionEnabled(os.Environ()) {
		t.Fatal("empty profile prefix enabled reduction")
	}
	reduceFlag = optionalBool{set: true, value: true}
	noElideFlag = true
	if outputReductionEnabled(os.Environ()) {
		t.Fatal("--no-elide did not beat --reduce and agent mode")
	}
	noElideFlag = false
	t.Setenv("VSC_PROFILE", "cert")
	if outputReductionEnabled(os.Environ()) {
		t.Fatal("certification mode must remain inert even with explicit activation")
	}
}

func TestOutputReductionFlagsRegistered(t *testing.T) {
	for _, name := range []string{"no-elide", "full", "reduce"} {
		if registered := flag.Lookup(name); registered == nil {
			t.Errorf("--%s is not registered", name)
		}
	}
}

func TestOutputShapeArgvNormalizesResolvedAndBashyShimPaths(t *testing.T) {
	for _, tc := range []struct {
		in   []string
		want []string
	}{
		{[]string{"/opt/go/bin/go", "test", "./..."}, []string{"go", "test", "./..."}},
		{[]string{"/opt/bashy/bin/bashy.real", "/opt/go/bin/go", "build"}, []string{"go", "build"}},
		{[]string{`C:\\bin\\npm.exe`, "test"}, []string{"npm", "test"}},
	} {
		got := outputShapeArgv(tc.in)
		if strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
			t.Errorf("outputShapeArgv(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func fixtureExec(outputs map[string][2]string, statuses map[string]int) func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			if pair, ok := outputs[strings.Join(args, " ")]; ok {
				hc := interp.HandlerCtx(ctx)
				_, _ = io.WriteString(hc.Stdout, pair[0])
				_, _ = io.WriteString(hc.Stderr, pair[1])
				if status := statuses[strings.Join(args, " ")]; status != 0 {
					return interp.NewExitStatus(uint8(status))
				}
				return nil
			}
			return next(ctx, args)
		}
	}
}

func runReducedSource(t *testing.T, source string, posix bool, fixture func(interp.ExecHandlerFunc) interp.ExecHandlerFunc) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	opts := []interp.RunnerOption{
		interp.Lang(syntax.LangBash),
		interp.Env(expand.ListEnviron("PATH=" + os.Getenv("PATH"))),
		interp.Dir(t.TempDir()),
	}
	opts = WireExec(opts, posix, os.Environ(), strings.NewReader(""), &out, &errOut)
	if fixture != nil {
		opts = append(opts, interp.ExecHandlers(fixture))
	}
	r, err := interp.New(opts...)
	if err != nil {
		t.Fatal(err)
	}
	file, err := syntax.NewParser().Parse(strings.NewReader(source), "output-reduce-test")
	if err != nil {
		t.Fatal(err)
	}
	err = r.Run(context.Background(), file)
	return out.String(), errOut.String(), err
}

var recoveryHandleRE = regexp.MustCompile(`bashy out ([0-9a-f]{8,64})`)

func recoverMarker(t *testing.T, marker string) []byte {
	t.Helper()
	m := recoveryHandleRE.FindStringSubmatch(marker)
	if m == nil {
		t.Fatalf("output has no recovery command: %q", marker)
	}
	store, err := shellOutputStore()
	if err != nil {
		t.Fatal(err)
	}
	var recovered bytes.Buffer
	if err := reduce.Recover(store, m[1], &recovered); err != nil {
		t.Fatalf("recover %s: %v", m[1], err)
	}
	return recovered.Bytes()
}

func TestShellOutputReductionExternalStreamsAndDeterministicRecovery(t *testing.T) {
	isolateOutputReduction(t)
	t.Setenv("BASHY_OUTPUT_REDUCE", "on")
	stdout := strings.Repeat("build ok\n", 80)
	stderr := strings.Repeat("progress\n", 40)
	fx := fixtureExec(map[string][2]string{"go test": {stdout, stderr}}, nil)

	out1, err1, runErr := runReducedSource(t, "go test", false, fx)
	if runErr != nil {
		t.Fatal(runErr)
	}
	for name, got := range map[string]string{"stdout": out1, "stderr": err1} {
		if lines := strings.Count(got, "\n"); lines != 1 {
			t.Errorf("%s verdict view has %d lines, want one: %q", name, lines, got)
		}
		if !strings.Contains(got, "go test succeeded") {
			t.Errorf("%s lacks verdict marker: %q", name, got)
		}
	}
	if got := string(recoverMarker(t, out1)); got != stdout {
		t.Fatalf("stdout recovery differs: got %d bytes, want %d", len(got), len(stdout))
	}
	if got := string(recoverMarker(t, err1)); got != stderr {
		t.Fatalf("stderr recovery differs: got %d bytes, want %d", len(got), len(stderr))
	}

	out2, err2, runErr := runReducedSource(t, "go test", false, fx)
	if runErr != nil {
		t.Fatal(runErr)
	}
	if out2 != out1 || err2 != err1 {
		t.Fatalf("identical output was not deterministic\nfirst: %q / %q\nsecond: %q / %q", out1, err1, out2, err2)
	}
}

func TestShellOutputReductionCoversCoreutilsAndPOSIX(t *testing.T) {
	isolateOutputReduction(t)
	t.Setenv("BASHY_OUTPUT_REDUCE", "on")
	for _, posix := range []bool{false, true} {
		out, errOut, runErr := runReducedSource(t, "seq 1 20000", posix, nil)
		if runErr != nil {
			t.Fatalf("posix=%v: %v (%s)", posix, runErr, errOut)
		}
		if !strings.Contains(out, "bashy out ") || len(out) > reduce.DefaultBudgetBytes {
			t.Fatalf("posix=%v: coreutils output was not bounded: %d bytes", posix, len(out))
		}
		recovered := recoverMarker(t, out)
		if !bytes.HasPrefix(recovered, []byte("1\n2\n")) || !bytes.HasSuffix(recovered, []byte("20000\n")) {
			t.Fatalf("posix=%v: recovered coreutils bytes are incomplete", posix)
		}
	}
}

func TestShellOutputReductionPreservesFailureEvidence(t *testing.T) {
	isolateOutputReduction(t)
	t.Setenv("BASHY_OUTPUT_REDUCE", "on")
	stdout := strings.Repeat("context\n", 7000)
	stderr := strings.Repeat("FAIL file.go:42\n", 4000)
	fx := fixtureExec(map[string][2]string{"go test": {stdout, stderr}}, map[string]int{"go test": 2})
	out, errOut, runErr := runReducedSource(t, "go test", false, fx)
	var status interp.ExitStatus
	if !errors.As(runErr, &status) || int(status) != 2 {
		t.Fatalf("exit = %v, want 2", runErr)
	}
	if out != stdout || errOut != stderr {
		t.Fatalf("failure evidence changed: stdout %d/%d stderr %d/%d", len(out), len(stdout), len(errOut), len(stderr))
	}
}

func TestShellOutputReductionRedactsUnreducedFailureEvidence(t *testing.T) {
	isolateOutputReduction(t)
	t.Setenv("BASHY_OUTPUT_REDUCE", "on")
	const credential = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	stdout := strings.Repeat("context "+credential+"\n", 20)
	stderr := "FAIL " + credential + "\n"
	fx := fixtureExec(map[string][2]string{"go test": {stdout, stderr}}, map[string]int{"go test": 2})
	out, errOut, runErr := runReducedSource(t, "go test", false, fx)
	var status interp.ExitStatus
	if !errors.As(runErr, &status) || int(status) != 2 {
		t.Fatalf("exit = %v, want 2", runErr)
	}
	if strings.Contains(out, credential) || strings.Contains(errOut, credential) {
		t.Fatal("credential-shaped value escaped in failure evidence")
	}
	if !strings.Contains(out, "[redacted:github-token]") || !strings.Contains(errOut, "[redacted:github-token]") {
		t.Fatal("failure evidence did not pass through the redaction gate")
	}
}

func TestShellOutputReductionRedactsBeforeSpill(t *testing.T) {
	isolateOutputReduction(t)
	t.Setenv("BASHY_OUTPUT_REDUCE", "on")
	const credential = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	full := strings.Repeat(credential+"\n", 2400)
	fx := fixtureExec(map[string][2]string{"dump result": {full, ""}}, nil)
	out, errOut, runErr := runReducedSource(t, "dump result", false, fx)
	if runErr != nil || errOut != "" {
		t.Fatalf("secret fixture failed: run=%v stderr=%d bytes", runErr, len(errOut))
	}
	recovered := recoverMarker(t, out)
	if bytes.Contains(recovered, []byte(credential)) {
		t.Fatal("credential-shaped value reached the recovery artifact")
	}
	if !bytes.Contains(recovered, []byte("[redacted:github-token]")) {
		t.Fatalf("recovered artifact lacks shape mask (%d bytes)", len(recovered))
	}
}

func TestShellOutputReductionDoesNotChangeDataSinks(t *testing.T) {
	isolateOutputReduction(t)
	t.Setenv("BASHY_OUTPUT_REDUCE", "on")
	full := sequenceBytes(20000)

	out, errOut, err := runReducedSource(t, "seq 1 20000 | wc -c", false, nil)
	if err != nil || errOut != "" || strings.TrimSpace(out) != strconv.Itoa(len(full)) {
		t.Fatalf("pipe saw %q / %q / %v, want byte count %d", out, errOut, err, len(full))
	}

	path := filepath.Join(t.TempDir(), "complete.txt")
	out, errOut, err = runReducedSource(t, fmt.Sprintf("seq 1 20000 > %q", path), false, nil)
	if err != nil || out != "" || errOut != "" {
		t.Fatalf("redirect emitted %q / %q / %v", out, errOut, err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil || !bytes.Equal(data, full) {
		t.Fatalf("redirect changed bytes: read=%v got=%d want=%d", readErr, len(data), len(full))
	}

	out, errOut, err = runReducedSource(t, `x=$(seq 1 20000); printf '%s\n' "${#x}"`, false, nil)
	if err != nil || errOut != "" || strings.TrimSpace(out) != strconv.Itoa(len(full)-1) {
		t.Fatalf("command substitution saw %q / %q / %v, want %d", out, errOut, err, len(full)-1)
	}
}

func sequenceBytes(last int) []byte {
	var b strings.Builder
	for i := 1; i <= last; i++ {
		fmt.Fprintln(&b, i)
	}
	return []byte(b.String())
}

func TestDryRunAndReducerShareOneStdoutChoice(t *testing.T) {
	isolateOutputReduction(t)
	t.Setenv("BASHY_OUTPUT_REDUCE", "on")
	*dryRunFlag = true
	out, _, err := runReducedSource(t, "printf builtin-output", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Fatalf("agent dry-run script stdout escaped the composed discard: %q", out)
	}
}
