package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

var bash53FixtureNames = []string{
	"alias", "appendop", "arith", "arith-for", "array", "array2", "assoc", "attr",
	"braces", "builtins", "case", "casemod", "complete", "comsub", "comsub-eof",
	"comsub-posix", "comsub2", "cond", "coproc", "cprint", "dbg-support",
	"dbg-support2", "dirstack", "dollars", "dynvar", "errors", "execscript",
	"exp-tests", "exportfunc", "extglob", "extglob2", "extglob3", "func",
	"getopts", "glob-bracket", "glob-test", "globstar", "heredoc", "herestr",
	"histexpand", "history", "ifs", "ifs-posix", "input-test", "intl", "invert",
	"invocation", "iquote", "jobs", "lastpipe", "mapfile", "more-exp", "nameref",
	"new-exp", "nquote", "nquote1", "nquote2", "nquote3", "nquote4", "nquote5",
	"parser", "posix2", "posixexp", "posixexp2", "posixpat", "posixpipe",
	"precedence", "printf", "procsub", "quote", "quotearray", "read", "redir",
	"rhs-exp", "rsh", "set-e", "set-x", "shopt", "strip", "test", "tilde",
	"tilde2", "trap", "type", "varenv", "vredir",
}

func TestCommittedChunkManifestCoversEveryBash53FixtureExactlyOnce(t *testing.T) {
	root := repoRoot(t)
	manifest, err := loadChunkManifest(filepath.Join(root, "chunks.json"))
	if err != nil {
		t.Fatal(err)
	}
	fixtures := fixturesFromNames(bash53FixtureNames)
	if err := validateChunkManifest(manifest, fixtures); err != nil {
		t.Fatal(err)
	}

	seen := map[string]int{}
	for _, chunk := range manifest.Chunks {
		if chunk.Seconds <= 0 {
			t.Fatalf("chunk %d duration = %v, want > 0", chunk.ID, chunk.Seconds)
		}
		for _, f := range chunk.Fixtures {
			if f.Seconds <= 0 {
				t.Fatalf("fixture %s duration = %v, want > 0", f.Name, f.Seconds)
			}
			seen[f.Name]++
		}
	}
	if got, want := len(seen), len(bash53FixtureNames); got != want {
		t.Fatalf("manifest names = %d, want %d", got, want)
	}
}

func TestChunkManifestValidationRejectsCoverageLoss(t *testing.T) {
	fixtures := fixturesFromNames([]string{"a", "b", "c"})
	valid := &chunkManifest{
		SchemaVersion: 1,
		Suite:         "bash-5.3",
		ChunkCount:    2,
		Chunks: []manifestChunk{
			{ID: 1, Fixtures: []manifestFixture{{Name: "a"}}},
			{ID: 2, Fixtures: []manifestFixture{{Name: "b"}, {Name: "c"}}},
		},
	}
	if err := validateChunkManifest(valid, fixtures); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}

	missing := *valid
	missing.Chunks = []manifestChunk{
		{ID: 1, Fixtures: []manifestFixture{{Name: "a"}}},
		{ID: 2, Fixtures: []manifestFixture{{Name: "b"}}},
	}
	if err := validateChunkManifest(&missing, fixtures); err == nil {
		t.Fatal("missing fixture accepted")
	}

	duplicate := *valid
	duplicate.Chunks = []manifestChunk{
		{ID: 1, Fixtures: []manifestFixture{{Name: "a"}, {Name: "b"}}},
		{ID: 2, Fixtures: []manifestFixture{{Name: "b"}, {Name: "c"}}},
	}
	if err := validateChunkManifest(&duplicate, fixtures); err == nil {
		t.Fatal("duplicate fixture accepted")
	}

	unknown := *valid
	unknown.Chunks = []manifestChunk{
		{ID: 1, Fixtures: []manifestFixture{{Name: "a"}}},
		{ID: 2, Fixtures: []manifestFixture{{Name: "b"}, {Name: "c"}, {Name: "d"}}},
	}
	if err := validateChunkManifest(&unknown, fixtures); err == nil {
		t.Fatal("unknown fixture accepted")
	}
}

func TestManifestChunkSelectionIgnoresFleetSizeModulo(t *testing.T) {
	fixtures := fixturesFromNames([]string{"a", "b", "c", "d"})
	manifest := &chunkManifest{
		SchemaVersion: 1,
		Suite:         "bash-5.3",
		ChunkCount:    2,
		Chunks: []manifestChunk{
			{ID: 1, Fixtures: []manifestFixture{{Name: "d"}, {Name: "b"}}},
			{ID: 2, Fixtures: []manifestFixture{{Name: "a"}, {Name: "c"}}},
		},
	}
	selected := selectFixtures(fixtures, nil, "1/2", manifest)
	got := fixtureNames(selected)
	want := []string{"b", "d"}
	if !sameStrings(got, want) {
		t.Fatalf("selected fixtures = %v, want %v", got, want)
	}
}

