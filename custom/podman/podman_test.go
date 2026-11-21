package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/bashy/custom/internal/adapter"
)

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

func TestPodmanPinsAndDeps(t *testing.T) {
	for _, plat := range []string{"linux/amd64", "linux/arm64", "darwin/arm64", "darwin/amd64", "windows/amd64", "windows/arm64"} {
		a, ok := podmanAssets[plat]
		if !ok {
			t.Fatalf("missing podman pin for %s", plat)
		}
		if !hex64.MatchString(a.sha256) || a.entrypoint == "" {
			t.Errorf("%s: bad pin %+v", plat, a)
		}
	}
	if !podmanStartsMachine([]string{"machine", "start"}) || podmanStartsMachine([]string{"ps"}) {
		t.Fatal("podmanStartsMachine mismatch")
	}
	if got := podmanLinuxPath("/bin"); !strings.Contains(got, "/usr/sbin") {
		t.Fatalf("podmanLinuxPath = %q, want /usr/sbin", got)
	}
	adapter.AssertStandaloneDeps(t, "podman", "github.com/qiangli/yoke/external/podman/winhelper")
	if runtime.GOOS != "windows" {
		dir := t.TempDir()
		fake := filepath.Join(dir, "podman")
		if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir)
		if code := run([]string{"--version"}); code != 7 {
			t.Fatalf("run(fake podman) = %d, want 7", code)
		}
	}
}
