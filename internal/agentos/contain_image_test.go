package agentos

// Sprint: #290; Story: #1042; Story-ID: 530c3e2e68ac

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const testDigestRef = "docker.io/swebench/sweb.eval.x86_64.django_1776_django-11740@sha256:cd95eb83c40d5738a534d6210a485345fffc9cf653c2c5995b4395d202ff0eb4"

func TestContainImageSpecValidate(t *testing.T) {
	t.Setenv("BASHY_CONTAIN_CUSTOM", "")
	ok := func(s containImageSpec) *containImageSpec {
		if s.Image == "" {
			s.Image = testDigestRef
		}
		if s.Argv == nil {
			s.Argv = []string{"-c", "true"}
		}
		return &s
	}
	for _, tc := range []struct {
		name string
		spec *containImageSpec
		want string // "" = valid
	}{
		{"digest", ok(containImageSpec{}), ""},
		{"tag refused", ok(containImageSpec{Image: "docker.io/swebench/x:latest"}), "not pinned by digest"},
		{"short digest refused", ok(containImageSpec{Image: "alpine@sha256:abc"}), "not pinned by digest"},
		{"custom needs wrapper", ok(containImageSpec{Provider: "custom"}), "BASHY_CONTAIN_CUSTOM"},
		{"native runs no image", ok(containImageSpec{Provider: "native"}), "does not run images"},
		{"relative workdir", ok(containImageSpec{Workdir: "testbed"}), "absolute"},
		{"ro ok", ok(containImageSpec{RO: []string{"/opt/py:/opt/py"}}), ""},
		{"ro relative host", ok(containImageSpec{RO: []string{"py:/opt/py"}}), "HOST:CTR"},
		{"ro with mode", ok(containImageSpec{RO: []string{"/a:/b:rw"}}), "HOST:CTR"},
		{"ro over bashy", ok(containImageSpec{RO: []string{"/tmp/x:/.bashy/bashy"}}), "reserved"},
		{"ro over out", ok(containImageSpec{RO: []string{"/tmp/x:/out"}}), "reserved"},
		{"env value refused", ok(containImageSpec{Env: []string{"A=1"}}), "NAMES"},
		{"no command", &containImageSpec{Image: testDigestRef}, "no command"},
	} {
		err := tc.spec.validate()
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: unexpected error %v", tc.name, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%s: error %v, want it to mention %q", tc.name, err, tc.want)
		}
	}
}

// The image call is a decorator only: `bashy contain` keeps exactly its
// --net deny surface.
func TestContainCommandSurfaceUnchanged(t *testing.T) {
	for _, args := range [][]string{
		{"--image", testDigestRef, "--", "true"},
		{"--workdir", "/testbed", "--net", "deny", "--", "true"},
		{"--net", "door", "--", "true"},
		{"pin", "alpine:3"},
		{"--init", "--", "true"},
	} {
		if code := dispatchContain(args); code != 2 {
			t.Errorf("dispatchContain(%q) = %d, want 2 (usage)", args, code)
		}
	}
}

func TestContainImageRefusesBeforeRunning(t *testing.T) {
	ran := false
	var stderr strings.Builder
	code := runContainedImage(&containImageSpec{Image: "alpine:3", Argv: []string{"-c", "true"}}, &stderr, func([]string) int { ran = true; return 0 })
	if code != 2 || ran || !strings.Contains(stderr.String(), "not pinned by digest") {
		t.Errorf("a tag: code %d ran %v stderr %q", code, ran, stderr.String())
	}
}

func TestContainRunArgs(t *testing.T) {
	t.Setenv("CONTAIN_TEST_PASS", "yes")
	s := &containImageSpec{Image: testDigestRef, Workdir: "/testbed", Out: "/tmp/out",
		Env: []string{"CONTAIN_TEST_PASS", "CONTAIN_TEST_UNSET"}, RO: []string{"/opt/tc:/opt/tc"}, Argv: []string{"-c", "echo hi"}}
	line := strings.Join(containRunArgs(s, "bashy-contain-t", "/usr/local/bin/bashy"), " ")
	for _, want := range []string{
		"podman run --rm -i --name bashy-contain-t --network=none",
		"--entrypoint /.bashy/bashy",
		"-v /usr/local/bin/bashy:/.bashy/bashy:ro",
		"-v /tmp/out:/out",
		"-v /opt/tc:/opt/tc:ro",
		"-w /testbed",
		"-e CONTAIN_TEST_PASS=yes",
		testDigestRef + " -c echo hi",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("run args lack %q:\n%s", want, line)
		}
	}
	if strings.Contains(line, "CONTAIN_TEST_UNSET") {
		t.Error("an unset env: name must not be passed")
	}
	// The caller's environment, not the process's, decides.
	s.Environ = map[string]string{"CONTAIN_TEST_UNSET": "caller"}
	line = strings.Join(containRunArgs(s, "n", "/b"), " ")
	if !strings.Contains(line, "-e CONTAIN_TEST_UNSET=caller") || strings.Contains(line, "CONTAIN_TEST_PASS") {
		t.Errorf("env: must come from the caller's environment:\n%s", line)
	}
	if (&containImageSpec{Environ: map[string]string{"BASHY_CONTAIN_CUSTOM": "w"}}).getenv("BASHY_CONTAIN_CUSTOM") != "w" {
		t.Error("provider custom must see the caller's BASHY_CONTAIN_CUSTOM")
	}
}

func TestContainScriptFunctions(t *testing.T) {
	dump := strings.Join([]string{
		"agent () ",
		"{ ",
		"    command '/Users/x/.local/bin/bashy.real' agent \"$@\"",
		"}",
		"helper () ",
		"{ ",
		"    echo \"helper $1\"",
		"}",
		"@effects(\"read,write,exec\")",
		"@contain(image: \"x@sha256:00\")",
		"arm () ",
		"{ ",
		"    helper \"$1\";",
		"    python3 -c 'print(1)'",
		"}",
		"@contain(net: \"deny\")",
		"tests () ",
		"{ ",
		"    python -m pytest",
		"}",
		"",
	}, "\n")
	got := containScriptFunctions(dump, "arm")
	for _, want := range []string{"helper () ", "echo \"helper $1\"", "arm () ", "python3 -c 'print(1)'", "@contain(net: \"deny\")\ntests () "} {
		if !strings.Contains(got, want) {
			t.Errorf("script lacks %q:\n%s", want, got)
		}
	}
	for _, bad := range []string{"bashy.real", "agent () ", "@contain(image:", "@effects"} {
		if strings.Contains(got, bad) {
			t.Errorf("script keeps %q:\n%s", bad, got)
		}
	}
}

func TestContainInjectedBashy(t *testing.T) {
	notELF := filepath.Join(t.TempDir(), "bashy")
	if err := os.WriteFile(notELF, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := containInjectedBashy(notELF, "amd64"); err == nil {
		t.Error("a non-ELF injected bashy must be refused")
	}
	if runtime.GOOS != "linux" {
		if _, err := containInjectedBashy("", "amd64"); err == nil || !strings.Contains(err.Error(), "BASHY_CONTAIN_BASHY") {
			t.Errorf("a non-linux host without BASHY_CONTAIN_BASHY: %v", err)
		}
	}
}