func TestShardSelectionDeterministicFromDiscoveryOrder(t *testing.T) {
	testsDir, bashPath := makePassingSuite(t, []string{"zeta", "alpha", "middle"})
	args := []string{"--json", "--shared-tree", "--tests-dir", testsDir, "--bash", bashPath, "--of", "2", "--shard", "0"}
	runNames := func() []string {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("run exit = %d, stderr:\n%s", code, stderr.String())
		}
		report := decodeSingleReport(t, stdout.Bytes())
		return verdictNames(report.Verdicts)
	}
	first := runNames()
	second := runNames()
	if got, want := first, []string{"alpha", "zeta"}; !sameStringsInOrder(got, want) {
		t.Fatalf("first shard = %v, want %v", got, want)
	}
	if got, want := second, first; !sameStringsInOrder(got, want) {
		t.Fatalf("repeated shard = %v, want %v", got, want)
	}
}

func TestShardOfOneSelectsAll(t *testing.T) {
	fixtures := fixturesFromNames([]string{"a", "b", "c", "d"})
	selected := shardFixtures(fixtures, 1, 0)
	if got, want := fixtureNamesInOrder(selected), fixtureNamesInOrder(fixtures); !sameStringsInOrder(got, want) {
		t.Fatalf("selected fixtures = %v, want %v", got, want)
	}
}

func TestTwoShardsAreCompleteAndDisjoint(t *testing.T) {
	fixtures := fixturesFromNames([]string{"a", "b", "c", "d", "e"})
	left := shardFixtures(fixtures, 2, 0)
	right := shardFixtures(fixtures, 2, 1)
	seen := map[string]int{}
	for _, group := range [][]fixture{left, right} {
		for _, f := range group {
			seen[f.Name]++
		}
	}
	if len(seen) != len(fixtures) {
		t.Fatalf("partition covers %d fixtures, want %d: %v", len(seen), len(fixtures), seen)
	}
	for _, f := range fixtures {
		if seen[f.Name] != 1 {
			t.Fatalf("fixture %q appears %d times, want exactly once", f.Name, seen[f.Name])
		}
	}
}

func TestJSONOutputIsOneValidDocument(t *testing.T) {
	testsDir, bashPath := makePassingSuite(t, []string{"alpha"})
	var stdout, stderr bytes.Buffer
	code := run([]string{"--json", "--shared-tree", "--tests-dir", testsDir, "--bash", bashPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run exit = %d, stderr:\n%s", code, stderr.String())
	}
	report := decodeSingleReport(t, stdout.Bytes())
	if report.SchemaVersion != 1 || report.Suite != "bash-5.3" {
		t.Fatalf("report identity = schema %d suite %q", report.SchemaVersion, report.Suite)
	}
	if report.Infrastructure.Status != "ok" || len(report.Infrastructure.PreflightErrors) != 0 {
		t.Fatalf("infrastructure = %+v", report.Infrastructure)
	}
	if len(report.Verdicts) != 1 || report.Verdicts[0].Name != "alpha" || report.Verdicts[0].Verdict != "passed" {
		t.Fatalf("verdicts = %+v", report.Verdicts)
	}
	if report.Summary != (jsonSummary{Passed: 1}) {
		t.Fatalf("summary = %+v", report.Summary)
	}
}

func TestJSONChunkMetadataAndSelection(t *testing.T) {
	testsDir, bashPath := makePassingSuite(t, []string{"alpha", "beta"})
	manifestPath := filepath.Join(t.TempDir(), "chunks.json")
	writeTestFile(t, manifestPath, `{
  "schema_version": 1,
  "suite": "bash-5.3",
  "chunk_count": 2,
  "measurement": {},
  "chunks": [
    {"id": 1, "fixtures": [{"name": "beta"}]},
    {"id": 2, "fixtures": [{"name": "alpha"}]}
  ]
}`)
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--json", "--shared-tree", "--tests-dir", testsDir, "--bash", bashPath,
		"--chunk", "1/2", "--chunks-manifest", manifestPath,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run exit = %d, stderr:\n%s", code, stderr.String())
	}
	report := decodeSingleReport(t, stdout.Bytes())
	if report.Chunk != (jsonChunk{Index: 1, Of: 2}) {
		t.Fatalf("chunk = %+v, want index 1 of 2", report.Chunk)
	}
	if len(report.Verdicts) != 1 || report.Verdicts[0].Name != "beta" {
		t.Fatalf("verdicts = %+v, want beta only", report.Verdicts)
	}
}

