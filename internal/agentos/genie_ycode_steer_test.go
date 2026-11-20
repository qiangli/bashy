package agentos

// Sprint: #676; Story: #3248; Story-ID: 96ff25a316fb
//
// `bashy ycode` is the pre-existing built-in verb (embedded ycode engine),
// so the `bashy <tool>` shortcut (toolshortcut.go) yields to it — but the
// verb opened no steerable bashy chat session. These tests pin the
// follow-up: the interactive (bare) `bashy ycode` joins as a steerable
// session named ycode under the shortcut conventions (yolo default, capped
// inbox, non-TTY refusal, --help shows the steer command).

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/term"
)

// The interactive bare open must refuse without a terminal instead of
// running on: a TUI needs a TTY, and the refusal names the steerable way
// back (`bashy chat --agent ycode`).
func TestDispatchYcodeBareRefusesWithoutTTY(t *testing.T) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		t.Skip("needs non-TTY stdin: the refusal only triggers off-terminal")
	}
	stubYcodeConfigSelection(t)
	_ = noGenieSideEffects(t)
	t.Setenv("YCODE_CONFIG", "")
	t.Chdir(t.TempDir())
	saved := YcodeMain
	t.Cleanup(func() { YcodeMain = saved })
	called := false
	YcodeMain = func(args []string) int { called = true; return 0 }
	var code int
	errText := captureStderr(t, func() { code = dispatchYcode(nil) })
	if called {
		t.Error("bare bashy ycode reached the raw engine; want the steerable chat path")
	}
	if code == 0 {
		t.Error("bare bashy ycode without a TTY exited 0, want refusal")
	}
	for _, want := range []string{"not a TTY", "bashy chat --agent ycode"} {
		if !strings.Contains(errText, want) {
			t.Errorf("refusal lacks %q:\nstderr=%s", want, errText)
		}
	}
}

// Only the bare open (plus -m/--model) joins the steerable session: a
// message, an entry word, --session, --, or any other flag stays on the raw
// path the steer template cannot carry.
func TestYcodeBareOpenArgs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		model string
		bare  bool
	}{
		{"empty", nil, "", true},
		{"model-short", []string{"-m", "qwen3:0.6b"}, "qwen3:0.6b", true},
		{"model-long", []string{"--model", "qwen3:0.6b"}, "qwen3:0.6b", true},
		{"model-equals", []string{"--model=qwen3:0.6b"}, "qwen3:0.6b", true},
		{"model-missing", []string{"-m"}, "", false},
		{"model-empty", []string{"--model="}, "", false},
		{"model-dash-value", []string{"-m", "--yolo"}, "", false},
		{"message", []string{"hello"}, "", false},
		{"entry", []string{"repl"}, "", false},
		{"entry-resume", []string{"resume"}, "", false},
		{"utility", []string{"status"}, "", false},
		{"session", []string{"--session", "s455"}, "", false},
		{"session-equals", []string{"--session=s455"}, "", false},
		{"literal", []string{"--", "--help"}, "", false},
		{"other-flag", []string{"--json"}, "", false},
		{"model-then-message", []string{"-m", "qwen3:0.6b", "hello"}, "", false},
	} {
		model, bare := ycodeBareOpenArgs(tc.args)
		if bare != tc.bare || model != tc.model {
			t.Errorf("%s: ycodeBareOpenArgs(%q) = (%q, %v), want (%q, %v)",
				tc.name, tc.args, model, bare, tc.model, tc.bare)
		}
	}
}

// A bare open naming a model delegates too (session id ycode-<model> by the
// shortcut convention); without a terminal it refuses the same way.
func TestDispatchYcodeBareModelRefusesWithoutTTY(t *testing.T) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		t.Skip("needs non-TTY stdin: the refusal only triggers off-terminal")
	}
	stubYcodeConfigSelection(t)
	_ = noGenieSideEffects(t)
	t.Setenv("YCODE_CONFIG", "")
	t.Chdir(t.TempDir())
	saved := YcodeMain
	t.Cleanup(func() { YcodeMain = saved })
	called := false
	YcodeMain = func(args []string) int { called = true; return 0 }
	var code int
	errText := captureStderr(t, func() { code = dispatchYcode([]string{"-m", "qwen3:0.6b"}) })
	if called {
		t.Error("bashy ycode -m reached the raw engine; want the steerable chat path")
	}
	if code == 0 {
		t.Error("bashy ycode -m without a TTY exited 0, want refusal")
	}
	if !strings.Contains(errText, "not a TTY") {
		t.Errorf("refusal lacks %q:\nstderr=%s", "not a TTY", errText)
	}
}

// The session's own child runs the raw engine: the steer-child marker skips
// delegation so the launch cannot recurse (outer delegates to chat, chat
// spawns `bashy ycode -m {model}`, the child runs raw).
func TestDispatchYcodeSteerChildStaysRaw(t *testing.T) {
	stubYcodeConfigSelection(t)
	_ = noGenieSideEffects(t)
	t.Setenv("YCODE_CONFIG", "")
	t.Setenv(ycodeSteerChildEnv, "1")
	t.Chdir(t.TempDir())
	saved := YcodeMain
	t.Cleanup(func() { YcodeMain = saved })
	called := false
	YcodeMain = func(args []string) int { called = true; return 0 }
	var code int
	errText := captureStderr(t, func() { code = dispatchYcode(nil) })
	if called {
		t.Error("steered child reached the engine without a bundle; the raw path must fail first in tests")
	}
	if code == 0 {
		t.Error("steered child bare open exited 0, want the raw recipe path")
	}
	if strings.Contains(errText, "not a TTY") {
		t.Errorf("steered child delegated to chat; the marker must keep it raw:\nstderr=%s", errText)
	}
}

