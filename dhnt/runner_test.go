package dhnt

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	runnerInputBytes  = "trusted input\n"
	runnerOutputBytes = "verified output\n"
)

func TestRunnerHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_DHNT_RUNNER_HELPER") != "1" {
		return
	}
	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	want := []string{"argument with spaces", "$literal", ";not-shell"}
	if separator < 0 || !equalStrings(os.Args[separator+1:], want) {
		os.Exit(97)
	}
	if name := os.Getenv("CHECK_ABSENT_PATH"); name != "" {
		if _, err := os.Lstat(name); !os.IsNotExist(err) {
			os.Exit(98)
		}
	}
	output := os.Getenv("DHNT_OUTPUT_RESULT_PATH")
	switch os.Getenv("HELPER_MODE") {
	case "success":
		if err := os.WriteFile(output, []byte(runnerOutputBytes), 0o600); err != nil {
			os.Exit(91)
		}
	case "success-count":
		countPath := os.Getenv("HELPER_COUNT_PATH")
		count := byte('0')
		if data, err := os.ReadFile(countPath); err == nil && len(data) == 1 {
			count = data[0]
		}
		if err := os.WriteFile(countPath, []byte{count + 1}, 0o600); err != nil {
			os.Exit(93)
		}
		if err := os.WriteFile(output, []byte(runnerOutputBytes), 0o600); err != nil {
			os.Exit(91)
		}
	case "fail-once":
		countPath := os.Getenv("HELPER_COUNT_PATH")
		if _, err := os.Stat(countPath); os.IsNotExist(err) {
			if err := os.WriteFile(countPath, []byte("1"), 0o600); err != nil {
				os.Exit(93)
			}
			os.Exit(7)
		}
		if err := os.WriteFile(output, []byte(runnerOutputBytes), 0o600); err != nil {
			os.Exit(91)
		}
	case "wrong":
		if err := os.WriteFile(output, []byte("wrong"), 0o600); err != nil {
			os.Exit(91)
		}
	case "partial":
		if err := os.WriteFile(output, []byte("verified"), 0o600); err != nil {
			os.Exit(91)
		}
	case "missing":
	case "failure":
		os.Exit(7)
	case "sleep":
		time.Sleep(30 * time.Second)
	case "signal":
		process, err := os.FindProcess(os.Getpid())
		if err != nil || process.Signal(os.Interrupt) != nil {
			os.Exit(94)
		}
		time.Sleep(30 * time.Second)
	case "symlink-output":
		if err := os.Symlink(os.Getenv("DHNT_INPUT_SOURCE_PATH"), output); err != nil {
			os.Exit(91)
		}
	case "inject-commit":
		if err := os.WriteFile(output, []byte(runnerOutputBytes), 0o600); err != nil {
			os.Exit(91)
		}
		if err := os.WriteFile(os.Getenv("HELPER_COMMIT_PATH"), []byte("forged"), 0o600); err != nil {
			os.Exit(92)
		}
	case "mutate-input":
		if err := os.WriteFile(os.Getenv("DHNT_INPUT_SOURCE_PATH"), []byte("mutated"), 0o600); err != nil {
			os.Exit(95)
		}
		if err := os.WriteFile(output, []byte(runnerOutputBytes), 0o600); err != nil {
			os.Exit(91)
		}
	case "delete-input":
		if err := os.Remove(os.Getenv("DHNT_INPUT_SOURCE_PATH")); err != nil {
			os.Exit(95)
		}
		if err := os.WriteFile(output, []byte(runnerOutputBytes), 0o600); err != nil {
			os.Exit(91)
		}
	case "replace-input":
		input := os.Getenv("DHNT_INPUT_SOURCE_PATH")
		replacement := input + ".replacement"
		if err := os.WriteFile(replacement, []byte(runnerInputBytes), 0o600); err != nil {
			os.Exit(95)
		}
		if err := os.Remove(input); err != nil {
			os.Exit(95)
		}
		if err := os.Symlink(replacement, input); err != nil {
			os.Exit(95)
		}
		if err := os.WriteFile(output, []byte(runnerOutputBytes), 0o600); err != nil {
			os.Exit(91)
		}
	default:
		os.Exit(96)
	}
	os.Exit(0)
}

