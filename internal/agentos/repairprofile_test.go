// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestRepairSuiteHelperProcess is the fake-suite subprocess the profile-runner
// tests drive (the standard GO_WANT_HELPER_PROCESS pattern; see
// transpile_agentos_test.go). It never does anything under a normal `go test`.
func TestRepairSuiteHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	switch os.Getenv("REPAIR_HELPER_BEHAVIOR") {
	case "true":
		fmt.Println("helper: ok")
		os.Exit(0)
	case "false":
		fmt.Println("helper: deliberate failure")
		os.Exit(1)
	case "sleep":
		fmt.Println("helper: sleeping past the suite timeout")
		// Well past the 500 ms suite bound but short enough that the pipe a
		// watchdog orphan holds (WaitDelay) does not slow the test batch.
		time.Sleep(5 * time.Second)
		os.Exit(0)
	case "flood":
		chunk := strings.Repeat("x", 4096)
		for i := 0; i < 64; i++ { // 256 KiB, far past any test tail cap
			fmt.Println(chunk)
		}
		os.Exit(0)
	case "signal":
		if runtime.GOOS == "windows" {
			os.Exit(1)
		}
		p, _ := os.FindProcess(os.Getpid())
		_ = p.Signal(syscall.SIGKILL)
		time.Sleep(10 * time.Second)
		os.Exit(0)
	case "gotestish":
		fmt.Println("=== RUN   TestA")
		fmt.Println("--- PASS: TestA (0.00s)")
		fmt.Println("--- PASS: TestB (0.00s)")
		fmt.Println("--- FAIL: TestC (0.00s)")
		fmt.Println("--- SKIP: TestD (0.00s)")
		fmt.Println("PASS")
		os.Exit(1)
	}
	os.Exit(3)
}

// helperSpec builds a suite spec that runs this test binary in helper mode.
func helperSpec(t *testing.T, name string, behavior string) RepairSuiteSpec {
	t.Helper()
	return RepairSuiteSpec{
		Name:    name,
		Lane:    "native",
		Kind:    "argv",
		Args:    []string{os.Args[0], "-test.run=TestRepairSuiteHelperProcess", "-test.v"},
		Env:     []string{"GO_WANT_HELPER_PROCESS=1", "REPAIR_HELPER_BEHAVIOR=" + behavior},
		Timeout: 60 * time.Second,
	}
}

// repairTestBashy is the REAL bashy binary the R4b suite tests run every
// suite inside — resolved the same way the synergy dag tests resolve it
// (BASHY_E2E_BIN, the checkout's bin/bashy, else a plain `go build` into a
// per-process temp dir), built at most once per test binary. The test binary
// itself is never exec'd as bashy.
var (
	repairBashyOnce sync.Once
	repairBashyBin  string
)

func repairTestBashy(t *testing.T) string {
	t.Helper()
	repairBashyOnce.Do(func() {
		if p := strings.TrimSpace(os.Getenv("BASHY_E2E_BIN")); p != "" {
			repairBashyBin = p
			return
		}
		root, ok := findBashySourceRoot(mustGetwd())
		if !ok {
			return
		}
		suffix := ""
		if runtime.GOOS == "windows" {
			suffix = ".exe"
		}
		built := filepath.Join(root, "bin", "bashy"+suffix)
		if st, err := os.Stat(built); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			repairBashyBin = built
			return
		}
		if testing.Short() {
			return
		}
		dir, err := os.MkdirTemp("", "bashy-repair-test-")
		if err != nil {
			return
		}
		out := filepath.Join(dir, "bashy"+suffix)
		cmd := exec.Command("go", "build", "-o", out, "./cmd/bashy")
		cmd.Dir = root
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Logf("build bashy: %v\n%s", err, b)
			return
		}
		repairBashyBin = out
	})
	if repairBashyBin == "" {
		t.Skip("needs a built bashy: run `make build-bashy` or set BASHY_E2E_BIN (not built under -short)")
	}
	return repairBashyBin
}

func runSpecForTest(t *testing.T, spec RepairSuiteSpec) RepairSuiteResult {
	t.Helper()
	ev := runProfileForTest(t, RepairProfileOptions{
		Component:    "bashy",
		Bashy:        repairTestBashy(t),
		CustomSuites: []RepairSuiteSpec{spec},
	})
	if len(ev.Suites) != 1 {
		t.Fatalf("expected exactly one suite result, got %d", len(ev.Suites))
	}
	return ev.Suites[0]
}

// --- evidence writer ---

func TestWriteRepairEvidenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "evidence.json")
	code := 0
	ev := &RepairProfileEvidence{
		Schema: RepairProfileSchema, Component: "bashy", Commit: "deadbee",
		OS: "linux", Arch: "amd64", Start: "2026-10-31T00:00:00Z", End: "2026-10-31T00:01:00Z",
		Runtime: &RepairRuntime{
			Bashy: RepairBashyIdent{Path: "/opt/bashy/bin/bashy", Version: "bashy 5.3.0(1)-bashy-dev"},
			Tools: []RepairToolResolution{{Tool: "go", Provider: "bashy go (pinned provider go1.27.2)"}},
		},
		Suites: []RepairSuiteResult{
			{Name: "make-test", Lane: "native", Command: "make test", ExitCode: &code,
				Complete: true, Pass: true, OutputTail: "ok"},
		},
		Complete: true, Pass: true,
	}
	if err := WriteRepairEvidence(path, ev); err != nil {
		t.Fatalf("WriteRepairEvidence: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var back RepairProfileEvidence
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("evidence is not JSON: %v\n%s", err, b)
	}
	if back.Schema != "bashy-repair-profile-v1" {
		t.Fatalf("schema = %q, want bashy-repair-profile-v1", back.Schema)
	}
	if !back.Pass || !back.Complete || back.Component != "bashy" || back.Commit != "deadbee" {
		t.Fatalf("round trip lost fields: %+v", back)
	}
	if back.Runtime == nil || back.Runtime.Bashy.Path != "/opt/bashy/bin/bashy" ||
		len(back.Runtime.Tools) != 1 || back.Runtime.Tools[0].Tool != "go" {
		t.Fatalf("round trip lost the runtime block: %+v", back.Runtime)
	}
	if len(back.Suites) != 1 || back.Suites[0].Name != "make-test" {
		t.Fatalf("round trip lost suites: %+v", back.Suites)
	}
}

// --- the R4b invocation: every suite runs as `<bashy> -c …` ---

func TestRepairCommandStringQuotesShellWords(t *testing.T) {
	if got := repairCommandString([]string{"make", "test"}); got != "make test" {
		t.Fatalf("plain suite argv must render readably: %q", got)
	}
	if got := repairCommandString([]string{"go", "test", "-run", "Two Words", "./..."}); got != `go test -run 'Two Words' ./...` {
		t.Fatalf("shell-reparsed words must be quoted: %q", got)
	}
	if got := repairCommandString([]string{"make", "VAR=a'b"}); got != `make 'VAR=a'\''b'` {
		t.Fatalf("embedded quote must be escaped: %q", got)
	}
}

func TestRepairSuiteEnvPinsWrapperPathAndCompilers(t *testing.T) {
	shims := filepath.Join(t.TempDir(), "shims") // any label; env math only
	env := repairSuiteEnv(RepairProfileOptions{Env: []string{"HOME=/h", "PATH=/usr/bin"}},
		RepairSuiteSpec{Env: []string{"CUSTOM=1"}}, shims)
	m := map[string]string{}
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		m[k] = v
	}
	if want := shims + string(os.PathListSeparator) + "/usr/bin"; m["PATH"] != want {
		t.Fatalf("PATH = %q, want the wrapper dir FIRST: %q", m["PATH"], want)
	}
	if m["CC"] != filepath.Join(shims, repairShimName("cc")) {
		t.Fatalf("CC = %q, want the wrapper default", m["CC"])
	}
	if m["CXX"] != filepath.Join(shims, repairShimName("c++")) {
		t.Fatalf("CXX = %q, want the wrapper default", m["CXX"])
	}
	if m["CUSTOM"] != "1" {
		t.Fatal("spec env entries must apply")
	}

	// A provided compiler is never overridden.
	env = repairSuiteEnv(RepairProfileOptions{Env: []string{"PATH=/x", "CC=clang-14", "CXX=clang++-14"}}, RepairSuiteSpec{}, shims)
	m = map[string]string{}
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		m[k] = v
	}
	if m["CC"] != "clang-14" || m["CXX"] != "clang++-14" {
		t.Fatalf("an environment-provided compiler must stand: CC=%q CXX=%q", m["CC"], m["CXX"])
	}

	// A stripped PATH stays stripped down to the wrapper dir (hermetic runs).
	env = repairSuiteEnv(RepairProfileOptions{Env: []string{"PATH="}}, RepairSuiteSpec{}, shims)
	for _, e := range env {
		if k, v, _ := strings.Cut(e, "="); k == "PATH" && v != shims {
			t.Fatalf("empty PATH must become exactly the wrapper dir: %q", v)
		}
	}
}

