// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/release"
	"github.com/spf13/cobra"
)

// releaseTestConfig is a minimal but REAL config: two platforms, one archive
// name template, and the SHA256SUMS manifest name the fleet-upgrade contract
// consumes. It exercises the same path a project config would.
const releaseTestConfig = `
project_name: demo
builds:
  - id: demo
    main: ./cmd/demo
    binary: demo
    targets: [linux_amd64, windows_amd64]
archives:
  - id: demo
    formats: [tar.gz]
    name_template: '{{ .ProjectName }}-{{ .Os }}-{{ .Arch }}'
checksum:
  name_template: SHA256SUMS
`

// newReleaseProject writes a project root holding cfg, and (for the
// --skip-build path) the binaries the plan expects to find already built.
func newReleaseProject(t *testing.T, cfg string, prebuilt map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".goreleaser.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	for rel, content := range prebuilt {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// demoPrebuilt is where the plan puts each target's binary: dist/<build
// id>_<goos>_<goarch>/<binary>.
var demoPrebuilt = map[string]string{
	"dist/demo_linux_amd64/demo":       "linux binary bytes\n",
	"dist/demo_windows_amd64/demo.exe": "windows binary bytes\n",
}

func runReleaseCmd(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := releaseCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	cmd.SilenceErrors = true
	silenceCobraUsage(cmd)
	err = cmd.Execute()
	return out.String(), errb.String(), err
}

// silenceCobraUsage keeps a usage error from dumping help into the captured
// streams, so an assertion reads the diagnostic and not the help text.
func silenceCobraUsage(cmd *cobra.Command) {
	cmd.SilenceUsage = true
	for _, c := range cmd.Commands() {
		silenceCobraUsage(c)
	}
}

func readLedger(t *testing.T, path string) release.Ledger {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	var l release.Ledger
	if err := json.Unmarshal(b, &l); err != nil {
		t.Fatalf("ledger json: %v\n%s", err, b)
	}
	return l
}

// tarGzMembers lists the in-archive names and contents of a tar.gz.
func tarGzMembers(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("%s: not a gzip stream: %v", path, err)
	}
	defer gz.Close()
	out := map[string]string{}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("%s: tar: %v", path, err)
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		out[hdr.Name] = string(b)
	}
	return out
}

// TestReleaseSnapshotEmitsArtifactsChecksumsAndLedger is the exit criterion of
// the wiring: one invocation produces the archives, the checksum manifest, and
// the bashy-release-v1 ledger — and the ledger's digests are the ones actually
// written to disk.
func TestReleaseSnapshotEmitsArtifactsChecksumsAndLedger(t *testing.T) {
	dir := newReleaseProject(t, releaseTestConfig, demoPrebuilt)

	stdout, _, err := runReleaseCmd(t, "--snapshot", "--dir", dir, "--version", "0.1.0", "--skip-build")
	if err != nil {
		t.Fatalf("release --snapshot: %v", err)
	}

	dist := filepath.Join(dir, "dist")
	for _, name := range []string{"demo-linux-amd64.tar.gz", "demo-windows-amd64.tar.gz", "SHA256SUMS", releaseLedgerName} {
		if _, err := os.Stat(filepath.Join(dist, name)); err != nil {
			t.Errorf("missing artifact %s: %v", name, err)
		}
	}

	ledger := readLedger(t, filepath.Join(dist, releaseLedgerName))
	if ledger.Schema != release.LedgerSchema {
		t.Errorf("ledger schema = %q, want %q", ledger.Schema, release.LedgerSchema)
	}
	if ledger.Version != "0.1.0" || !ledger.Snapshot || ledger.ProjectName != "demo" {
		t.Errorf("ledger = %+v, want demo/0.1.0/snapshot", ledger)
	}
	if len(ledger.Artifacts) != 3 { // two archives + the checksum manifest
		t.Fatalf("ledger artifacts = %d, want 3: %+v", len(ledger.Artifacts), ledger.Artifacts)
	}

	// The recorded digest must be the digest of the file on disk — a ledger
	// that describes bytes nobody can verify is the failure this whole verb
	// exists to prevent.
	sums, err := os.ReadFile(filepath.Join(dist, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range ledger.Artifacts {
		if a.Type != "archive" {
			continue
		}
		want := a.SHA256 + "  " + a.Name
		if !strings.Contains(string(sums), want) {
			t.Errorf("SHA256SUMS is missing %q:\n%s", want, sums)
		}
	}
	if strings.Contains(string(sums), releaseLedgerName) {
		t.Errorf("the ledger must not be listed in SHA256SUMS (it is written after hashing):\n%s", sums)
	}

	// The archive holds the binary under its plain name, not a dist/ path.
	members := tarGzMembers(t, filepath.Join(dist, "demo-linux-amd64.tar.gz"))
	if got := members["demo"]; got != demoPrebuilt["dist/demo_linux_amd64/demo"] {
		t.Errorf("archive member demo = %q, want the built binary's bytes (members: %v)", got, members)
	}
	if got := tarGzMembers(t, filepath.Join(dist, "demo-windows-amd64.tar.gz")); got["demo.exe"] == "" {
		t.Errorf("windows archive must carry demo.exe, got members %v", got)
	}

	// The human summary names the version, the artifacts and the ledger.
	for _, want := range []string{"demo 0.1.0 (snapshot)", "demo-linux-amd64.tar.gz", "SHA256SUMS", release.LedgerSchema} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout is missing %q:\n%s", want, stdout)
		}
	}
}