// Non-bare invocations never delegate: messages stay headless turns, entries
// keep their own behavior, and --pull keeps the recipe's download gating.
func TestDispatchYcodeNonBareStaysRaw(t *testing.T) {
	stubYcodeConfigSelection(t)
	_ = noGenieSideEffects(t)
	t.Setenv("YCODE_CONFIG", "")
	t.Chdir(t.TempDir())
	saved := YcodeMain
	t.Cleanup(func() { YcodeMain = saved })
	YcodeMain = func(args []string) int { return 0 }
	for _, args := range [][]string{
		{"hello"},
		{"repl"},
		{"resume"},
		{"session", "list"},
		{"--pull"},
		{"--session", "s455"},
	} {
		var code int
		errText := captureStderr(t, func() { code = dispatchYcode(args) })
		if code == 0 {
			t.Errorf("bashy ycode %q exited 0, want the raw recipe path (stub build fails)", args)
		}
		if strings.Contains(errText, "not a TTY") {
			t.Errorf("bashy ycode %q delegated to chat; it must stay raw:\nstderr=%s", args, errText)
		}
	}
}

// An explicit --yolo on the bare open delegates the same way (the shortcut
// default is on; the flag only makes it explicit). Off-terminal it refuses
// before touching the engine.
func TestDispatchYcodeBareYoloRefusesWithoutTTY(t *testing.T) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		t.Skip("needs non-TTY stdin: the refusal only triggers off-terminal")
	}
	stubYcodeConfigSelection(t)
	_ = noGenieSideEffects(t)
	t.Setenv("YCODE_CONFIG", "")
	t.Chdir(t.TempDir())
	saved := YcodeMain
	t.Cleanup(func() { YcodeMain = saved })
	called := false
	YcodeMain = func(args []string) int { called = true; return 0 }
	var code int
	errText := captureStderr(t, func() { code = dispatchYcode([]string{"--yolo"}) })
	if called {
		t.Error("bashy ycode --yolo reached the raw engine; want the steerable chat path")
	}
	if code == 0 {
		t.Error("bashy ycode --yolo without a TTY exited 0, want refusal")
	}
	if !strings.Contains(errText, "not a TTY") {
		t.Errorf("refusal lacks %q:\nstderr=%s", "not a TTY", errText)
	}
}

// The yolo decision rides the delegation to the steered child: with the
// marker set the raw path runs the YOLO profile by default (the shortcut's
// yolo default), and BASHY_YCODE_STEER_YOLO=0 — the --no-yolo opt-out — runs
// the attended profile. The fixed steer template has no --yolo slot, so
// without this channel the TUI would silently prompt.
func TestDispatchYcodeSteerChildYoloProfile(t *testing.T) {
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
	body := "#!/bin/sh\nprintf '%s\\n' \"$GENIE_HARNESS_CONFIG\" > \"$CAPTURE_PROFILE\"\n"
	if err := os.WriteFile(self, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASHY_SELF", self)
	t.Setenv("GENIE_BAR", bar)
	t.Setenv("GENIE_MODEL_ID", "")
	t.Setenv("GENIE_EXTERNAL_MODEL", "")
	t.Setenv("YCODE_CONFIG", "")
	t.Setenv("GENIE_MODE", "chat")
	t.Chdir(t.TempDir())
	saved := YcodeMain
	t.Cleanup(func() { YcodeMain = saved })
	YcodeMain = func([]string) int { t.Error("steered child skipped the recipe"); return 1 }
	for _, tc := range []struct{ name, steerYolo, want string }{
		{"default-on", "", "agent-yolo.yaml"},
		{"explicit-on", "1", "agent-yolo.yaml"},
		{"opt-out", "0", "agent.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(ycodeSteerChildEnv, "1")
			if tc.steerYolo == "" {
				os.Unsetenv(ycodeSteerYoloEnv)
			} else {
				t.Setenv(ycodeSteerYoloEnv, tc.steerYolo)
			}
			capture := filepath.Join(root, tc.name+".profile")
			t.Setenv("CAPTURE_PROFILE", capture)
			if code := dispatchYcode(nil); code != 0 {
				t.Fatalf("steered child bare open: exit %d, want the raw recipe to run", code)
			}
			data, err := os.ReadFile(capture)
			if err != nil || strings.TrimSpace(string(data)) != tc.want {
				t.Fatalf("steered child profile = %q, err %v, want %s", data, err, tc.want)
			}
		})
	}
}

// --help shows the steer command for the session the bare open joins.
func TestYcodeHelpShowsSteerCommand(t *testing.T) {
	stubYcodeConfigSelection(t)
	_ = noGenieSideEffects(t)
	t.Setenv("YCODE_CONFIG", "")
	t.Chdir(t.TempDir())
	saved := YcodeMain
	t.Cleanup(func() { YcodeMain = saved })
	YcodeMain = func(args []string) int { return 0 }
	var code int
	out := captureStdout(t, func() { code = dispatchYcode([]string{"--help"}) })
	if code != 0 {
		t.Fatalf("bashy ycode --help: exit %d, want 0", code)
	}
	for _, want := range []string{"bashy chat steer ycode", "bashy chat attach ycode"} {
		if !strings.Contains(out, want) {
			t.Errorf("--help lacks %q:\n%s", want, out)
		}
	}
}