func TestRepairUnitsMaterializeAsBashSharpShims(t *testing.T) {
	units := repairUnitsIndex(findRepairUnitsDir(RepairProfileOptions{}))
	for _, tool := range []string{"make", "go", "cc", "c++"} {
		if _, canonical, ok := repairToolRowFor(tool); !ok || !units.rowAvailable(canonical) {
			t.Fatalf("the repo's ci/bin units must provide %q (units dir %q)", tool, units.dir)
		}
	}
	bashy := "/opt/bashy/bin/bashy"
	if runtime.GOOS == "windows" {
		bashy = `C:\opt\bashy\bin\bashy.exe`
	}
	dir, err := materializeRepairShims(units, bashy)
	if err != nil {
		t.Fatalf("materializeRepairShims: %v", err)
	}
	defer os.RemoveAll(dir)
	for _, name := range []string{"go", "cc", "gcc", "clang", "c++", "g++", "clang++"} {
		p := filepath.Join(dir, name)
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("shim %s: %v", name, err)
		}
		if !strings.HasPrefix(string(b), "#!"+bashy+" --bashsharp\n") {
			t.Fatalf("shim %s must run through the pinned bashy as a Bash# unit:\n%s", name, b)
		}
		if fi, err := os.Stat(p); err == nil && runtime.GOOS != "windows" && fi.Mode()&0o111 == 0 {
			t.Fatalf("shim %s must be executable", name)
		}
	}
	if cc, err := os.ReadFile(filepath.Join(dir, "cc")); err == nil {
		if !strings.Contains(string(cc), `zig cc "$@"`) {
			t.Fatalf("cc shim must run bashy zig cc:\n%s", cc)
		}
	}
	if goShim, err := os.ReadFile(filepath.Join(dir, "go")); err == nil {
		if !strings.Contains(string(goShim), `"`+bashy+`" go "$@"`) {
			t.Fatalf("go shim must run the pinned bashy's go:\n%s", goShim)
		}
	}
	if runtime.GOOS == "windows" {
		b, err := os.ReadFile(filepath.Join(dir, "cc.cmd"))
		if err != nil || !strings.Contains(string(b), "zig cc") {
			t.Fatalf("windows cc.cmd twin missing or wrong:\n%v\n%s", err, b)
		}
	}
	// A bashy path with whitespace cannot ride a shebang — refuse clearly.
	if _, err := materializeRepairShims(units, "/opt/ba shy/bashy"); err == nil {
		t.Fatal("a whitespace bashy path must be refused, not half-work")
	}
}

func TestRepairBashyResolutionPrecedence(t *testing.T) {
	// Explicit wins over everything else.
	dir := t.TempDir()
	fake := filepath.Join(dir, "bashy")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveRepairBashy(fake)
	if err != nil || got != fake {
		t.Fatalf("explicit bashy = %q, %v; want %q", got, err, fake)
	}
	// Nothing anywhere → hard error (the profile never falls back to host
	// make/go). BASHY_SELF is honored before PATH.
	t.Setenv("PATH", "")
	t.Setenv("BASHY_SELF", "")
	if _, err := resolveRepairBashy(""); err == nil {
		t.Fatal("no bashy anywhere must be an error, not a silent host-binary run")
	}
	t.Setenv("BASHY_SELF", fake)
	if got, err := resolveRepairBashy(""); err != nil || got != fake {
		t.Fatalf("BASHY_SELF = %q, %v; want %q", got, err, fake)
	}
}

// --- commit identity for trees without .git (R4b) ---

// repairDummyBashy is a stat-valid placeholder for tests whose suites all
// skip before any execution — resolution only checks the file exists; nothing
// ever exec's it (and never the test binary).
func repairDummyBashy(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "bashy")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRepairCommitOverrideBeatsUnknown(t *testing.T) {
	// A suite needing a nonexistent tool skips before any exec, so this runs
	// hermetically with no bashy involved.
	skip := RepairSuiteSpec{Name: "f95", Lane: "native", Kind: "argv",
		Args: []string{"f95", "x"}, Tools: []string{"f95"}, Command: "f95 x"}
	ev := runProfileForTest(t, RepairProfileOptions{
		Component:    "bashy",
		Bashy:        repairDummyBashy(t),
		Commit:       "feedc0d",
		CustomSuites: []RepairSuiteSpec{skip},
	})
	if ev.Commit != "feedc0d" {
		t.Fatalf("-commit must be recorded verbatim: %q", ev.Commit)
	}
	ev = runProfileForTest(t, RepairProfileOptions{
		Component:    "bashy",
		Bashy:        repairDummyBashy(t),
		CustomSuites: []RepairSuiteSpec{skip},
	})
	// t.TempDir() has no .git — R4a recorded "unknown"; R4b keeps that honest
	// fallback while BASHY_REPAIR_COMMIT/-commit override it.
	if ev.Commit != "unknown" {
		t.Fatalf("a tree with no .git and no -commit records the fact, want \"unknown\": %q", ev.Commit)
	}
}