// The --json form is what an agent consumes: the ledger itself on stdout.
func TestReleaseSnapshotJSONEmitsTheLedger(t *testing.T) {
	dir := newReleaseProject(t, releaseTestConfig, demoPrebuilt)
	stdout, _, err := runReleaseCmd(t, "--snapshot", "--dir", dir, "--version", "v0.2.0", "--skip-build", "--json")
	if err != nil {
		t.Fatalf("release --snapshot --json: %v", err)
	}
	var ledger release.Ledger
	if err := json.Unmarshal([]byte(stdout), &ledger); err != nil {
		t.Fatalf("stdout is not the ledger: %v\n%s", err, stdout)
	}
	if ledger.Schema != release.LedgerSchema {
		t.Errorf("schema = %q", ledger.Schema)
	}
	// A leading v is stripped: the version is what goes INSIDE the name, and
	// the tag form belongs to the tag.
	if ledger.Version != "0.2.0" {
		t.Errorf("version = %q, want 0.2.0", ledger.Version)
	}
	for _, a := range ledger.Artifacts {
		if a.SHA256 == "" || a.Size == 0 {
			t.Errorf("artifact %s has no digest/size: %+v", a.Name, a)
		}
	}
}

// Determinism is the T0 exit criterion ("byte-for-byte where the build is
// deterministic"): the same inputs must produce the same archive bytes, or a
// checksum comparison across two runs means nothing.
func TestReleaseSnapshotIsByteDeterministic(t *testing.T) {
	first := newReleaseProject(t, releaseTestConfig, demoPrebuilt)
	second := newReleaseProject(t, releaseTestConfig, demoPrebuilt)
	for _, dir := range []string{first, second} {
		if _, _, err := runReleaseCmd(t, "--snapshot", "--dir", dir, "--version", "0.1.0", "--skip-build"); err != nil {
			t.Fatalf("release: %v", err)
		}
	}
	a := readLedger(t, filepath.Join(first, "dist", releaseLedgerName))
	b := readLedger(t, filepath.Join(second, "dist", releaseLedgerName))
	if len(a.Artifacts) != len(b.Artifacts) {
		t.Fatalf("artifact counts differ: %d vs %d", len(a.Artifacts), len(b.Artifacts))
	}
	for i := range a.Artifacts {
		if a.Artifacts[i].Name != b.Artifacts[i].Name || a.Artifacts[i].SHA256 != b.Artifacts[i].SHA256 {
			t.Errorf("run-to-run drift: %+v vs %+v", a.Artifacts[i], b.Artifacts[i])
		}
	}
}

