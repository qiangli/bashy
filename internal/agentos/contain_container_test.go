package agentos

import (
	"reflect"
	"strings"
	"testing"
)

func TestContainBackendChoice(t *testing.T) {
	t.Setenv("BASHY_CONTAIN_BACKEND", "container")
	if got := containBackend(); got != containContainer {
		t.Fatalf("forced container: %q", got)
	}
	t.Setenv("BASHY_CONTAIN_BACKEND", "native")
	if got := containBackend(); got != containNative {
		t.Fatalf("forced native: %q", got)
	}
	t.Setenv("BASHY_CONTAIN_BACKEND", "")
	want := containContainer
	if nativeContainSupported() == nil {
		want = containNative
	}
	if got := containBackend(); got != want {
		t.Fatalf("default: %q, want %q", got, want)
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
