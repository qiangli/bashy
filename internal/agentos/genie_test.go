package agentos

// Sprint: #290; Story: #933; Story-ID: 6a0507862385

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	coreacp "github.com/qiangli/yoke/pkg/acp"
)

func writeGenieSource(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dag := "# genie — Bashy workflow\n\n## Tasks\n\n### solve\nEffects: read\n\n```bsh\n:\n```\n"
	if err := os.WriteFile(filepath.Join(dir, "dag.md"), []byte(dag), 0o644); err != nil {
		t.Fatal(err)
	}
}

func stubYcodeConfigSelection(t *testing.T) {
	t.Helper()
	saved := YcodeHasExplicitConfig
	YcodeHasExplicitConfig = func(args []string) bool {
		if os.Getenv("YCODE_CONFIG") != "" {
			return true
		}
		if _, err := os.Stat("agent.yaml"); err == nil {
			return true
		}
		return len(args) > 0 && (args[0] == "-f" || args[0] == "--file" || args[0] == "--config")
	}
	t.Cleanup(func() { YcodeHasExplicitConfig = saved })
}

// Exercise the shipped recipe's profile target and config generator together.
// A cached source filename alone would pass a dispatch mock while granting
// the agent the wrong workspace and model.
func TestGenieRecipeGeneratesCallerWorkspaceAndModel(t *testing.T) {
	if _, err := exec.LookPath("bashy"); err != nil {
		t.Skip("bashy executable unavailable")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("Go executable unavailable")
	}
	root := t.TempDir()
	t.Setenv("BASHY_HOME", root)
	source, err := builtinGenieConfig()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(source)
	model := "s387-real-profile"
	profile := exec.Command("bashy", "dag", "-f", "dag.md", "profile-model")
	profile.Dir = dir
	profile.Env = append(os.Environ(), "GENIE_PROFILE=s387test", "GENIE_MODEL_ID="+model, "GENIE_CONTEXT_TOKENS=32768")
	if out, err := profile.CombinedOutput(); err != nil {
		t.Fatalf("profile-model: %v\n%s", err, out)
	}
	workspace := filepath.Join(t.TempDir(), "caller")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	generate := exec.Command("go", "run", "cmd/genie/main.go", "-config", filepath.Join(dir, "dist/profiles/s387test/agent.yaml"), "-workspace", workspace, "-instance-dir", filepath.Join(root, "instance"))
	generate.Dir = dir
	out, err := generate.CombinedOutput()
	if err != nil {
		t.Fatalf("instance config: %v\n%s", err, out)
	}
	config := strings.TrimSpace(string(out))
	data, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"workspace: \"" + workspace + "\"",
		"readableRoots: [\"" + workspace + "\",",
		"writableRoots: [\"" + workspace + "\"]",
		"id: \"" + model + "\"",
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("generated config %q missing %q", config, want)
		}
	}
	// ACP initialize only checks protocol transport. Create a real client
	// session and verify its repository in the persisted session record.
	ycodeRoot, err := filepath.Abs(filepath.Join("..", "..", "..", "ycode"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(ycodeRoot, "cmd/ycode/main.go")); err != nil {
		t.Skip("ycode checkout unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	acpCommand := exec.Command("go", "run", "./cmd/ycode", "-f", config, "acp")
	acpCommand.Dir = ycodeRoot
	acpCommand.Env = os.Environ()
	client, err := coreacp.NewClient(ctx, coreacp.BaseHandler{}, acpCommand)
	if err != nil {
		t.Fatalf("ACP initialize: %v", err)
	}
	defer client.Close()
	sessionID, err := client.NewSession(ctx, workspace)
	if err != nil {
		t.Fatalf("ACP session/new: %v", err)
	}
	state, err := os.ReadFile(filepath.Join(root, "ycode", "harness", "acp-sessions.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(state), sessionID) || !strings.Contains(string(state), workspace) {
		t.Fatalf("ACP session %q missing caller repository %q: %s", sessionID, workspace, state)
	}
}

func TestGenieBundleResolution(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	t.Setenv("GENIE_BAR", "")
	// The real builtin build runs this binary (`bashy dag`): in a test, the
	// test binary. Stand in for it.
	builds := 0
	saved := buildBuiltinGenie
	t.Cleanup(func() { buildBuiltinGenie = saved })
	buildBuiltinGenie = func(home string, _ io.Writer) error {
		builds++
		digest, err := builtinGenieDigest()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(home), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(home, "genie.bar"), []byte("built"), 0o644); err != nil {
			return err
		}
		return writeGenieProvenance(home, genieProvenance{Source: "builtin", Digest: digest})
	}
	installed := filepath.Join(home, "genie", "genie.bar")
	// No bundle: built from the builtin source on first use.
	if got, err := genieBundle(); err != nil || got != installed || builds != 1 {
		t.Fatalf("missing bundle: got %q, %v, builds %d", got, err, builds)
	}
	// A current builtin bundle is reused.
	if got, err := genieBundle(); err != nil || got != installed || builds != 1 {
		t.Fatalf("current builtin bundle: got %q, %v, builds %d", got, err, builds)
	}
	// A developer's build (recorded as a directory) is kept as is.
	if err := os.WriteFile(installed, []byte("bar"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeGenieProvenance(filepath.Dir(installed), genieProvenance{Source: "dir", Path: "/src/genie"}); err != nil {
		t.Fatal(err)
	}
	if got, err := genieBundle(); err != nil || got != installed || builds != 1 {
		t.Fatalf("developer bundle: got %q, %v, builds %d", got, err, builds)
	}
	// doctor only looks: a missing bundle is reported, never built.
	if err := os.Remove(installed); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveGenieBundle(false); err == nil || builds != 1 {
		t.Fatalf("a look-only resolve built or found a bundle: %v, builds %d", err, builds)
	}
	explicit := filepath.Join(t.TempDir(), "other.bar")
	if err := os.WriteFile(explicit, []byte("bar"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GENIE_BAR", explicit)
	if got, err := genieBundle(); err != nil || got != explicit {
		t.Fatalf("GENIE_BAR wins: got %q, %v", got, err)
	}
	t.Setenv("GENIE_BAR", filepath.Join(t.TempDir(), "missing.bar"))
	if _, err := genieBundle(); err == nil {
		t.Fatal("a missing GENIE_BAR must be an error, not a fallback")
	}
}

func TestGenieSourceDiscovery(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "ycode", "examples", "genie")
	writeGenieSource(t, source)
	nested := filepath.Join(root, "some", "project")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	t.Setenv("GENIE_SOURCE", "")
	got, err := genieSource("")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(source)
	if resolved, _ := filepath.EvalSymlinks(got); resolved != want {
		t.Fatalf("found %q, want %q", got, source)
	}
	if _, err := genieSource(nested); err == nil {
		t.Fatal("--from a directory without a genie dag.md must be refused")
	}
	if got, err := genieSource(source); err != nil || got != source {
		t.Fatalf("--from: got %q, %v", got, err)
	}
}

func TestDispatchGenieUsageAndPull(t *testing.T) {
	t.Setenv("BASHY_HOME", t.TempDir())
	t.Setenv("GENIE_BAR", "")
	// No arguments is the interactive session; without a bundle genie builds
	// the builtin one first. The real build runs this binary (the test
	// binary here), so a stand-in fails it: nothing is launched.
	saved := buildBuiltinGenie
	t.Cleanup(func() { buildBuiltinGenie = saved })
	buildBuiltinGenie = func(string, io.Writer) error { return errors.New("stub: no build in tests") }
	if code := dispatchGenie(nil); code != 2 {
		t.Fatalf("no arguments and no bundle: exit %d, want 2", code)
	}
	if code := dispatchGenie([]string{"--help"}); code != 0 {
		t.Fatalf("--help: exit %d, want 0", code)
	}
	for _, args := range [][]string{{"web", "hello"}, {"resume", "x"}, {"-m"}, {"--bogus"}} {
		if code := dispatchGenie(args); code != 2 {
			t.Fatalf("%q: exit %d, want 2", args, code)
		}
	}
	if code := dispatchGenie([]string{"pull"}); code != 2 {
		t.Fatalf("pull before a release exists: exit %d, want 2", code)
	}
}

func TestGenieModelFlag(t *testing.T) {
	for _, c := range []struct {
		args  []string
		model string
		rest  []string
	}{
		{nil, "", nil},
		{[]string{"-m", "qwen3:8b", "fix", "the", "bug"}, "qwen3:8b", []string{"fix", "the", "bug"}},
		{[]string{"--model=gemma4:e4b", "web"}, "gemma4:e4b", []string{"web"}},
		{[]string{"--model", "x"}, "x", nil},
		{[]string{"explain", "-m", "flag"}, "", []string{"explain", "-m", "flag"}},
		{[]string{"--", "-v is a flag?"}, "", []string{"-v is a flag?"}},
		{[]string{"solve", "-m", "x", "task"}, "", []string{"solve", "-m", "x", "task"}},
	} {
		model, rest, err := genieModelFlag(c.args)
		if err != nil || model != c.model || strings.Join(rest, "|") != strings.Join(c.rest, "|") {
			t.Errorf("%q: model %q rest %q err %v", c.args, model, rest, err)
		}
	}
}

func TestYcodeBuiltinConfigAndCustomConfig(t *testing.T) {
	stubYcodeConfigSelection(t)
	t.Setenv("BASHY_HOME", t.TempDir())
	t.Setenv("YCODE_CONFIG", "")
	t.Chdir(t.TempDir())
	saved := YcodeMain
	t.Cleanup(func() { YcodeMain = saved })
	var seenArgs []string
	var seenConfig string
	YcodeMain = func(args []string) int {
		seenArgs = append([]string(nil), args...)
		seenConfig = os.Getenv("YCODE_CONFIG")
		return 0
	}
	if code := dispatchYcode([]string{"status"}); code != 0 {
		t.Fatalf("builtin status exit %d", code)
	}
	data, err := os.ReadFile(seenConfig)
	if err != nil || !strings.Contains(string(data), "name: genie") {
		t.Fatalf("builtin config %q: %v", seenConfig, err)
	}
	if len(seenArgs) != 1 || seenArgs[0] != "status" {
		t.Fatalf("args changed: %q", seenArgs)
	}
	custom := filepath.Join(t.TempDir(), "custom.yaml")
	if err := os.WriteFile(custom, []byte("custom"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YCODE_CONFIG", custom)
	if code := dispatchYcode([]string{"status"}); code != 0 || seenConfig != custom {
		t.Fatalf("environment config replaced: exit %d, config %q", code, seenConfig)
	}
	t.Setenv("YCODE_CONFIG", "")
	if code := dispatchYcode([]string{"-f", custom, "status"}); code != 0 || seenConfig != "" {
		t.Fatalf("flag config replaced: exit %d, config %q", code, seenConfig)
	}
	if err := os.WriteFile("agent.yaml", []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := dispatchYcode([]string{"status"}); code != 0 || seenConfig != "" {
		t.Fatalf("local config replaced: exit %d, config %q", code, seenConfig)
	}
}

func TestYcodeDocsValuesDoNotSelectConfig(t *testing.T) {
	stubYcodeConfigSelection(t)
	t.Setenv("BASHY_HOME", t.TempDir())
	t.Setenv("YCODE_CONFIG", "")
	t.Chdir(t.TempDir())
	saved := YcodeMain
	t.Cleanup(func() { YcodeMain = saved })
	var selected string
	YcodeMain = func([]string) int { selected = os.Getenv("YCODE_CONFIG"); return 0 }
	for _, args := range [][]string{{"docs", "--search", "-filter"}, {"docs", "--search", "--file=topic"}} {
		t.Setenv("YCODE_CONFIG", "")
		if code := dispatchYcode(args); code != 0 {
			t.Fatalf("%q: exit %d", args, code)
		}
		if data, err := os.ReadFile(selected); err != nil || !strings.Contains(string(data), "name: genie") {
			t.Fatalf("%q selected %q as a config: %v", args, selected, err)
		}
	}
}

func TestYcodeHumanEntryUsesGenieRecipe(t *testing.T) {
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
	if err := os.WriteFile(self, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CAPTURE_ARGS\"\nprintf '%s\\n' \"$GENIE_MODE\" > \"$CAPTURE_MODE\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASHY_SELF", self)
	t.Setenv("GENIE_BAR", bar)
	t.Setenv("GENIE_EXTERNAL_MODEL", "")
	t.Setenv("YCODE_CONFIG", "")
	t.Chdir(t.TempDir())
	saved := YcodeMain
	t.Cleanup(func() { YcodeMain = saved })
	YcodeMain = func([]string) int { t.Fatal("human entry skipped the recipe"); return 1 }
	for _, tc := range []struct {
		name, mode, model string
		args              []string
	}{
		{"bare", "chat", "", nil},
		{"model", "chat", "s387-local-model", []string{"-m", "s387-local-model"}},
		{"web", "web", "", []string{"web"}},
		{"web-leading-model", "web", "s387-local-model", []string{"-m", "s387-local-model", "web"}},
		{"web-trailing-model", "web", "s387-local-model", []string{"web", "-m", "s387-local-model"}},
		{"repl", "repl", "", []string{"repl"}},
		{"resume", "resume", "", []string{"resume"}},
		{"prompt", "prompt", "", []string{"prompt", "hello"}},
		{"prompt-flag-text", "prompt", "", []string{"prompt", "--", "-filter"}},
		{"text", "chat", "", []string{"--", "hello"}},
		{"flag-text", "chat", "", []string{"--", "-filter"}},
		{"acp", "acp", "s387-local-model", []string{"-m", "s387-local-model", "acp"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := filepath.Join(root, tc.name+".args")
			modeCapture := filepath.Join(root, tc.name+".mode")
			t.Setenv("CAPTURE_ARGS", capture)
			t.Setenv("CAPTURE_MODE", modeCapture)
			ambient := "web"
			if tc.name == "model" || tc.name == "prompt" || tc.name == "acp" || tc.mode == "web" {
				ambient = "session"
			}
			t.Setenv("GENIE_MODE", ambient) // a hostile parent session
			t.Setenv("GENIE_MODEL_ID", "")
			if code := dispatchYcode(tc.args); code != 0 {
				t.Fatalf("exit %d", code)
			}
			data, err := os.ReadFile(capture)
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(got) < 4 || got[0] != "run" || got[1] != "--target" || got[2] != "chat" || got[3] != bar {
				t.Fatalf("recipe args: %q", got)
			}
			modeData, err := os.ReadFile(modeCapture)
			if err != nil || strings.TrimSpace(string(modeData)) != tc.mode {
				t.Fatalf("child mode = %q, err %v; want %q", modeData, err, tc.mode)
			}
			if tc.model != "" && os.Getenv("GENIE_MODEL_ID") != tc.model {
				t.Fatalf("model not passed to recipe: %q", os.Getenv("GENIE_MODEL_ID"))
			}
		})
	}
}