func TestExecuteTaskSuccessCommitLastAndExactArgv(t *testing.T) {
	workspace, spec, argv := runnerFixture(t, "success")
	spec.Environment = append(spec.Environment, Environment{
		Name: "CHECK_ABSENT_PATH", Value: filepath.Join(workspace, filepath.FromSlash(spec.CommitManifestPath)),
	})
	run, err := ExecuteTask(context.Background(), workspace, spec, argv, runnerMetadata())
	if err != nil {
		t.Fatal(err)
	}
	if run.Result.Class != ResultPass || run.OutputCommit == nil {
		t.Fatalf("unexpected run: %+v", run)
	}
	assertRunnerEvidence(t, workspace, spec, ResultPass, true)
	if got, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(spec.Outputs[0].Path))); err != nil ||
		string(got) != runnerOutputBytes {
		t.Fatalf("published output: %q, %v", got, err)
	}
	manifest, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(spec.CommitManifestPath)))
	if err != nil {
		t.Fatal(err)
	}
	want, err := MarshalOutputCommitManifest(runnerArtifacts(spec.Outputs))
	if err != nil {
		t.Fatal(err)
	}
	if !equalBytes(manifest, want) {
		t.Fatal("published manifest differs from canonical commit bytes")
	}
}

func TestExecuteTaskPreexistingIdenticalDestinationsStillReruns(t *testing.T) {
	workspace, spec, argv := runnerFixture(t, "success-count")
	countPath := filepath.Join(workspace, "command-count")
	spec.Environment = append(spec.Environment, Environment{Name: "HELPER_COUNT_PATH", Value: countPath})
	first, err := ExecuteTask(context.Background(), workspace, spec, argv, runnerMetadata())
	if err != nil || first.Result.Class != ResultPass {
		t.Fatalf("first run: %+v, %v", first, err)
	}
	metadata := runnerMetadata()
	metadata.PodUID = "11234567-89ab-cdef-0123-456789abcdef"
	second, err := ExecuteTask(context.Background(), workspace, spec, argv, metadata)
	if err != nil || second.Result.Class != ResultPass {
		t.Fatalf("second run: %+v, %v", second, err)
	}
	count, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(count) != "2" {
		t.Fatalf("preexisting commit was treated as a cache hit; command count %q", count)
	}
}

func TestExecuteTaskRetriesPreserveEveryAttemptEvidence(t *testing.T) {
	workspace, spec, argv := runnerFixture(t, "fail-once")
	spec.NonzeroClass = ResultTestFail
	spec.Environment = append(spec.Environment, Environment{
		Name: "HELPER_COUNT_PATH", Value: filepath.Join(workspace, "attempt-count"),
	})
	firstMetadata := runnerMetadata()
	first, err := ExecuteTask(context.Background(), workspace, spec, argv, firstMetadata)
	if err != nil || first.Result.Class != ResultTestFail {
		t.Fatalf("first attempt: %+v, %v", first, err)
	}
	secondMetadata := runnerMetadata()
	secondMetadata.PodUID = "11234567-89ab-cdef-0123-456789abcdef"
	second, err := ExecuteTask(context.Background(), workspace, spec, argv, secondMetadata)
	if err != nil || second.Result.Class != ResultPass {
		t.Fatalf("second attempt: %+v, %v", second, err)
	}
	firstEvidence := readRunnerEvidence(t, workspace, spec, firstMetadata)
	secondEvidence := readRunnerEvidence(t, workspace, spec, secondMetadata)
	if firstEvidence.Result.Class != ResultTestFail || firstEvidence.OutputCommit != nil {
		t.Fatalf("first evidence was overwritten: %+v", firstEvidence)
	}
	if secondEvidence.Result.Class != ResultPass || secondEvidence.OutputCommit == nil {
		t.Fatalf("second evidence missing: %+v", secondEvidence)
	}
}

