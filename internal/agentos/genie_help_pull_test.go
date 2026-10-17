package agentos

// Sprint: #455; Story: #2308; Story-ID: 140dc1006a0e
//
// E1 F8: `bashy ycode prompt --help` ran a turn — it built the genie bundle
// under the real home, auto-picked a 17 GB model and pulled it until the
// disk was full. Two contracts, both measured here: --help/-h on every
// ycode/genie entry prints help and exits 0 touching nothing; and a model
// the recipe finds missing is pulled only with room on the disk, and in a
// non-interactive session only with explicit consent (--pull).

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// noGenieSideEffects makes every path past the help check fail loudly: the
// bundle build is a stub that errors, and the self binary the recipe would
// run does not exist (runSelf returns 127).
func noGenieSideEffects(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	t.Setenv("GENIE_BAR", "")
	t.Setenv("GENIE_MODEL_ID", "")
	t.Setenv("GENIE_EXTERNAL_MODEL", "")
	t.Setenv("BASHY_SELF", filepath.Join(home, "no-such-bashy"))
	saved := buildBuiltinGenie
	t.Cleanup(func() { buildBuiltinGenie = saved })
	buildBuiltinGenie = func(string, io.Writer) error { return errors.New("stub: no build in tests") }
	return home
}

func TestGenieHelpOnEverySubcommandHasNoSideEffects(t *testing.T) {
	home := noGenieSideEffects(t)
	unattendedOff(t)
	for _, args := range [][]string{
		{"--help"}, {"-h"}, {"help"},
		{"prompt", "--help"}, {"prompt", "-h"},
		{"solve", "--help"}, {"smoke", "-h"},
		{"web", "--help"}, {"resume", "-h"}, {"repl", "--help"}, {"acp", "--help"},
		{"session", "--help"}, {"session", "list", "--help"},
		{"build", "--help"}, {"doctor", "-h"}, {"pull", "--help"},
		{"-m", "qwen3:0.6b", "prompt", "--help"}, {"--yolo", "prompt", "--help"},
		{"--effort", "low", "-h"}, {"prompt", "-m", "qwen3:0.6b", "--help"},
	} {
		var code int
		out := captureStdout(t, func() { code = dispatchGenie(args) })
		if code != 0 {
			t.Errorf("bashy genie %q: exit %d, want 0", args, code)
		}
		if !strings.Contains(out, "usage: bashy genie") {
			t.Errorf("bashy genie %q printed no usage: %q", args, out)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "genie")); !os.IsNotExist(err) {
		t.Fatalf("help touched the genie home: %v", err)
	}
	// A literal message that happens to be --help is still a message: it
	// reaches the recipe, which needs the bundle (the stub build fails).
	if code := dispatchGenie([]string{"--", "--help"}); code != 2 {
		t.Fatalf("literal -- --help: exit %d, want 2 (the recipe path)", code)
	}
}

func TestYcodeHelpOnEverySubcommandHasNoSideEffects(t *testing.T) {
	stubYcodeConfigSelection(t)
	home := noGenieSideEffects(t)
	t.Setenv("YCODE_CONFIG", "")
	t.Chdir(t.TempDir())
	saved := YcodeMain
	t.Cleanup(func() { YcodeMain = saved })
	var seen []string
	var seenConfig string
	YcodeMain = func(args []string) int {
		seen = append([]string(nil), args...)
		seenConfig = os.Getenv("YCODE_CONFIG")
		return 0
	}
	for _, tc := range []struct{ args, want []string }{
		{[]string{"--help"}, []string{"--help"}},
		{[]string{"-h"}, []string{"-h"}},
		{[]string{"prompt", "--help"}, []string{"prompt", "--help"}},
		{[]string{"prompt", "-h"}, []string{"prompt", "-h"}},
		{[]string{"web", "--help"}, []string{"web", "--help"}},
		{[]string{"repl", "--help"}, []string{"repl", "--help"}},
		{[]string{"resume", "-h"}, []string{"resume", "-h"}},
		{[]string{"acp", "--help"}, []string{"acp", "--help"}},
		{[]string{"session", "--help"}, []string{"session", "--help"}},
		{[]string{"session", "list", "--help"}, []string{"session", "list", "--help"}},
		{[]string{"status", "--help"}, []string{"status", "--help"}},
		{[]string{"docs", "-h"}, []string{"docs", "-h"}},
		// bashy's own flags are taken off before ycode sees the words.
		{[]string{"-m", "qwen3:0.6b", "prompt", "--help"}, []string{"prompt", "--help"}},
		{[]string{"--yolo", "prompt", "--help"}, []string{"prompt", "--help"}},
		{[]string{"--effort", "low", "prompt", "-h"}, []string{"prompt", "-h"}},
		{[]string{"--pull", "prompt", "--help"}, []string{"prompt", "--help"}},
		{[]string{"--session", "s455", "prompt", "--help"}, []string{"--session", "s455", "prompt", "--help"}},
	} {
		seen, seenConfig = nil, "unset-by-test"
		var code int
		captureStdout(t, func() { code = dispatchYcode(tc.args) })
		if code != 0 {
			t.Errorf("bashy ycode %q: exit %d, want 0", tc.args, code)
		}
		if strings.Join(seen, "|") != strings.Join(tc.want, "|") {
			t.Errorf("bashy ycode %q: ycode saw %q, want %q", tc.args, seen, tc.want)
		}
		if seenConfig != "" {
			t.Errorf("bashy ycode %q: help selected a config (%q); it must not materialize the builtin genie", tc.args, seenConfig)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "genie")); !os.IsNotExist(err) {
		t.Fatalf("help touched the genie home: %v", err)
	}
	// A literal message stays a message: the recipe path (its self binary
	// is missing here), never ycode's help.
	seen = nil
	if code := dispatchYcode([]string{"--", "--help"}); code == 0 || seen != nil {
		t.Fatalf("literal -- --help: exit %d, ycode args %q; want the recipe path", code, seen)
	}
}