// A config declaring a stage this tier does not implement must be refused BY
// NAME, before anything is built — never a run that silently ships less.
func TestReleaseRefusesUnimplementedStageByName(t *testing.T) {
	cfg := releaseTestConfig + "\nsigns:\n  - cmd: cosign\n"
	dir := newReleaseProject(t, cfg, demoPrebuilt)
	_, _, err := runReleaseCmd(t, "--snapshot", "--dir", dir, "--version", "0.1.0", "--skip-build")
	if err == nil {
		t.Fatal("a config with signs: must be refused")
	}
	if !errors.Is(err, release.ErrUnsupportedStage) || !strings.Contains(err.Error(), "signs") {
		t.Errorf("error must name the stage: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "dist", "SHA256SUMS")); statErr == nil {
		t.Error("a refused config must not have produced artifacts")
	}
}

func TestReleaseMissingConfigNamesWhatItLookedFor(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runReleaseCmd(t, "--snapshot", "--dir", dir, "--version", "0.1.0")
	if err == nil {
		t.Fatal("a project with no config must fail")
	}
	for _, want := range []string{".goreleaser.yaml", "--config"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("diagnostic must mention %q: %v", want, err)
		}
	}
}

// Outside a git repository there is no commit for the default snapshot version
// template to name the build after. That must be an error pointing at
// --version, not a build named after an empty string.
func TestReleaseSnapshotOutsideGitDemandsAVersion(t *testing.T) {
	dir := newReleaseProject(t, releaseTestConfig, demoPrebuilt)
	_, _, err := runReleaseCmd(t, "--snapshot", "--dir", dir, "--skip-build")
	if err == nil {
		t.Fatal("a snapshot with no commit and no --version must fail")
	}
	if !strings.Contains(err.Error(), "--version") {
		t.Errorf("diagnostic must point at --version: %v", err)
	}
}

// `release plan` answers "what would this produce" without compiling or
// writing anything.
func TestReleasePlanBuildsNothing(t *testing.T) {
	dir := newReleaseProject(t, releaseTestConfig, nil)
	stdout, _, err := runReleaseCmd(t, "plan", "--dir", dir, "--version", "0.1.0", "--json")
	if err != nil {
		t.Fatalf("release plan: %v", err)
	}
	var plan struct {
		Schema   string           `json:"schema_version"`
		Version  string           `json:"version"`
		Dist     string           `json:"dist"`
		Targets  []map[string]any `json:"targets"`
		Archives []map[string]any `json:"archives"`
		Checksum string           `json:"checksum"`
	}
	if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
		t.Fatalf("plan json: %v\n%s", err, stdout)
	}
	if plan.Schema != releasePlanSchema || len(plan.Targets) != 2 || len(plan.Archives) != 2 || plan.Checksum != "SHA256SUMS" {
		t.Errorf("plan = %+v", plan)
	}
	if _, err := os.Stat(filepath.Join(dir, "dist")); err == nil {
		t.Error("`release plan` must not create the output directory")
	}
}

func TestReleaseCheckValidatesConfig(t *testing.T) {
	dir := newReleaseProject(t, releaseTestConfig, nil)
	stdout, _, err := runReleaseCmd(t, "check", "--dir", dir, "--version", "0.1.0")
	if err != nil {
		t.Fatalf("release check: %v", err)
	}
	if !strings.Contains(stdout, "config ok") || !strings.Contains(stdout, "2 archives") {
		t.Errorf("check output = %q", stdout)
	}
}