func TestExecuteTaskRejectsPreexistingAttemptEvidence(t *testing.T) {
	workspace, spec, argv := runnerFixture(t, "success")
	metadata := runnerMetadata()
	directory := filepath.Join(workspace, filepath.FromSlash(spec.EvidenceDirectory))
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(directory, metadata.PodUID+".json")
	if err := os.WriteFile(destination, []byte("forged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteTask(context.Background(), workspace, spec, argv, metadata); err == nil ||
		!strings.Contains(err.Error(), "already exists") {
		t.Fatalf("got %v", err)
	}
	if got, err := os.ReadFile(destination); err != nil || string(got) != "forged" {
		t.Fatalf("forged destination was rewritten: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(spec.CommitManifestPath))); !os.IsNotExist(err) {
		t.Fatal("preexisting evidence destination allowed a commit")
	}
}

func TestExecuteTaskRejectsTamperedInputWithoutLaunching(t *testing.T) {
	workspace, spec, argv := runnerFixture(t, "success")
	marker := filepath.Join(workspace, "helper-ran")
	spec.Environment = append(spec.Environment, Environment{Name: "HELPER_MARKER", Value: marker})
	if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(spec.Inputs[0].Path)), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	run, err := ExecuteTask(context.Background(), workspace, spec, argv, runnerMetadata())
	if err != nil {
		t.Fatal(err)
	}
	if run.Result.Class != ResultInfraFail || run.OutputCommit != nil {
		t.Fatalf("tampered input produced dishonest evidence: %+v", run)
	}
	if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(spec.CommitManifestPath))); !os.IsNotExist(err) {
		t.Fatal("tampered input published a commit")
	}
}

func TestExecuteTaskWrongMissingPartialAndSymlinkOutputsAreIncomplete(t *testing.T) {
	for _, mode := range []string{"wrong", "missing", "partial", "symlink-output"} {
		t.Run(mode, func(t *testing.T) {
			workspace, spec, argv := runnerFixture(t, mode)
			run, err := ExecuteTask(context.Background(), workspace, spec, argv, runnerMetadata())
			if err != nil {
				t.Fatal(err)
			}
			if run.Result.Class != ResultIncomplete || run.OutputCommit != nil {
				t.Fatalf("unexpected evidence: %+v", run)
			}
			assertRunnerEvidence(t, workspace, spec, ResultIncomplete, false)
			if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(spec.CommitManifestPath))); !os.IsNotExist(err) {
				t.Fatal("invalid output published a commit")
			}
		})
	}
}

func TestExecuteTaskRejectsInputMutationAfterCommand(t *testing.T) {
	for _, mode := range []string{"mutate-input", "delete-input", "replace-input"} {
		t.Run(mode, func(t *testing.T) {
			workspace, spec, argv := runnerFixture(t, mode)
			run, err := ExecuteTask(context.Background(), workspace, spec, argv, runnerMetadata())
			if err != nil {
				t.Fatal(err)
			}
			if run.Result.Class != ResultInfraFail || run.OutputCommit != nil {
				t.Fatalf("input mutation produced dishonest evidence: %+v", run)
			}
			assertRunnerEvidence(t, workspace, spec, ResultInfraFail, false)
			if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(spec.CommitManifestPath))); !os.IsNotExist(err) {
				t.Fatal("mutated input allowed an output commit")
			}
			if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(spec.Outputs[0].Path))); !os.IsNotExist(err) {
				t.Fatal("mutated input allowed output publication")
			}
		})
	}
}

func TestExecuteTaskCommandFailureUsesDeclaredPolicy(t *testing.T) {
	workspace, spec, argv := runnerFixture(t, "failure")
	spec.NonzeroClass = ResultTestFail
	run, err := ExecuteTask(context.Background(), workspace, spec, argv, runnerMetadata())
	if err != nil {
		t.Fatal(err)
	}
	if run.Result.Class != ResultTestFail || run.Result.ExitCode == nil || *run.Result.ExitCode != 7 || run.OutputCommit != nil {
		t.Fatalf("unexpected command failure evidence: %+v", run)
	}
	assertRunnerEvidence(t, workspace, spec, ResultTestFail, false)
}

