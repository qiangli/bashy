package agentos

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestYcodeYoloFlag(t *testing.T) {
	for _, tc := range []struct {
		args, want []string
		yolo       bool
	}{
		{[]string{"--yolo", "-m", "local", "web"}, []string{"-m", "local", "web"}, true},
		{[]string{"web", "--yolo=true"}, []string{"web"}, true},
		{[]string{"--yolo=false", "status"}, []string{"status"}, false},
		{[]string{"--", "--yolo"}, []string{"--", "--yolo"}, false},
		{[]string{"prompt", "hello", "--yolo"}, []string{"prompt", "hello", "--yolo"}, false},
		{[]string{"shell", "-c", "--yolo"}, []string{"shell", "-c", "--yolo"}, false},
		{[]string{"--session", "--yolo", "status"}, []string{"--session", "--yolo", "status"}, false},
	} {
		got, yolo, err := ycodeYoloFlag(tc.args)
		if err != nil || yolo != tc.yolo || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: args %q, yolo %v, err %v", tc.args, got, yolo, err)
		}
	}
	if _, _, err := ycodeYoloFlag([]string{"--yolo=maybe"}); err == nil {
		t.Fatal("invalid boolean accepted")
	}
}

func TestYcodeYoloSelectsAuthoredProfile(t *testing.T) {
	stubYcodeConfigSelection(t)
	t.Setenv("BASHY_HOME", t.TempDir())
	t.Setenv("YCODE_CONFIG", "")
	t.Setenv("GENIE_HARNESS_CONFIG", "previous")
	t.Chdir(t.TempDir())
	saved := YcodeMain
	t.Cleanup(func() { YcodeMain = saved })
	YcodeMain = func(args []string) int {
		if !reflect.DeepEqual(args, []string{"status"}) {
			t.Fatalf("args = %q", args)
		}
		path := os.Getenv("YCODE_CONFIG")
		data, err := os.ReadFile(path)
		if err != nil || filepath.Base(path) != "agent-yolo.yaml" || !strings.Contains(string(data), "id: yolo-allow-all") {
			t.Fatalf("YOLO config %q: %v", path, err)
		}
		return 0
	}
	if code := dispatchYcode([]string{"--yolo", "status"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if os.Getenv("GENIE_HARNESS_CONFIG") != "previous" {
		t.Fatal("profile selection leaked into the parent environment")
	}
}

func TestYcodeYoloRefusesCustomConfig(t *testing.T) {
	stubYcodeConfigSelection(t)
	t.Setenv("YCODE_CONFIG", "")
	t.Chdir(t.TempDir())
	saved := YcodeMain
	t.Cleanup(func() { YcodeMain = saved })
	YcodeMain = func([]string) int { t.Fatal("conflicting configuration reached engine"); return 0 }
	if code := dispatchYcode([]string{"--yolo", "--file", "custom.yaml", "status"}); code != 2 {
		t.Fatalf("flag conflict exit = %d", code)
	}
	t.Setenv("YCODE_CONFIG", "custom.yaml")
	if code := dispatchYcode([]string{"--yolo", "status"}); code != 2 {
		t.Fatalf("environment conflict exit = %d", code)
	}
	t.Setenv("YCODE_CONFIG", "")
	if err := os.WriteFile("agent.yaml", []byte("custom"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := dispatchYcode([]string{"--yolo", "status"}); code != 2 {
		t.Fatalf("local configuration conflict exit = %d", code)
	}
}

// Exercise the shipped model-profile recipe, rather than only a dispatch
// mock: model substitution must retain the selected policy in the instance.
func TestGenieYoloRecipeProfile(t *testing.T) {
	if _, err := exec.LookPath("bashy"); err != nil {
		t.Skip("bashy unavailable")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go unavailable")
	}
	t.Setenv("BASHY_HOME", t.TempDir())
	source, err := builtinGenieConfig()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(source)
	profile := exec.Command("bashy", "dag", "-f", "dag.md", "profile-model")
	profile.Dir = dir
	profile.Env = append(os.Environ(), "GENIE_PROFILE=yolotest", "GENIE_MODEL_ID=yolo-test-model", "GENIE_HARNESS_CONFIG=agent-yolo.yaml")
	if out, err := profile.CombinedOutput(); err != nil {
		t.Fatalf("profile: %v\n%s", err, out)
	}
	workspace, instance := t.TempDir(), filepath.Join(t.TempDir(), "instance")
	generate := exec.Command("go", "run", "cmd/genie/main.go", "-config", filepath.Join(dir, "dist/profiles/yolotest/agent.yaml"), "-workspace", workspace, "-instance-dir", instance)
	generate.Dir = dir
	generate.Env = append(os.Environ(), "GOWORK=off", "GENIE_APPROVAL=auto")
	out, err := generate.CombinedOutput()
	if err != nil {
		t.Fatalf("instance: %v\n%s", err, out)
	}
	data, err := os.ReadFile(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"id: yolo-allow-all", "permissionCeiling: danger-full-access", "path: prompts/system-yolo.md", "id: \"yolo-test-model\"", "workspace: " + strconv.Quote(workspace)} {
		if !strings.Contains(string(data), want) {
			t.Errorf("generated YOLO profile missing %q", want)
		}
	}
	prompt, err := os.ReadFile(filepath.Join(instance, "prompts", "system-yolo.md"))
	if err != nil || !strings.Contains(string(prompt), "explicit YOLO harness profile") {
		t.Fatalf("generated YOLO prompt: %v", err)
	}
}