// --dist selects the output directory, and cannot be used to write outside the
// project root.
func TestReleaseDistOverride(t *testing.T) {
	dir := newReleaseProject(t, releaseTestConfig, map[string]string{
		"out/demo_linux_amd64/demo":       "linux\n",
		"out/demo_windows_amd64/demo.exe": "windows\n",
	})
	if _, _, err := runReleaseCmd(t, "--snapshot", "--dir", dir, "--dist", "out", "--version", "0.1.0", "--skip-build"); err != nil {
		t.Fatalf("release --dist out: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "out", "SHA256SUMS")); err != nil {
		t.Errorf("--dist out did not write there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dist")); err == nil {
		t.Error("--dist out must not also write dist/")
	}

	for _, bad := range []string{"../escape", filepath.Join(t.TempDir(), "abs")} {
		_, _, err := runReleaseCmd(t, "--snapshot", "--dir", dir, "--dist", bad, "--version", "0.1.0", "--skip-build")
		if err == nil || !strings.Contains(err.Error(), "--dist") {
			t.Errorf("--dist %q must be refused, got %v", bad, err)
		}
	}
}

// Bare `bashy release` is neither a no-op nor a full release: it must name the
// invocation that exists, and exit 2 (usage) rather than 1 (failed run).
func TestReleaseBareInvocationIsAUsageError(t *testing.T) {
	if code := dispatchRelease([]string{}); code != 2 {
		t.Errorf("bare `bashy release` exit = %d, want 2", code)
	}
	if code := dispatchRelease([]string{"--nope"}); code != 2 {
		t.Errorf("unknown flag exit = %d, want 2", code)
	}
	_, _, err := runReleaseCmd(t)
	if err == nil || !strings.Contains(err.Error(), "--snapshot") {
		t.Errorf("bare invocation must point at --snapshot: %v", err)
	}
}

// The full pipeline with a REAL compile: the archive must hold a binary the
// Go toolchain actually produced, not a file the harness pre-placed.
func TestReleaseSnapshotRealBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a Go program")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go toolchain on PATH")
	}
	cfg := "project_name: demo\nbuilds:\n  - id: demo\n    main: ./cmd/demo\n    binary: demo\n    targets: [" +
		runtime.GOOS + "_" + runtime.GOARCH + "]\narchives:\n  - id: demo\n    formats: [tar.gz]\n" +
		"    name_template: '{{ .ProjectName }}-{{ .Os }}-{{ .Arch }}'\nchecksum:\n  name_template: SHA256SUMS\n"
	dir := newReleaseProject(t, cfg, map[string]string{
		"go.mod":                "module demo\n\ngo 1.24\n",
		"cmd/demo/main.go":      "package main\n\nfunc main() { println(\"demo\") }\n",
		"unused-placeholder.md": "keep the tree non-empty\n",
	})

	stdout, stderr, err := runReleaseCmd(t, "--snapshot", "--dir", dir, "--version", "0.1.0")
	if err != nil {
		t.Fatalf("release --snapshot: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	name := "demo-" + runtime.GOOS + "-" + runtime.GOARCH + ".tar.gz"
	binary := "demo"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	members := tarGzMembers(t, filepath.Join(dir, "dist", name))
	if len(members[binary]) < 100_000 {
		t.Errorf("%s: member %q is %d bytes — that is not a compiled binary (members: %v)",
			name, binary, len(members[binary]), keysOf(members))
	}
	ledger := readLedger(t, filepath.Join(dir, "dist", releaseLedgerName))
	if len(ledger.Artifacts) != 2 {
		t.Errorf("artifacts = %+v, want the archive + SHA256SUMS", ledger.Artifacts)
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// The verb must be discoverable the same way every other front-door verb is:
// catalogued, synopsised, and classified in the atlas.
func TestReleaseIsCataloguedAndClassified(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "1")
	_, _, verbs := commandsCatalog()
	if !containsString(verbs, "release") {
		t.Fatal("`release` is missing from the front-door verb catalog")
	}
	if strings.TrimSpace(verbSynopsis["release"]) == "" {
		t.Error("`release` has no synopsis")
	}
	r := verbAtlasRecord("release", false)
	if r.Group == "" || r.Tier == "" || r.Stage == "" {
		t.Fatalf("`release` has no atlas classification: %+v", r)
	}
	if r.Stage != "deploy" || r.Tier != "workspace" {
		t.Errorf("release = %+v, want deploy/workspace", r)
	}
	// Local-first: the T0 slice reaches no network, so it must not claim the
	// net effect. A publish stage would earn it — and does not exist here.
	if containsString(r.Effects, "net") {
		t.Errorf("release declares the net effect but --snapshot is local-first: %+v", r.Effects)
	}
	if !containsString(r.Effects, "write") || !containsString(r.Effects, "exec") {
		t.Errorf("release must declare write+exec (it writes dist/ and runs the Go toolchain): %+v", r.Effects)
	}
}
