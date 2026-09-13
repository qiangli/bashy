package harnessrunner

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/qiangli/coreutils/pkg/atlas"
	"github.com/qiangli/coreutils/pkg/kb"
)

func compileRequest(t *testing.T, script string) Request {
	t.Helper()
	t.Setenv("BASHY_KB_DIR", filepath.Join(t.TempDir(), "host-kb"))
	t.Setenv("BASHY_HOME", filepath.Join(t.TempDir(), "bashy-home"))
	t.Setenv("BASHY_SKILLS_DIR", filepath.Join(t.TempDir(), "skills"))
	t.Setenv("YCODE_DATA_DIR", filepath.Join(t.TempDir(), "ycode-data"))
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

func compileRequestInCwd(t *testing.T, script, cwd string) Request {
	t.Helper()
	req := compileRequest(t, script)
	req.Command.Cwd = cwd
	return req
}

func makeRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
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

func TestPureBuiltinDynamicArgumentsAreComplete(t *testing.T) {
	for _, script := range []string{`printf %s "$YCODE_IN_X"`, `echo "${VAR}"`} {
		req := compileRequest(t, script)
		intent, err := CompileIntent(req)
		if err != nil {
			t.Fatalf("%q: %v", script, err)
		}
		if !intent.Complete || len(intent.Unsupported) != 0 {
			t.Fatalf("%q incomplete: %#v", script, intent)
		}
		if len(intent.Commands) != 1 {
			t.Fatalf("%q command facts = %#v", script, intent.Commands)
		}
		if !contains(intent.Commands[0].Argv, "<dynamic>") {
			t.Fatalf("%q dynamic arg not recorded in argv: %#v", script, intent.Commands[0])
		}
		if intent.ScriptDigest != digestBytes([]byte(script)) {
			t.Fatalf("%q script digest = %q", script, intent.ScriptDigest)
		}
	}
}

func TestDynamicCommandAndDynamicRedirectionStillFailClosed(t *testing.T) {
	tests := map[string]string{
		`"$CMD" x`:     "dynamicCommand",
		`echo hi > $F`: "dynamicRedirection",
	}
	for script, kind := range tests {
		intent := mustCompileScript(t, script)
		if intent.Complete {
			t.Fatalf("%q unexpectedly complete: %#v", script, intent)
		}
		assertUnsupportedKind(t, intent, kind)
	}
}

func TestCompileIntentFailsClosedForDynamicAndUnknownCommands(t *testing.T) {
	for _, script := range []string{`"$COMMAND" arg`, `read "$X"`, `pwd "$X"`, "definitely-not-a-bashy-command arg", `/bin/echo unsafe`, `rm target`} {
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

func TestKBReadOnlySubverbsRefineToExactRead(t *testing.T) {
	repo := makeRepo(t)
	req := compileRequestInCwd(t, `bashy kb context --for x --rings repo,host --budget 700 --json`, repo)
	intent, err := CompileIntent(req)
	if err != nil {
		t.Fatal(err)
	}
	if !intent.Complete || len(intent.Unsupported) != 0 {
		t.Fatalf("kb context incomplete: %#v", intent)
	}
	assertSingleEffect(t, intent, atlas.EffRead, filepath.Join(repo, kb.RepoSub))
}

func TestKBReadOnlySubverbsAllowDynamicValues(t *testing.T) {
	repo := makeRepo(t)
	req := compileRequestInCwd(t, `bashy kb context --for "$TASK" --rings repo,host --budget 700 --json`, repo)
	intent, err := CompileIntent(req)
	if err != nil {
		t.Fatal(err)
	}
	if !intent.Complete || len(intent.Unsupported) != 0 {
		t.Fatalf("dynamic kb context incomplete: %#v", intent)
	}
	assertSingleEffect(t, intent, atlas.EffRead, filepath.Join(repo, kb.RepoSub))
}

func TestKBWriteSubverbsRefineStaticRingAndRejectDynamicRing(t *testing.T) {
	repo := makeRepo(t)
	req := compileRequestInCwd(t, `bashy kb note add --candidate --ring agent --title t --body b`, repo)
	agentKB := filepath.Join(os.Getenv("YCODE_DATA_DIR"), "kb")
	intent, err := CompileIntent(req)
	if err != nil {
		t.Fatal(err)
	}
	if !intent.Complete || len(intent.Unsupported) != 0 {
		t.Fatalf("kb note incomplete: %#v", intent)
	}
	assertEffect(t, intent, atlas.EffRead, agentKB)
	assertEffect(t, intent, atlas.EffWrite, agentKB)
	if len(intent.Effects) != 2 {
		t.Fatalf("write effects = %#v", intent.Effects)
	}

	req = compileRequestInCwd(t, `bashy kb note add --ring "$R" --title t --body b`, repo)
	intent, err = CompileIntent(req)
	if err != nil {
		t.Fatal(err)
	}
	if intent.Complete || len(intent.Unsupported) == 0 {
		t.Fatalf("dynamic write ring unexpectedly complete: %#v", intent)
	}
	assertUnsupportedKind(t, intent, "dynamicCommand")
}

func TestKBUnknownSubverbAndGraphStayIncomplete(t *testing.T) {
	repo := makeRepo(t)
	for _, script := range []string{`bashy kb frobnicate`, `bashy graph impact x`} {
		req := compileRequestInCwd(t, script, repo)
		intent, err := CompileIntent(req)
		if err != nil {
			t.Fatalf("%q: %v", script, err)
		}
		if intent.Complete || len(intent.Unsupported) == 0 {
			t.Fatalf("%q unexpectedly complete: %#v", script, intent)
		}
		assertUnsupportedKind(t, intent, "effectRefinement")
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

func mustCompileScript(t *testing.T, script string) Intent {
	t.Helper()
	req := compileRequest(t, script)
	intent, err := CompileIntent(req)
	if err != nil {
		t.Fatalf("%q: %v", script, err)
	}
	return intent
}

func assertUnsupportedKind(t *testing.T, intent Intent, kind string) {
	t.Helper()
	for _, fact := range intent.Unsupported {
		if fact.Kind == kind {
			return
		}
	}
	t.Fatalf("missing unsupported kind %q in %#v", kind, intent.Unsupported)
}

func assertSingleEffect(t *testing.T, intent Intent, kind, target string) {
	t.Helper()
	if len(intent.Effects) != 1 {
		t.Fatalf("effects = %#v", intent.Effects)
	}
	assertEffect(t, intent, kind, target)
}

func assertEffect(t *testing.T, intent Intent, kind, target string) {
	t.Helper()
	target = canonicalTarget(target)
	for _, effect := range intent.Effects {
		if effect.Kind == kind && effect.Target == target && effect.Certainty == "exact" {
			return
		}
	}
	t.Fatalf("missing %s effect on %s in %#v", kind, target, intent.Effects)
}
