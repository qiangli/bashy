// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
		time.Sleep(30 * time.Second)
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
		Timeout: 30 * time.Second,
	}
}

func runSpecForTest(t *testing.T, spec RepairSuiteSpec) RepairSuiteResult {
	t.Helper()
	res, err := runRepairSuite(spec, RepairProfileOptions{
		Root:      t.TempDir(),
		TailBytes: 4096,
		Goos:      runtime.GOOS,
		Arch:      runtime.GOARCH,
	})
	if err != nil {
		t.Fatalf("runRepairSuite(%s): %v", spec.Name, err)
	}
	return res
}

// --- evidence writer ---

func TestWriteRepairEvidenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "evidence.json")
	code := 0
	ev := &RepairProfileEvidence{
		Schema: RepairProfileSchema, Component: "bashy", Commit: "deadbee",
		OS: "linux", Arch: "amd64", Start: "2026-10-31T00:00:00Z", End: "2026-10-31T00:01:00Z",
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
	if len(back.Suites) != 1 || back.Suites[0].Name != "make-test" {
		t.Fatalf("round trip lost suites: %+v", back.Suites)
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
	if res.Complete || res.Pass {
		t.Fatalf("signal-killed suite: complete=%v pass=%v — killed is never a pass", res.Complete, res.Pass)
	}
	if !strings.Contains(res.Reason, "signal") && !strings.Contains(res.Reason, "killed") {
		t.Fatalf("reason = %q, want it to name the signal kill", res.Reason)
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
		Args: []string{"repairprofile-no-such-binary-xyz", "--version"}, Timeout: time.Second}
	res, err := runRepairSuite(spec, RepairProfileOptions{Root: t.TempDir(), TailBytes: 4096})
	if err != nil {
		t.Fatalf("a start failure is a failed suite, not a runner error: %v", err)
	}
	if res.Complete || res.Pass {
		t.Fatalf("a command that never started: complete=%v pass=%v", res.Complete, res.Pass)
	}
	if !strings.Contains(res.Reason, "start") {
		t.Fatalf("reason = %q, want a start failure", res.Reason)
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
	ev, err := RunRepairProfile(opts)
	if err != nil {
		t.Fatalf("RunRepairProfile: %v", err)
	}
	return ev
}

func TestRepairProfileAllGreenPasses(t *testing.T) {
	ev := runProfileForTest(t, RepairProfileOptions{
		Component: "bashy",
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
}

func TestRepairProfileOneFailureBlocksPass(t *testing.T) {
	ev := runProfileForTest(t, RepairProfileOptions{
		Component: "bashy",
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
				Fixtures: true, Timeout: time.Minute, Command: "make test-bash"},
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
