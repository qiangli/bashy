// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !bashy_engines || (windows && (!remote || !containers_image_openpgp))

package agentos

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Every pinned asset must carry a real sha256 and a tree entrypoint — the
// fail-closed download path refuses anything else, and a typo here would only
// surface on a fresh host.
func TestPodmanPinsComplete(t *testing.T) {
	for plat, a := range podmanAssets {
		if !hex64.MatchString(a.sha256) {
			t.Errorf("%s: sha256 %q is not 64 hex", plat, a.sha256)
		}
		if a.entrypoint == "" || !strings.HasPrefix(a.url, "https://github.com/") {
			t.Errorf("%s: incomplete pin %+v", plat, a)
		}
		if strings.HasPrefix(plat, "windows/") != strings.HasSuffix(a.entrypoint, ".exe") {
			t.Errorf("%s: entrypoint %q has the wrong suffix", plat, a.entrypoint)
		}
	}
	for name, a := range darwinHelperAssets {
		if !hex64.MatchString(a.sha256) || a.entrypoint != "" {
			t.Errorf("darwin helper %s: bad pin %+v", name, a)
		}
	}
	for _, plat := range []string{"linux/amd64", "linux/arm64", "darwin/arm64", "darwin/amd64", "windows/amd64", "windows/arm64"} {
		if _, ok := podmanAssets[plat]; !ok {
			t.Errorf("no podman pin for %s", plat)
		}
	}
}

func TestProvisionPodmanRepairsMissingDarwinHelpersOnCacheHit(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin helper layout")
	}

	cache := t.TempDir()
	t.Setenv("BASHY_BIN_CACHE", cache)
	t.Setenv("CONTAINERS_CONF_OVERRIDE", "")
	writeExecutable := func(p string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("stub"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	podmanAsset := podmanAssets["darwin/"+runtime.GOARCH]
	podman := filepath.Join(cache, "podman", podmanAsset.version, filepath.FromSlash(podmanAsset.entrypoint))
	writeExecutable(podman)
	for name, asset := range darwinHelperAssets {
		writeExecutable(filepath.Join(cache, name, asset.version, name))
	}

	if got := provisionPodman(context.Background()); got != podman {
		t.Fatalf("provisionPodman() = %q, want cached %q", got, podman)
	}
	applyManagedPodmanEnv(podman)
	for name := range darwinHelperAssets {
		if !isExecutable(filepath.Join(cache, podmanHelperDirName, name)) {
			t.Errorf("missing repaired %s helper", name)
		}
	}
	conf := filepath.Join(cache, podmanHelperDirName, podmanConfOverrideName)
	if got := os.Getenv("CONTAINERS_CONF_OVERRIDE"); got != conf {
		t.Fatalf("CONTAINERS_CONF_OVERRIDE = %q, want %q", got, conf)
	}
	if _, err := os.Stat(conf); err != nil {
		t.Fatalf("repaired helper config: %v", err)
	}
}

func TestManagedPodmanRootLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux layout")
	}
	a := podmanAssets["linux/"+runtime.GOARCH]
	root := filepath.Join("/cache", "podman", a.version, strings.SplitN(a.entrypoint, "/", 2)[0])
	bin := filepath.Join("/cache", "podman", a.version, filepath.FromSlash(a.entrypoint))
	if got := managedPodmanRoot(bin); got != root {
		t.Fatalf("managedPodmanRoot(%q) = %q, want %q", bin, got, root)
	}
	if managedPodmanRoot("/usr/bin/podman") != "" {
		t.Fatal("a host podman must not be treated as managed")
	}
}

func TestApparmorUsernsHint(t *testing.T) {
	if apparmorUsernsHint(0, "1") != "" || apparmorUsernsHint(1000, "0") != "" || apparmorUsernsHint(1000, "") != "" {
		t.Fatal("hint must only fire for a non-root user on a restricting host")
	}
	if h := apparmorUsernsHint(1000, "1"); !strings.Contains(h, "apparmor_restrict_unprivileged_userns=0") {
		t.Fatalf("hint should name the sysctl: %q", h)
	}
}

func TestPodmanConfOverrideLinuxPaths(t *testing.T) {
	conf := podmanConfOverrideLinux("/c/podman/v6/podman-linux-amd64")
	for _, want := range []string{
		`conmon_path = ["/c/podman/v6/podman-linux-amd64/usr/local/lib/podman/conmon"]`,
		`crun = ["/c/podman/v6/podman-linux-amd64/usr/local/bin/crun"]`,
		`network_backend = "netavark"`,
		`cgroup_manager = "cgroupfs"`,
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("override missing %s\n%s", want, conf)
		}
	}
}

func TestPodmanLinuxPathAddsSystemSbinWithoutReordering(t *testing.T) {
	got := strings.Split(podmanLinuxPath("/candidate/bin:/usr/bin:/bin:/usr/sbin"), ":")
	want := []string{"/candidate/bin", "/usr/bin", "/bin", "/usr/sbin", "/usr/local/sbin", "/sbin"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("podmanLinuxPath() = %q, want %q", got, want)
	}
}