func TestGeniePullPolicy(t *testing.T) {
	for _, tc := range []struct {
		explicit, consent, interactive bool
		want                           string
	}{
		{false, false, false, geniePullDeny},
		{false, true, false, geniePullAllow},
		{true, false, false, geniePullAllow},
		{false, false, true, geniePullAllow},
		{true, true, true, geniePullAllow},
	} {
		if got := geniePullPolicy(tc.explicit, tc.consent, tc.interactive); got != tc.want {
			t.Errorf("explicit=%t consent=%t interactive=%t: %q, want %q", tc.explicit, tc.consent, tc.interactive, got, tc.want)
		}
	}
}

func TestGenieGuardOllamaPullWithFakeDisk(t *testing.T) {
	savedFree, savedSize := geniePullFreeBytes, genieModelSizeGB
	t.Cleanup(func() { geniePullFreeBytes, genieModelSizeGB = savedFree, savedSize })
	const gb = 1e9
	free := uint64(30 * gb)
	geniePullFreeBytes = func(string) (uint64, bool) { return free, true }
	genieModelSizeGB = func(model string) (float64, bool) {
		if model == "big:27b" {
			return 17, true
		}
		return 0, false
	}
	t.Setenv("OLLAMA_MODELS", t.TempDir())
	run := func(policy string, args ...string) (int, string) {
		t.Helper()
		t.Setenv(geniePullPolicyEnv, policy)
		var out strings.Builder
		code := genieGuardOllamaPull(args, &out)
		return code, out.String()
	}
	// Not a genie session: bashy ollama is the user's own call, untouched.
	if code, out := run("", "pull", "big:27b"); code != 0 || out != "" {
		t.Fatalf("no policy: exit %d, %q", code, out)
	}
	// Room on the disk (30 free, 17 + margin needed): the pull proceeds.
	if code, out := run(geniePullAllow, "pull", "big:27b"); code != 0 {
		t.Fatalf("allow with room: exit %d, %q", code, out)
	}
	// The margin counts: 21 free is not 17 + 5.
	free = 21 * gb
	code, out := run(geniePullAllow, "pull", "big:27b")
	if code != 2 || !strings.Contains(out, "big:27b") || !strings.Contains(out, "17") || !strings.Contains(out, "21") {
		t.Fatalf("allow without room: exit %d, %q", code, out)
	}
	free = 10 * gb
	if code, _ := run(geniePullAllow, "pull", "big:27b"); code != 2 {
		t.Fatalf("allow on a nearly full disk: exit %d", code)
	}
	// A model the table does not size cannot be checked for room: it is
	// pulled when the policy allows it.
	free = 10 * gb
	if code, out := run(geniePullAllow, "pull", "unknown:1b"); code != 0 {
		t.Fatalf("allow, unsized model: exit %d, %q", code, out)
	}
	// Non-interactive, auto-picked, no consent: never a pull, whatever the disk.
	free = 500 * gb
	code, out = run(geniePullDeny, "pull", "big:27b")
	if code != 2 || !strings.Contains(out, "--pull") || !strings.Contains(out, "big:27b") || !strings.Contains(out, "-m") {
		t.Fatalf("deny: exit %d, %q", code, out)
	}
	// Only a pull is guarded: the recipe's show/list/run go through.
	for _, args := range [][]string{{"show", "big:27b"}, {"list"}, {"run", "big:27b", "hi"}, {}} {
		if code, out := run(geniePullDeny, args...); code != 0 || out != "" {
			t.Fatalf("%q under deny: exit %d, %q", args, code, out)
		}
	}
	// A disk that cannot be measured (Windows) skips the room check only.
	free = 10 * gb
	geniePullFreeBytes = func(string) (uint64, bool) { return 0, false }
	if code, _ := run(geniePullAllow, "pull", "big:27b"); code != 0 {
		t.Fatalf("allow, unmeasurable disk: exit %d", code)
	}
	if code, _ := run(geniePullDeny, "pull", "big:27b"); code != 2 {
		t.Fatalf("deny, unmeasurable disk: exit %d", code)
	}
}

