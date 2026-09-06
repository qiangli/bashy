package harnessrunner

import (
	"path/filepath"
	"reflect"
	"testing"
)

func compileRequest(t *testing.T, script string) Request {
	t.Helper()
	return Request{
		SchemaVersion: RequestSchemaVersion,
		RequestID:     "req-1",
		Binding: Binding{
			RunID: "run-1", NodeID: "node-1", Attempt: 1, StateRevision: 2,
			ConfigDigest: "sha256:config", LifecycleGeneration: 3, IdempotencyKey: "once-1",
		},
		Command:   Command{Script: script, Cwd: t.TempDir()},
		Limits:    Limits{WallTimeMs: 5000, StdoutBytes: 1024, StderrBytes: 1024},
		Placement: Placement{ID: "local", Generation: 1},
	}
}

func TestCompileIntentIsDeterministicAndBindsEnvironmentWithoutValue(t *testing.T) {
	req := compileRequest(t, "printf '%s' fixed > output.txt")
	req.Command.Environment = []EnvironmentVariable{
		{Name: "ZED", ValueRef: "config://zed", Value: "hidden"},
		{Name: "ALPHA", Value: "visible"},
	}
	first, err := CompileIntent(req)
	if err != nil {
		t.Fatal(err)
	}
	req.Command.Environment[0], req.Command.Environment[1] = req.Command.Environment[1], req.Command.Environment[0]
	second, err := CompileIntent(req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest || !first.Complete {
		t.Fatalf("non-deterministic intent: %#v %#v", first, second)
	}
	if first.Environment[0].Name != "ALPHA" || first.Environment[1].Name != "ZED" || first.Environment[1].ValueDigest == "" {
		t.Fatalf("environment facts = %#v", first.Environment)
	}
	wantTarget := filepath.Join(first.Cwd, "output.txt")
	var kinds []string
	foundTarget := false
	for _, effect := range first.Effects {
		kinds = append(kinds, effect.Kind)
		foundTarget = foundTarget || effect.Target == wantTarget
	}
	if !foundTarget {
		t.Fatalf("redirection target missing from effects: %#v", first.Effects)
	}
	if !contains(kinds, "write") || !contains(kinds, "destroy") {
		t.Fatalf("redirection effects = %v", kinds)
	}
}

func TestCompileIntentFailsClosedForDynamicAndUnknownCommands(t *testing.T) {
	for _, script := range []string{`"$COMMAND" arg`, "definitely-not-a-bashy-command arg", `/bin/echo unsafe`, `rm target`} {
		req := compileRequest(t, script)
		intent, err := CompileIntent(req)
		if err != nil {
			t.Fatalf("%q: %v", script, err)
		}
		if intent.Complete || len(intent.Unsupported) == 0 {
			t.Fatalf("%q unexpectedly complete: %#v", script, intent)
		}
	}
}

func TestCompileIntentRejectsInvalidEnvelope(t *testing.T) {
	req := compileRequest(t, "true")
	req.Command.Argv = []string{"true"}
	if _, err := CompileIntent(req); err == nil {
		t.Fatal("script plus argv was accepted")
	}
	req = compileRequest(t, "true")
	req.Command.Environment = []EnvironmentVariable{{Name: "BAD-NAME", Value: "x"}}
	if _, err := CompileIntent(req); err == nil {
		t.Fatal("invalid environment name was accepted")
	}
}

func TestEnvironmentOrderDoesNotChangeFacts(t *testing.T) {
	a, err := compileEnvironment([]EnvironmentVariable{{Name: "B", Value: "2"}, {Name: "A", Value: "1"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := compileEnvironment([]EnvironmentVariable{{Name: "A", Value: "1"}, {Name: "B", Value: "2"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("facts differ: %#v %#v", a, b)
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