// --- suites needing tools the runtime cannot provide: skip, never pass ---

func TestRepairMissingToolSkipsSuiteNeverPasses(t *testing.T) {
	ev := runProfileForTest(t, RepairProfileOptions{
		Component: "bashy",
		Bashy:     repairDummyBashy(t),
		CustomSuites: []RepairSuiteSpec{
			{Name: "fortran-check", Lane: "native", Kind: "argv",
				Args: []string{"f95", "x"}, Tools: []string{"f95"}, Command: "f95 x"},
		},
	})
	entry := ev.Suites[0]
	if !entry.Skipped || entry.Pass || entry.Complete {
		t.Fatalf("missing-tool suite: skipped=%v pass=%v complete=%v — never a pass",
			entry.Skipped, entry.Pass, entry.Complete)
	}
	want := "skipped (tool f95 unavailable in bashy runtime)"
	if entry.Reason != want {
		t.Fatalf("reason = %q, want %q", entry.Reason, want)
	}
	if ev.Complete || ev.Pass {
		t.Fatal("a profile whose only suite never ran must fail closed")
	}
}

func TestRepairMissingWrapperUnitSkipsGoSuites(t *testing.T) {
	// The tool table knows `go`, but the wrapper units are unavailable (a
	// bad explicit units dir is authoritative, never a fallback) — the suite
	// skips naming the tool, and a sibling native suite still passes.
	ok := RepairSuiteSpec{Name: "ok", Lane: "native", Kind: "argv",
		Args: []string{"true"}, Command: "true", Timeout: 30 * time.Second}
	needsGo := RepairSuiteSpec{Name: "go-suite", Lane: "native", Kind: "argv",
		Args: []string{"go", "version"}, Tools: []string{"go"}, Command: "go version"}
	ev := runProfileForTest(t, RepairProfileOptions{
		Component:    "bashy",
		Bashy:        repairTestBashy(t),
		UnitsDir:     filepath.Join(t.TempDir(), "no-units"),
		CustomSuites: []RepairSuiteSpec{ok, needsGo},
	})
	if !ev.Pass || !ev.Complete {
		t.Fatalf("the runnable suite must still carry the profile: complete=%v pass=%v (%s)",
			ev.Complete, ev.Pass, ev.Reason)
	}
	var skip *RepairSuiteResult
	for i := range ev.Suites {
		if ev.Suites[i].Name == "go-suite" {
			skip = &ev.Suites[i]
		}
	}
	if skip == nil || !skip.Skipped || skip.Pass {
		t.Fatalf("go suite without its wrapper unit must be skipped, never pass: %+v", skip)
	}
	if !strings.Contains(skip.Reason, "skipped (tool go unavailable in bashy runtime)") {
		t.Fatalf("reason = %q, want the tool-unavailable skip naming go", skip.Reason)
	}
}

// --- per-suite completeness rules (killed or partial must NEVER be pass) ---

func TestRepairSuiteTruePasses(t *testing.T) {
	res := runSpecForTest(t, helperSpec(t, "true-suite", "true"))
	if !res.Complete || !res.Pass {
		t.Fatalf("true suite: complete=%v pass=%v reason=%q", res.Complete, res.Pass, res.Reason)
	}
	if res.ExitCode == nil || *res.ExitCode != 0 {
		t.Fatalf("exit code = %v, want 0", res.ExitCode)
	}
	if res.Skipped {
		t.Fatal("a run suite must not be marked skipped")
	}
}

func TestRepairSuiteFalseFailsButCompletes(t *testing.T) {
	res := runSpecForTest(t, helperSpec(t, "false-suite", "false"))
	if !res.Complete {
		t.Fatalf("exit-1 suite ran to completion: reason=%q", res.Reason)
	}
	if res.Pass {
		t.Fatal("exit 1 must not pass")
	}
	if res.ExitCode == nil || *res.ExitCode != 1 {
		t.Fatalf("exit code = %v, want 1", res.ExitCode)
	}
}

func TestRepairSuiteTimeoutKilledNeverPasses(t *testing.T) {
	spec := helperSpec(t, "sleep-suite", "sleep")
	spec.Timeout = 500 * time.Millisecond
	res := runSpecForTest(t, spec)
	if res.Complete {
		t.Fatal("a timeout-killed suite must not be complete")
	}
	if res.Pass {
		t.Fatal("a timeout-killed suite must NEVER be pass")
	}
	if !strings.Contains(res.Reason, "timeout") {
		t.Fatalf("reason = %q, want it to name the timeout kill", res.Reason)
	}
}

