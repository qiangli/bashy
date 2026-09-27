package agentos

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestContainProviderResolution(t *testing.T) {
	t.Setenv("BASHY_CONTAIN_PROVIDER", "")
	builtin := containImageP
	if nativeContainSupported() == nil {
		builtin = containNative
	}
	for _, tc := range []struct{ requested, env, want string }{
		{"", "", builtin},
		{"builtin", "", builtin},
		{"image", "", containImageP},
		{"native", "", containNative},
		{"custom", "", containCustom},
		{"", "image", containImageP},       // host default
		{"native", "image", containNative}, // the decorator wins
	} {
		t.Setenv("BASHY_CONTAIN_PROVIDER", tc.env)
		if got := resolveContainProvider(tc.requested); got != tc.want {
			t.Errorf("resolve(%q, env %q) = %q, want %q", tc.requested, tc.env, got, tc.want)
		}
	}
	t.Setenv("BASHY_CONTAIN_PROVIDER", "")
	t.Setenv("BASHY_CONTAIN_CUSTOM", "")
	if containSupportedFor("custom") == nil {
		t.Error("custom without BASHY_CONTAIN_CUSTOM must fail closed")
	}
	if validContainProvider("docker") {
		t.Error("unknown provider accepted")
	}
}

func TestContainToolchainsAndImage(t *testing.T) {
	for _, tc := range []struct {
		argv []string
		want []string
	}{
		{[]string{"python3", "-m", "unittest"}, []string{"python"}},
		{[]string{`C:\x\python.exe`, "t.py"}, []string{"python"}},
		{[]string{"/opt/bin/bashy", "python", "-c", "1"}, []string{"python"}},
		{[]string{"go", "test"}, []string{"go"}},
		{[]string{"make"}, nil},
	} {
		if got := containToolchains(tc.argv); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("containToolchains(%q) = %q, want %q", tc.argv, got, tc.want)
		}
	}
	t.Setenv("BASHY_CONTAIN_IMAGE", "")
	img, _ := containImage([]string{"python3"})
	if !strings.HasPrefix(img, "localhost/bashy:") || !strings.HasSuffix(img, "-with-python") {
		t.Errorf("default image %q is not bashy's own python image", img)
	}
	t.Setenv("BASHY_CONTAIN_IMAGE", "localhost/bashy:custom")
	if img, _ := containImage([]string{"python3"}); img != "localhost/bashy:custom" {
		t.Errorf("override: %q", img)
	}
}

func TestContainerCommandAndArgs(t *testing.T) {
	if got := containerCommand([]string{`C:\Tools\python3.exe`, "-c", "print('a b')"}); got != `'python3' '-c' 'print('\''a b'\'')'` {
		t.Errorf("host path child: %s", got)
	}
	if got := containerCommand([]string{"/opt/bin/bashy", "python", "-m", "unittest"}); got != `bashy 'python' '-m' 'unittest'` {
		t.Errorf("bashy child: %s", got)
	}
	args := containerRunArgs("localhost/bashy:v", `C:\work`, "true")
	for _, want := range []string{"--network=none", `C:\work:/work`, "-w", "localhost/bashy:v"} {
		found := false
		for _, a := range args {
			if a == want {
				found = true
			}
		}
		if !found {
			t.Errorf("run args lack %q: %q", want, args)
		}
	}
}

// provider custom runs the child under the user's wrapper and keeps its exit
// status; an unknown provider is a usage error.
func TestDispatchContainCustomProvider(t *testing.T) {
	if _, err := exec.LookPath("env"); err != nil {
		t.Skip("no env(1)")
	}
	t.Setenv("BASHY_CONTAIN_CUSTOM", "env")
	if code := dispatchContain([]string{"--net", "deny", "--provider", "custom", "--", "true"}); code != 0 {
		t.Fatalf("custom true: exit %d", code)
	}
	if code := dispatchContain([]string{"--net", "deny", "--provider=custom", "--", "false"}); code != 1 {
		t.Fatalf("custom false: exit %d, want 1", code)
	}
	if code := dispatchContain([]string{"--net", "deny", "--provider", "docker", "--", "true"}); code != 2 {
		t.Fatalf("unknown provider: exit %d, want 2", code)
	}
	t.Setenv("BASHY_CONTAIN_CUSTOM", "")
	if code := dispatchContain([]string{"--net", "deny", "--provider", "custom", "--", "true"}); code != containUnsupportedStatus {
		t.Fatalf("custom without a wrapper: exit %d, want %d", code, containUnsupportedStatus)
	}
}