func TestExecuteTaskCancellationIsCanceled(t *testing.T) {
	workspace, spec, argv := runnerFixture(t, "sleep")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	run, err := ExecuteTask(ctx, workspace, spec, argv, runnerMetadata())
	if err != nil {
		t.Fatal(err)
	}
	if run.Result.Class != ResultCanceled || run.OutputCommit != nil {
		t.Fatalf("unexpected cancellation evidence: %+v", run)
	}
	assertRunnerEvidence(t, workspace, spec, ResultCanceled, false)
}

func TestExecuteTaskRejectsSymlinkInputAndMismatchedDestination(t *testing.T) {
	t.Run("symlink input", func(t *testing.T) {
		workspace, spec, argv := runnerFixture(t, "success")
		input := filepath.Join(workspace, filepath.FromSlash(spec.Inputs[0].Path))
		if err := os.Remove(input); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(workspace, "target")
		if err := os.WriteFile(target, []byte(runnerInputBytes), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, input); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		run, err := ExecuteTask(context.Background(), workspace, spec, argv, runnerMetadata())
		if err != nil {
			t.Fatal(err)
		}
		if run.Result.Class != ResultInfraFail || run.OutputCommit != nil {
			t.Fatalf("unexpected evidence: %+v", run)
		}
	})
	t.Run("mismatched destination", func(t *testing.T) {
		workspace, spec, argv := runnerFixture(t, "success")
		final := filepath.Join(workspace, filepath.FromSlash(spec.Outputs[0].Path))
		if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(final, []byte("mismatch"), 0o600); err != nil {
			t.Fatal(err)
		}
		run, err := ExecuteTask(context.Background(), workspace, spec, argv, runnerMetadata())
		if err != nil {
			t.Fatal(err)
		}
		if run.Result.Class != ResultInfraFail || run.OutputCommit != nil {
			t.Fatalf("unexpected evidence: %+v", run)
		}
	})
}

func TestExecuteTaskRejectsCommandCreatedCommitAndEmitsNoPass(t *testing.T) {
	workspace, spec, argv := runnerFixture(t, "inject-commit")
	spec.Environment = append(spec.Environment, Environment{
		Name: "HELPER_COMMIT_PATH", Value: filepath.Join(workspace, filepath.FromSlash(spec.CommitManifestPath)),
	})
	run, err := ExecuteTask(context.Background(), workspace, spec, argv, runnerMetadata())
	if err != nil {
		t.Fatal(err)
	}
	if run.Result.Class != ResultInfraFail || run.OutputCommit != nil {
		t.Fatalf("command-created commit produced pass evidence: %+v", run)
	}
	if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(spec.CommitManifestPath))); !os.IsNotExist(err) {
		t.Fatal("untrusted command-created commit was not removed")
	}
	assertRunnerEvidence(t, workspace, spec, ResultInfraFail, false)
}