func TestRepairSuiteSignalKilledNeverPasses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("signal-shape test is unix-only; the timeout-kill test covers the windows kill path")
	}
	res := runSpecForTest(t, helperSpec(t, "signal-suite", "signal"))
	// The suite payload kills itself; the bashy shell carrying it survives to
	// report 128+9 — a completed run that FAILED, never a pass.
	if res.Pass {
		t.Fatal("a signal-killed payload must NEVER be pass")
	}
	if res.ExitCode == nil || *res.ExitCode != 137 {
		t.Fatalf("exit code = %v, want 137 (128+SIGKILL)", res.ExitCode)
	}
	if !res.Complete {
		t.Fatalf("the shell reported the kill (exit 137): complete=%v reason=%q", res.Complete, res.Reason)
	}
}

func TestRepairSuiteTruncatedOutputTailBounded(t *testing.T) {
	res := runSpecForTest(t, helperSpec(t, "flood-suite", "flood"))
	if !res.Complete || !res.Pass {
		t.Fatalf("flood suite exited on its own: complete=%v pass=%v reason=%q", res.Complete, res.Pass, res.Reason)
	}
	if !res.OutputTruncated {
		t.Fatal("256 KiB of output against a 4 KiB cap must set output_truncated")
	}
	if len(res.OutputTail) > 4096 {
		t.Fatalf("output_tail is %d bytes, cap is 4096", len(res.OutputTail))
	}
	if !strings.Contains(res.OutputTail, "x") {
		t.Fatal("output_tail must hold the TAIL of the output")
	}
}

func TestRepairSuiteCountsParsedWhenParseable(t *testing.T) {
	res := runSpecForTest(t, helperSpec(t, "gotestish-suite", "gotestish"))
	if res.Counts == nil {
		t.Fatal("go-test-shaped output must yield counts")
	}
	if res.Counts.Pass != 2 || res.Counts.Fail != 1 || res.Counts.Skip != 1 {
		t.Fatalf("counts = %+v, want pass=2 fail=1 skip=1", res.Counts)
	}
}

func TestRepairSuiteMissingCommandFailsClosed(t *testing.T) {
	spec := RepairSuiteSpec{Name: "no-such-bin", Lane: "native", Kind: "argv",
		Args: []string{"repairprofile-no-such-binary-xyz", "--version"}, Timeout: time.Minute}
	res := runSpecForTest(t, spec)
	if res.Pass {
		t.Fatal("a command that resolves nowhere inside the runtime must not pass")
	}
	// The bashy shell ran the command and reported 127 — a completed, failed
	// suite (start failures are now failures OF the bashy runtime itself).
	if res.ExitCode == nil || *res.ExitCode != 127 {
		t.Fatalf("exit code = %v, want 127 (command not found in the bashy runtime)", res.ExitCode)
	}
	if !res.Complete {
		t.Fatalf("the shell reported 127 itself: complete=%v reason=%q", res.Complete, res.Reason)
	}
}

// --- aggregate evidence rules ---

func runProfileForTest(t *testing.T, opts RepairProfileOptions) *RepairProfileEvidence {
	t.Helper()
	if opts.Root == "" {
		opts.Root = t.TempDir()
	}
	opts.Out = filepath.Join(t.TempDir(), "evidence.json")
	opts.Goos = runtime.GOOS
	opts.Arch = runtime.GOARCH
	opts.TailBytes = 4096
	opts.Stderr = nil
	ev, err := RunRepairProfile(opts)
	if err != nil {
		t.Fatalf("RunRepairProfile: %v", err)
	}
	return ev
}

func TestRepairProfileAllGreenPasses(t *testing.T) {
	bin := repairTestBashy(t)
	ev := runProfileForTest(t, RepairProfileOptions{
		Component: "bashy",
		Bashy:     bin,
		CustomSuites: []RepairSuiteSpec{
			helperSpec(t, "ok1", "true"),
			helperSpec(t, "ok2", "true"),
		},
	})
	if !ev.Complete || !ev.Pass {
		t.Fatalf("all-green profile: complete=%v pass=%v reason=%q", ev.Complete, ev.Pass, ev.Reason)
	}
	if len(ev.Suites) != 2 {
		t.Fatalf("suites = %d, want 2", len(ev.Suites))
	}
	if ev.Schema != RepairProfileSchema {
		t.Fatalf("schema = %q", ev.Schema)
	}
	if ev.Commit == "" {
		t.Fatal("evidence must record a commit (or \"unknown\")")
	}
	// R4b: the runtime block names the bashy that ran every suite.
	if ev.Runtime == nil || ev.Runtime.Bashy.Path != bin {
		t.Fatalf("runtime.bashy.path = %+v, want the pinned test bashy %q", ev.Runtime, bin)
	}
	if ev.Runtime.Bashy.Version == "" {
		t.Fatal("runtime.bashy.version must carry the --version banner")
	}
}