func TestGenieModelSizeFromTheEmbeddedTable(t *testing.T) {
	size, ok := genieModelSizeGB("qwen3.6:27b")
	if !ok || size < 10 {
		t.Fatalf("qwen3.6:27b: %v GB, ok %t; the embedded models.json sizes it at 17", size, ok)
	}
	if _, ok := genieModelSizeGB("no-such-model:0b"); ok {
		t.Fatal("an unknown model must report no size")
	}
}

// The front doors decide the policy (test stdin is never a terminal: a
// non-interactive session) and the recipe receives it in the environment.
func TestGenieFrontDoorsExportThePullPolicy(t *testing.T) {
	stubYcodeConfigSelection(t)
	if runtime.GOOS == "windows" {
		t.Skip("the child-process capture uses /bin/sh")
	}
	root := t.TempDir()
	bar := filepath.Join(root, "genie.bar")
	if err := os.WriteFile(bar, []byte("stub"), 0o600); err != nil {
		t.Fatal(err)
	}
	self := filepath.Join(root, "bashy-self")
	if err := os.WriteFile(self, []byte("#!/bin/sh\nprintf '%s\\n' \"$GENIE_PULL_POLICY\" > \"$CAPTURE_POLICY\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASHY_HOME", root)
	t.Setenv("BASHY_SELF", self)
	t.Setenv("GENIE_BAR", bar)
	t.Setenv("GENIE_EXTERNAL_MODEL", "")
	t.Setenv("YCODE_CONFIG", "")
	t.Chdir(t.TempDir())
	saved := YcodeMain
	t.Cleanup(func() { YcodeMain = saved })
	YcodeMain = func([]string) int { t.Fatal("the recipe entry reached ycode directly"); return 1 }
	for _, tc := range []struct {
		name, want, modelEnv string
		ycode                bool
		args                 []string
	}{
		{"ycode-prompt", geniePullDeny, "", true, []string{"prompt", "hello"}},
		{"ycode-prompt-consent", geniePullAllow, "", true, []string{"--pull", "prompt", "hello"}},
		{"ycode-prompt-consent-after", geniePullAllow, "", true, []string{"prompt", "--pull", "hello"}},
		{"ycode-explicit-model", geniePullAllow, "", true, []string{"-m", "s455-local-model", "prompt", "hello"}},
		{"ycode-bare-web", geniePullDeny, "", true, []string{"web"}},
		{"genie-message", geniePullDeny, "", false, []string{"hello"}},
		{"genie-consent", geniePullAllow, "", false, []string{"--pull", "hello"}},
		{"genie-explicit-model", geniePullAllow, "", false, []string{"-m", "s455-local-model", "hello"}},
		{"genie-env-model", geniePullAllow, "s455-local-model", false, []string{"hello"}},
		{"genie-solve", geniePullDeny, "", false, []string{"solve", "task"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := filepath.Join(root, tc.name+".policy")
			t.Setenv("CAPTURE_POLICY", capture)
			t.Setenv("GENIE_MODEL_ID", tc.modelEnv)
			t.Setenv(geniePullPolicyEnv, "stale-from-a-parent")
			var code int
			if tc.ycode {
				code = dispatchYcode(tc.args)
			} else {
				code = dispatchGenie(tc.args)
			}
			if code != 0 {
				t.Fatalf("exit %d", code)
			}
			data, err := os.ReadFile(capture)
			if err != nil || strings.TrimSpace(string(data)) != tc.want {
				t.Fatalf("recipe saw policy %q (err %v), want %q", strings.TrimSpace(string(data)), err, tc.want)
			}
		})
	}
}