func TestJSONInfrastructureFailureHasNoVerdicts(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--json", "--tests-dir", t.TempDir(), "--bash", filepath.Join(t.TempDir(), "missing-bash")}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run exit = %d, want 2; stderr:\n%s", code, stderr.String())
	}
	report := decodeSingleReport(t, stdout.Bytes())
	if report.Infrastructure.Status != "failed" || len(report.Infrastructure.PreflightErrors) == 0 {
		t.Fatalf("infrastructure = %+v", report.Infrastructure)
	}
	if len(report.Verdicts) != 0 {
		t.Fatalf("verdicts = %+v, want empty", report.Verdicts)
	}
	if report.Summary != (jsonSummary{}) {
		t.Fatalf("summary = %+v, want zero values", report.Summary)
	}
}

func fixtureNamesInOrder(fixtures []fixture) []string {
	out := make([]string, 0, len(fixtures))
	for _, f := range fixtures {
		out = append(out, f.Name)
	}
	return out
}

func verdictNames(verdicts []jsonVerdict) []string {
	out := make([]string, 0, len(verdicts))
	for _, verdict := range verdicts {
		out = append(out, verdict.Name)
	}
	return out
}

func sameStringsInOrder(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func makePassingSuite(t *testing.T, names []string) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell script")
	}
	dir := t.TempDir()
	testsDir := filepath.Join(dir, "tests")
	if err := os.MkdirAll(testsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		writeTestFile(t, filepath.Join(testsDir, "run-"+name), "")
		writeTestFile(t, filepath.Join(testsDir, name+".tests"), name+"\n")
		writeTestFile(t, filepath.Join(testsDir, name+".right"), name+"\n")
	}
	bashPath := filepath.Join(dir, "fake-bash")
	writeTestFile(t, bashPath, "#!/bin/sh\ncat \"$1\"\n")
	if err := os.Chmod(bashPath, 0o755); err != nil {
		t.Fatal(err)
	}
	return testsDir, bashPath
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func decodeSingleReport(t *testing.T, data []byte) jsonReport {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	var report jsonReport
	if err := dec.Decode(&report); err != nil {
		t.Fatalf("decode report: %v\nstdout: %s", err, data)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		t.Fatalf("stdout contains more than one JSON document: %v\nstdout: %s", err, data)
	}
	return report
}

func fixturesFromNames(names []string) []fixture {
	out := make([]fixture, 0, len(names))
	for _, name := range names {
		out = append(out, fixture{Name: name})
	}
	return out
}

func fixtureNames(fixtures []fixture) []string {
	out := make([]string, 0, len(fixtures))
	for _, f := range fixtures {
		out = append(out, f.Name)
	}
	sort.Strings(out)
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			t.Fatal("could not find repo root")
		}
		dir = next
	}
}

// normalizeHostSignalOrder must reorder a trap listing without ever changing
// which trap lines are present — the regression it has to stay blind to is an
// ORDER difference (SIGUSR1 sorts below SIGTERM on Linux, above it on Darwin),
// and the regression it must still catch is an EXTRA or MISSING trap line, which
// is the shape of the spurious `trap -- '' SIGINT` the baseline once carried.
func TestNormalizeHostSignalOrder(t *testing.T) {
	linux := "this is bashenv\n" +
		"trap -- 'echo EXIT' EXIT\n" +
		"trap -- 'echo USR1' SIGUSR1\n" +
		"trap -- '' SIGTERM\n" +
		"USR1\n"
	darwin := "this is bashenv\n" +
		"trap -- 'echo EXIT' EXIT\n" +
		"trap -- '' SIGTERM\n" +
		"trap -- 'echo USR1' SIGUSR1\n" +
		"USR1\n"

	gotLinux := string(normalizeHostSignalOrder("execscript", []byte(linux)))
	gotDarwin := string(normalizeHostSignalOrder("execscript", []byte(darwin)))
	if gotLinux != gotDarwin {
		t.Fatalf("host signal order not normalized:\nlinux:  %q\ndarwin: %q", gotLinux, gotDarwin)
	}

	// An extra trap line must still diff.
	extra := "this is bashenv\n" +
		"trap -- 'echo EXIT' EXIT\n" +
		"trap -- '' SIGINT\n" +
		"trap -- 'echo USR1' SIGUSR1\n" +
		"trap -- '' SIGTERM\n" +
		"USR1\n"
	if got := string(normalizeHostSignalOrder("execscript", []byte(extra))); got == gotLinux {
		t.Fatal("a spurious trap line was normalized away; it must still diff")
	}

	// Other fixtures are untouched.
	if got := string(normalizeHostSignalOrder("trap", []byte(linux))); got != linux {
		t.Fatalf("non-execscript fixture was rewritten: %q", got)
	}
}