func TestRepairProfileOneFailureBlocksPass(t *testing.T) {
	ev := runProfileForTest(t, RepairProfileOptions{
		Component: "bashy",
		Bashy:     repairTestBashy(t),
		CustomSuites: []RepairSuiteSpec{
			helperSpec(t, "ok", "true"),
			helperSpec(t, "bad", "false"),
		},
	})
	if !ev.Complete {
		t.Fatalf("both suites ran to completion: reason=%q", ev.Reason)
	}
	if ev.Pass {
		t.Fatal("one failing suite must fail the profile")
	}
}

func TestRepairProfileKilledSuiteBlocksPassAndCompletion(t *testing.T) {
	sleep := helperSpec(t, "sleep", "sleep")
	sleep.Timeout = 500 * time.Millisecond
	ev := runProfileForTest(t, RepairProfileOptions{
		Component:    "bashy",
		Bashy:        repairTestBashy(t),
		CustomSuites: []RepairSuiteSpec{helperSpec(t, "ok", "true"), sleep},
	})
	if ev.Complete {
		t.Fatal("a killed suite makes the run incomplete")
	}
	if ev.Pass {
		t.Fatal("a killed suite must NEVER be pass")
	}
}

func TestRepairProfileLaneUnavailableRecordsSkipNeverPass(t *testing.T) {
	landmine := helperSpec(t, "test-uutils", "true")
	landmine.Lane = "oci-linux"
	ev := runProfileForTest(t, RepairProfileOptions{
		Component:    "bashy",
		Bashy:        repairTestBashy(t),
		CustomSuites: []RepairSuiteSpec{helperSpec(t, "ok", "true"), landmine},
		LaneProbe:    func() (bool, string) { return false, "oci runtime not found on this host" },
	})
	if !ev.Complete || !ev.Pass {
		t.Fatalf("a structurally unavailable landmine lane must not fail the native profile: complete=%v pass=%v", ev.Complete, ev.Pass)
	}
	var skip *RepairSuiteResult
	for i := range ev.Suites {
		if ev.Suites[i].Name == "test-uutils" {
			skip = &ev.Suites[i]
		}
	}
	if skip == nil {
		t.Fatal("the skipped landmine suite must still appear in the evidence")
	}
	if !skip.Skipped || skip.Pass || skip.Complete {
		t.Fatalf("skipped entry: skipped=%v pass=%v complete=%v — never a pass", skip.Skipped, skip.Pass, skip.Complete)
	}
	if !strings.Contains(skip.Reason, "skipped (lane unavailable") {
		t.Fatalf("reason = %q, want the spec's skipped (lane unavailable) wording", skip.Reason)
	}
}

func TestRepairProfileNoEligibleSuiteFailsClosed(t *testing.T) {
	landmine := helperSpec(t, "test-uutils", "true")
	landmine.Lane = "oci-linux"
	ev := runProfileForTest(t, RepairProfileOptions{
		Component:    "bashy",
		Bashy:        repairTestBashy(t),
		CustomSuites: []RepairSuiteSpec{landmine},
		LaneProbe:    func() (bool, string) { return false, "oci runtime not found on this host" },
	})
	if ev.Complete || ev.Pass {
		t.Fatal("a profile where nothing ran must fail closed, never pass")
	}
	if !strings.Contains(ev.Reason, "no suite ran") {
		t.Fatalf("reason = %q, want the fail-closed no-suite explanation", ev.Reason)
	}
}

// --- the hermetic proof: stripped PATH, only the bashy binary supplied ---

// repairHermeticEnv is a bare environment: no PATH content, no inherited
// toolchain vars — whatever the suites resolve must come from the bashy
// runtime itself (the wrapper dir the runner prepends).
func repairHermeticEnv(t *testing.T) []string {
	t.Helper()
	env := []string{
		"HOME=" + os.Getenv("HOME"),
		"TMPDIR=" + os.TempDir(),
		"PATH=",
	}
	if runtime.GOOS == "windows" {
		for _, k := range []string{"SYSTEMROOT", "SYSTEMDRIVE", "COMSPEC", "USERPROFILE", "LOCALAPPDATA"} {
			if v, ok := os.LookupEnv(k); ok {
				env = append(env, k+"="+v)
			}
		}
	}
	return env
}