func TestRunnerSpecRejectsAliasesTreesSecretsAndMalformedMetadata(t *testing.T) {
	workspace, spec, argv := runnerFixture(t, "success")
	t.Run("ancestor alias", func(t *testing.T) {
		edited := spec
		edited.EvidenceDirectory = pathParent(spec.Outputs[0].Path)
		if err := edited.Validate(); err == nil || !strings.Contains(err.Error(), "path alias") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("tree fail closed", func(t *testing.T) {
		edited := spec
		edited.Outputs = append([]RunnerArtifact(nil), spec.Outputs...)
		edited.Outputs[0].Kind = ArtifactTree
		edited.Outputs[0].DigestAlgorithm = DigestSHA256TreeV1
		if err := edited.Validate(); err == nil || !strings.Contains(err.Error(), "only file") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("secret-like env", func(t *testing.T) {
		edited := spec
		edited.Environment = append(edited.Environment, Environment{Name: "API_TOKEN", Value: "literal"})
		if err := edited.Validate(); err == nil || !strings.Contains(err.Error(), "secret-bearing") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("malformed node", func(t *testing.T) {
		if _, err := ExecuteTask(context.Background(), workspace, spec, argv, RunnerMetadata{
			Node: "Bad Node", PodUID: runnerMetadata().PodUID,
		}); err == nil {
			t.Fatal("malformed Downward API node was accepted")
		}
	})
	t.Run("malformed pod UID", func(t *testing.T) {
		if _, err := ExecuteTask(context.Background(), workspace, spec, argv, RunnerMetadata{
			Node: runnerMetadata().Node, PodUID: "not-a-uid",
		}); err == nil {
			t.Fatal("malformed Downward API pod UID was accepted")
		}
	})
}

func TestRunnerSpecRequiresCanonicalStrictJSON(t *testing.T) {
	_, spec, _ := runnerFixture(t, "success")
	data, err := MarshalRunnerSpec(spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeRunnerSpec(data); err != nil {
		t.Fatal(err)
	}
	noncanonical := append(append([]byte(nil), data...), ' ')
	if _, err := DecodeRunnerSpec(noncanonical); err == nil || !strings.Contains(err.Error(), "canonical") {
		t.Fatalf("got %v", err)
	}
	unknown := []byte(strings.Replace(string(data), `"schema":`, `"unknown":true,"schema":`, 1))
	if _, err := DecodeRunnerSpec(unknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("got %v", err)
	}
}

func runnerFixture(t *testing.T, mode string) (string, RunnerSpec, []string) {
	t.Helper()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "inputs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "inputs", "source"), []byte(runnerInputBytes), 0o600); err != nil {
		t.Fatal(err)
	}
	inputDigest := sha256.Sum256([]byte(runnerInputBytes))
	outputDigest := sha256.Sum256([]byte(runnerOutputBytes))
	outputHex := hexDigest(outputDigest)
	argv := []string{
		os.Args[0], "-test.run=^TestRunnerHelperProcess$", "--",
		"argument with spaces", "$literal", ";not-shell",
	}
	spec := RunnerSpec{
		Schema:   RunnerSpecSchema,
		Pipeline: "runner-test",
		Task:     "test",
		Source: Source{
			Repository: "https://example.test/project.git",
			Commit:     "abc123",
			SHA256:     strings.Repeat("a", 64),
		},
		Argv:             argv,
		WorkingDirectory: ".",
		Environment: []Environment{
			{Name: "GO_WANT_DHNT_RUNNER_HELPER", Value: "1"},
			{Name: "HELPER_MODE", Value: mode},
		},
		Inputs: []RunnerArtifact{{
			Name: "source", Kind: ArtifactFile, DigestAlgorithm: DigestSHA256FileV1,
			SHA256: hexDigest(inputDigest), Path: "inputs/source",
		}},
		Outputs: []RunnerArtifact{{
			Name: "result", Kind: ArtifactFile, DigestAlgorithm: DigestSHA256FileV1,
			SHA256: outputHex, Path: "blobs/" + outputHex,
		}},
		EvidenceDirectory:  "evidence/test",
		CommitManifestPath: "commits/test.json",
		NonzeroClass:       ResultInfraFail,
		Platform:           Platform{Backend: "k3s", OS: "linux", Arch: "arm64"},
	}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	return workspace, spec, argv
}

func runnerMetadata() RunnerMetadata {
	return RunnerMetadata{
		Node:   "dragon",
		PodUID: "01234567-89ab-cdef-0123-456789abcdef",
	}
}

func assertRunnerEvidence(t *testing.T, workspace string, spec RunnerSpec, class ResultClass, committed bool) {
	t.Helper()
	run := readRunnerEvidence(t, workspace, spec, runnerMetadata())
	if run.Result.Class != class || (run.OutputCommit != nil) != committed {
		t.Fatalf("unexpected persisted evidence: %+v", run)
	}
}

func readRunnerEvidence(t *testing.T, workspace string, spec RunnerSpec, metadata RunnerMetadata) Run {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(spec.EvidenceDirectory), metadata.PodUID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := DecodeRun(data)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func equalStrings(a, b []string) bool {
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

func pathParent(name string) string {
	index := strings.LastIndexByte(name, '/')
	if index < 0 {
		return "."
	}
	return name[:index]
}
