package agentos

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDispatchEmbeddedOllamaRunnerPreservesProtocolArgsAndStatus(t *testing.T) {
	oldEnsure, oldRun := ensureEmbeddedOllamaRunner, runEmbeddedOllamaRunner
	t.Cleanup(func() { ensureEmbeddedOllamaRunner, runEmbeddedOllamaRunner = oldEnsure, oldRun })
	var gotCache, gotPath string
	var gotArgs []string
	ensureEmbeddedOllamaRunner = func(cache string) (string, error) {
		gotCache = cache
		return "/verified/runner", nil
	}
	runEmbeddedOllamaRunner = func(path string, args []string) int {
		gotPath = path
		gotArgs = append([]string(nil), args...)
		return 23
	}

	args := []string{"--model", "a model.gguf", "--port", "43123", "--flag=one two"}
	if got := dispatchEmbeddedOllamaRunner(args); got != 23 {
		t.Fatalf("status = %d, want 23", got)
	}
	if gotCache != filepath.Join(engineCacheDir(), "ollama-runner") {
		t.Fatalf("cache = %q", gotCache)
	}
	if gotPath != "/verified/runner" || !reflect.DeepEqual(gotArgs, args) {
		t.Fatalf("runner invocation = %q %q, want verified path and original args %q", gotPath, gotArgs, args)
	}
}

func TestDispatchEmbeddedOllamaRunnerFailsClosedWhenUnavailable(t *testing.T) {
	oldEnsure, oldRun := ensureEmbeddedOllamaRunner, runEmbeddedOllamaRunner
	t.Cleanup(func() { ensureEmbeddedOllamaRunner, runEmbeddedOllamaRunner = oldEnsure, oldRun })
	ensureEmbeddedOllamaRunner = func(string) (string, error) { return "", errors.New("no signed runner") }
	ran := false
	runEmbeddedOllamaRunner = func(string, []string) int { ran = true; return 0 }

	if got := dispatchEmbeddedOllamaRunner([]string{"--model", "x"}); got == 0 {
		t.Fatal("missing embedded runner returned success")
	}
	if ran {
		t.Fatal("runner was launched after verification failed")
	}
}

func TestRunnerDispatchDoesNotResolvePATH(t *testing.T) {
	// The dispatch entrypoint is an embedded-only protocol route. A fake
	// executable named "runner" on PATH must never be selected.
	dir := t.TempDir()
	fake := filepath.Join(dir, "runner")
	if err := os.WriteFile(fake, []byte("this must not run"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	oldEnsure, oldRun := ensureEmbeddedOllamaRunner, runEmbeddedOllamaRunner
	t.Cleanup(func() { ensureEmbeddedOllamaRunner, runEmbeddedOllamaRunner = oldEnsure, oldRun })
	ensureEmbeddedOllamaRunner = func(string) (string, error) { return "", errors.New("not embedded") }
	ran := false
	runEmbeddedOllamaRunner = func(string, []string) int { ran = true; return 0 }
	if got := dispatchEmbeddedOllamaRunner(nil); got == 0 || ran {
		t.Fatalf("PATH runner fallback occurred: status=%d ran=%v", got, ran)
	}
}