// TestRepairHermeticStrippedPathRunsThroughBashyRuntime is the S549.R4b
// acceptance lane: with PATH stripped to an empty dir and only the bashy
// binary supplied, a profile whose suites run `go version` and
// `make --version` succeeds via bashy's own commands — the pinned Go
// provider through the ci/bin/go.md wrapper on the suite's private PATH, the
// in-process coreutils make with no PATH at all.
func TestRepairHermeticStrippedPathRunsThroughBashyRuntime(t *testing.T) {
	bin := repairTestBashy(t)
	ev := runProfileForTest(t, RepairProfileOptions{
		Component: "bashy",
		Bashy:     bin,
		Env:       repairHermeticEnv(t),
		CustomSuites: []RepairSuiteSpec{
			{Name: "go-version", Lane: "native", Kind: "argv",
				Args: []string{"go", "version"}, Tools: []string{"go"},
				Timeout: 10 * time.Minute, Command: "go version"},
			{Name: "make-version", Lane: "native", Kind: "argv",
				Args: []string{"make", "--version"}, Tools: []string{"make"},
				Timeout: time.Minute, Command: "make --version"},
		},
	})
	if !ev.Complete || !ev.Pass {
		var tails []string
		for _, s := range ev.Suites {
			tails = append(tails, fmt.Sprintf("%s: pass=%v reason=%q tail=%q", s.Name, s.Pass, s.Reason, s.OutputTail))
		}
		t.Fatalf("hermetic profile must pass via bashy's own commands: complete=%v pass=%v\n%s",
			ev.Complete, ev.Pass, strings.Join(tails, "\n"))
	}
	if ev.Runtime == nil || ev.Runtime.Bashy.Path != bin {
		t.Fatalf("evidence must name the bashy that ran the suites: %+v", ev.Runtime)
	}
	resolutions := map[string]RepairToolResolution{}
	for _, r := range ev.Runtime.Tools {
		resolutions[r.Tool] = r
	}
	if r, ok := resolutions["go"]; !ok || !strings.Contains(r.Provider, "bashy go") {
		t.Fatalf("go resolution = %+v, want the pinned bashy go provider", r)
	}
	if r, ok := resolutions["make"]; !ok || !strings.Contains(r.Provider, "coreutils make") {
		t.Fatalf("make resolution = %+v, want the in-process coreutils make", r)
	}
}

// --- built-in matrix ---

func TestRepairSuitesForMatrix(t *testing.T) {
	bashy, err := RepairSuitesFor("bashy")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]RepairSuiteSpec{}
	for _, s := range bashy {
		names[s.Name] = s
	}
	for _, want := range []string{"make-test", "test-bash", "test-uutils"} {
		if _, ok := names[want]; !ok {
			t.Fatalf("bashy profile is missing suite %q: %+v", want, bashy)
		}
	}
	if names["test-uutils"].Lane != "oci-linux" {
		t.Fatalf("test-uutils lane = %q, want oci-linux", names["test-uutils"].Lane)
	}
	if names["test-bash"].Lane != "native" || !names["test-bash"].Fixtures {
		t.Fatalf("test-bash must be the native lane and need the sha-pinned fixture cache: %+v", names["test-bash"])
	}
	for _, s := range bashy {
		if len(s.Tools) != 1 || s.Tools[0] != "make" {
			t.Fatalf("bashy suites run make inside the bashy shell: %+v", s.Tools)
		}
		if strings.Contains(strings.ToLower(s.Name), "vsc") || strings.Contains(strings.ToLower(s.Name), "pcts") {
			t.Fatalf("VSC/PCTS is excluded from repair profiles: %q", s.Name)
		}
	}

	outpost, err := RepairSuitesFor("outpost")
	if err != nil {
		t.Fatal(err)
	}
	if len(outpost) != 1 || outpost[0].Name != "go-test-short-headless" {
		t.Fatalf("outpost profile = %+v", outpost)
	}
	spec := outpost[0]
	if spec.Kind != "go-test-list" || len(spec.Exclude) != 1 || spec.Exclude[0] != "internal/agent/shell" {
		t.Fatalf("outpost suite must be go-test-list excluding internal/agent/shell: %+v", spec)
	}
	if len(spec.Tools) != 1 || spec.Tools[0] != "go" {
		t.Fatalf("outpost suite needs go from the bashy runtime: %+v", spec.Tools)
	}

	if _, err := RepairSuitesFor("nonsense"); err == nil {
		t.Fatal("an unknown component must be a usage error")
	}
}

// --- offline fixture cache (the bash 5.3 fetch-by-SHA lane) ---

