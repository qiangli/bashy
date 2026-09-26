package agentos

// Sprint: #290; Story: #973; Story-ID: 40fb076220e5

import (
	"strings"
	"testing"
)

func TestPreloadStepsForVariants(t *testing.T) {
	if got, err := preloadSteps(nil); err != nil || got != "" {
		t.Fatalf("no --with: %q, %v", got, err)
	}
	got, err := preloadSteps([]string{"go", " Python ", "python"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"ENV BASHY_BIN_CACHE=/opt/bashy/bin UV_PYTHON_INSTALL_DIR=/opt/bashy/python",
		`RUN ["/bashy", "go", "version"]`,
		`RUN ["/bashy", "uv", "python", "install", "3.`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("steps missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "RUN ") != 2 {
		t.Errorf("duplicates must collapse:\n%s", got)
	}
	// Provisioned toolchains live outside /tmp: a --read-only --tmpfs /tmp
	// run must still see them.
	if strings.Contains(got, "/tmp") {
		t.Errorf("preloaded toolchains must not live under /tmp:\n%s", got)
	}
	if _, err := preloadSteps([]string{"rust"}); err == nil || !strings.Contains(err.Error(), "supported: python, go") {
		t.Fatalf("unknown toolchain: %v", err)
	}
	if n := normalizedWith([]string{"python", "go"}); strings.Join(n, "-") != "go-python" {
		t.Fatalf("tag suffix order = %v", n)
	}
}