func TestRepairFixturesOfflineEmptyCacheFailsClearly(t *testing.T) {
	root := t.TempDir()
	cache := t.TempDir()
	err := ensureRepairFixtures(root, cache, true)
	if err == nil {
		t.Fatal("offline + empty cache must fail clearly")
	}
	if !strings.Contains(err.Error(), "BASHY_OFFLINE") {
		t.Fatalf("error must name BASHY_OFFLINE: %v", err)
	}
}

func TestRepairFixturesOfflineValidCachePassesFromCache(t *testing.T) {
	root := t.TempDir()
	cache := t.TempDir()
	dst := filepath.Join(cache, "bashy", "conformance", "bash-5.3")
	if err := os.MkdirAll(filepath.Join(dst, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	sha := bash53TarballSHA256 + "\n"
	if err := os.WriteFile(filepath.Join(dst, ".bashy-source-sha256"), []byte(sha), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureRepairFixtures(root, cache, true); err != nil {
		t.Fatalf("offline run with a valid sha-pinned cache must pass from the cache: %v", err)
	}
}

func TestRepairFixturesExistingLinkIsNoOp(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "external", "bash-5.3", "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensureRepairFixtures(root, t.TempDir(), true); err != nil {
		t.Fatalf("an already-linked fixture tree is a no-op even offline: %v", err)
	}
}

func TestRepairProfileFixtureFailureFailsClosed(t *testing.T) {
	ev := runProfileForTest(t, RepairProfileOptions{
		Component: "bashy",
		CustomSuites: []RepairSuiteSpec{
			{Name: "test-bash", Lane: "native", Kind: "argv", Args: []string{"make", "test-bash"},
				Tools: []string{"make"}, Fixtures: true, Timeout: time.Minute, Command: "make test-bash"},
		},
		CacheBase: t.TempDir(), // empty cache: offline + Fixtures must fail closed
		Offline:   true,
	})
	if ev.Complete || ev.Pass {
		t.Fatal("an unavailable fixture cache must fail the profile closed")
	}
	if !strings.Contains(ev.Reason, "BASHY_OFFLINE") {
		t.Fatalf("reason = %q, want the offline cache failure", ev.Reason)
	}
}

// --- the dag recipe (dry-run: per-OS tasks for both components) ---

func TestRepairProfilesRecipeListsPerOSTasks(t *testing.T) {
	recipe := filepath.Join("..", "..", "ci", "repair-profiles.dag.md")
	var out, errOut bytes.Buffer
	if code := runDagDispatch([]string{"-f", recipe, "--list"}, &out, &errOut); code != 0 {
		t.Fatalf("dag --list exit = %d stderr=%s", code, errOut.String())
	}
	listed := out.String()
	for _, want := range []string{
		"profile",
		"bashy-linux", "bashy-darwin", "bashy-windows",
		"outpost-linux", "outpost-darwin", "outpost-windows",
		"bashy-uutils-linux-oci",
	} {
		if !strings.Contains(listed, want) {
			t.Fatalf("recipe --list missing %q:\n%s", want, listed)
		}
	}
}

func TestRepairProfilesRecipeChecksClean(t *testing.T) {
	recipe := filepath.Join("..", "..", "ci", "repair-profiles.dag.md")
	var out, errOut bytes.Buffer
	if code := runDagDispatch([]string{"-f", recipe, "--check"}, &out, &errOut); code != 0 {
		t.Fatalf("dag --check exit = %d\nstdout=%s\nstderr=%s", code, out.String(), errOut.String())
	}
}

func TestRepairProfilesRecipeDryRunPlansProfile(t *testing.T) {
	recipe := filepath.Join("..", "..", "ci", "repair-profiles.dag.md")
	var out, errOut bytes.Buffer
	code := runDagDispatch([]string{"-f", recipe, "--dry-run", "profile", "COMPONENT=outpost"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("dag --dry-run exit = %d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String()+errOut.String(), "profile") {
		t.Fatalf("dry-run plan does not name the profile target:\n%s", out.String())
	}
}

func TestRepairProfileListOnlySelectingNothingErrors(t *testing.T) {
	var out bytes.Buffer
	_, err := ListRepairProfile(RepairProfileOptions{
		Component: "bashy",
		Only:      []string{"no-such-suite"},
	}, &out)
	if err == nil {
		t.Fatal("-only selecting no suite must be a usage error in -list too, not an empty plan")
	}
	if !strings.Contains(err.Error(), "no suite") {
		t.Fatalf("error = %v, want the -only selected no suite wording", err)
	}
	if strings.Contains(out.String(), "component=") {
		t.Fatalf("a failed plan must not print a header first:\n%s", out.String())
	}
}
