// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !bashy_engines || (windows && (!remote || !containers_image_openpgp))

package agentos

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/qiangli/yoke/pkg/binmgr"
)

// The lean binary provisions podman the way it already provisions ollama and
// the Windows machine helpers: from the UPSTREAM releases, at a pinned version,
// verified against a sha256 committed here (the trust root is this file's
// reviewed history — pins.go in yoke explains why the release's own checksum is
// not enough). Fetched into $BASHY_BIN_CACHE and exec'd as a separate process;
// nothing is linked or bundled (docs/licensing-supply-chain-policy.md).
//
//	linux    mgoltzsche/podman-static — a fully static podman + conmon + crun/runc +
//	         netavark/aardvark-dns + pasta tree (Apache-2.0 project; crun, pasta,
//	         fuse-overlayfs are GPL-2.0 — download+exec, never redistributed by bashy)
//	darwin   containers/podman remote client + gvproxy + vfkit (Apache-2.0)
//	windows  containers/podman remote client; gvproxy.exe + win-sshproxy.exe ship
//	         in the same zip (and winhelper keeps its own pinned copy)
//
// Bumping: download the new asset, take its sha256 yourself, confirm it against
// the vendor's published shasums, update the entry.
const (
	podmanUpstreamVersion  = "v6.1.2" // containers/podman + mgoltzsche/podman-static
	podmanDarwinAmd64Vers  = "v5.8.7" // last upstream release with a darwin_amd64 client
	gvproxyDarwinVersion   = "v0.8.8" // containers/gvisor-tap-vsock (same pin as winhelper)
	vfkitVersion           = "v0.6.4" // crc-org/vfkit — the signed universal binary
	podmanStaticRepo       = "mgoltzsche/podman-static"
	podmanUpstreamRepo     = "containers/podman"
	gvisorTapVsockRepo     = "containers/gvisor-tap-vsock"
	vfkitRepo              = "crc-org/vfkit"
	podmanHelperDirName    = "podman-helpers" // darwin: gvproxy + vfkit land here
	podmanConfOverrideName = "containers.conf"
)

// podmanAsset is one pinned upstream download.
type podmanAsset struct {
	version, url, sha256, entrypoint string // entrypoint: slash path inside the archive
}

// podmanAssets is the per-platform pin table. An absent platform means "not
// auto-provisionable here" (engineNotFoundMessage).
var podmanAssets = map[string]podmanAsset{
	"linux/amd64": {podmanUpstreamVersion, ghDownload(podmanStaticRepo, podmanUpstreamVersion, "podman-linux-amd64.tar.gz"),
		"481b6f5a57919aea837b54df1eaa6a5b968df505eedbdd78356e80306af676f1", "podman-linux-amd64/usr/local/bin/podman"},
	"linux/arm64": {podmanUpstreamVersion, ghDownload(podmanStaticRepo, podmanUpstreamVersion, "podman-linux-arm64.tar.gz"),
		"0782f7e6934745cb24be81ebaf17fd0d3dc08040edfe0c25afe67a5fe2fcd0cf", "podman-linux-arm64/usr/local/bin/podman"},
	"darwin/arm64": {podmanUpstreamVersion, ghDownload(podmanUpstreamRepo, podmanUpstreamVersion, "podman-remote-release-darwin_arm64.zip"),
		"c19c7add3dfa6d5f42e74f4b07dbecbc14b61fd21e7092ffd3dd1ba6cafbe2a6", "podman-6.1.2/usr/bin/podman"},
	"darwin/amd64": {podmanDarwinAmd64Vers, ghDownload(podmanUpstreamRepo, podmanDarwinAmd64Vers, "podman-remote-release-darwin_amd64.zip"),
		"4d69d9cbea91c65631c0e58b5a32d28014a44b1ef0503d33c571fd1e449c94c9", "podman-5.8.7/usr/bin/podman"},
	"windows/amd64": {podmanUpstreamVersion, ghDownload(podmanUpstreamRepo, podmanUpstreamVersion, "podman-remote-release-windows_amd64.zip"),
		"98c309e1cba4f36fc89a0819607de0696d52f0dcc0c5ef3a8d5cd87fdcf062ba", "podman-6.1.2/usr/bin/podman.exe"},
	"windows/arm64": {podmanUpstreamVersion, ghDownload(podmanUpstreamRepo, podmanUpstreamVersion, "podman-remote-release-windows_arm64.zip"),
		"9c652543765737d22692023e682b1dea0be72bc4dc93d1d3c940718c689360dd", "podman-6.1.2/usr/bin/podman.exe"},
}

// darwinHelperAssets are the two VM helpers `podman machine` execs on macOS.
// Both are universal (x86_64 + arm64) binaries, so one pin serves both arches.
var darwinHelperAssets = map[string]podmanAsset{
	"gvproxy": {gvproxyDarwinVersion, ghDownload(gvisorTapVsockRepo, gvproxyDarwinVersion, "gvproxy-darwin"),
		"614d58dd3f8473258fb6ec18f7631000d2e5c9a761ca52748cc714c07d16382f", ""},
	"vfkit": {vfkitVersion, ghDownload(vfkitRepo, vfkitVersion, "vfkit"),
		"0ed83fc8ca7aa708598835480dba1362406aa7cd1dab3b27464eb76327d9652d", ""},
}

func ghDownload(repo, version, asset string) string {
	return "https://github.com/" + repo + "/releases/download/" + version + "/" + asset
}

// podmanTool is the binmgr Tool for this platform's pinned podman archive.
func podmanTool() (binmgr.Tool, bool) {
	a, ok := podmanAssets[binmgr.Platform()]
	if !ok {
		return binmgr.Tool{}, false
	}
	return binmgr.Tool{
		Name:    "podman",
		Version: a.version,
		Assets: map[string]binmgr.Asset{
			binmgr.Platform(): {URL: a.url, SHA256: a.sha256, Tree: true, Entrypoint: a.entrypoint},
		},
	}, true
}

// cachedPodman returns the pinned podman entrypoint if it is already in the
// cache — no network, no "fetching" line. Mirrors binmgr.Ensure's layout:
// <CacheDir>/podman/<version>/<entrypoint>.
func cachedPodman() string {
	a, ok := podmanAssets[binmgr.Platform()]
	if !ok {
		return ""
	}
	root, err := binmgr.CacheDir()
	if err != nil {
		return ""
	}
	p := filepath.Join(root, "podman", a.version, filepath.FromSlash(a.entrypoint))
	if isExecutable(p) {
		return p
	}
	return ""
}

// provisionPodman fetches the pinned upstream podman for this platform (and, on
// macOS, the two machine helpers) into bashy's cache and returns the podman
// entrypoint. "" means not available here (→ engineNotFoundMessage).
func provisionPodman(ctx context.Context) string {
	if p := cachedPodman(); p != "" {
		return p
	}
	t, ok := podmanTool()
	if !ok {
		return ""
	}
	fmt.Fprintf(os.Stderr, "bashy podman: fetching podman %s for %s — first run only…\n", t.Version, binmgr.Platform())
	path, err := binmgr.Ensure(ctx, t)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy podman: %v\n", err)
		return ""
	}
	if runtime.GOOS == "darwin" {
		if err := provisionDarwinHelpers(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "bashy podman: %v\n", err)
			return ""
		}
	}
	return path
}

// darwinHelperDir is where gvproxy and vfkit live: a flat dir podman is pointed
// at through helper_binaries_dir.
func darwinHelperDir() string {
	root, err := binmgr.CacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(root, podmanHelperDirName)
}

// provisionDarwinHelpers fetches gvproxy + vfkit (pinned) into darwinHelperDir.
// Each is a single raw binary — binmgr.Ensure lands it at
// <cache>/<name>/<version>/<name>; we symlink/copy the result into the flat
// helper dir podman expects.
func provisionDarwinHelpers(ctx context.Context) error {
	dir := darwinHelperDir()
	if dir == "" {
		return fmt.Errorf("no cache dir")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, a := range darwinHelperAssets {
		dest := filepath.Join(dir, name)
		if isExecutable(dest) {
			continue
		}
		t := binmgr.Tool{Name: name, Version: a.version, Assets: map[string]binmgr.Asset{
			binmgr.Platform(): {URL: a.url, SHA256: a.sha256},
		}}
		src, err := binmgr.Ensure(ctx, t)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		_ = os.Remove(dest)
		if err := os.Symlink(src, dest); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

// managedPodmanRoot returns the extracted podman-static tree root (the dir that
// holds usr/ and etc/) for a managed linux podman path, or "" when bin is not a
// managed linux podman.
func managedPodmanRoot(bin string) string {
	a, ok := podmanAssets[binmgr.Platform()]
	if !ok || runtime.GOOS != "linux" {
		return ""
	}
	suffix := filepath.FromSlash(strings.TrimPrefix(a.entrypoint, strings.SplitN(a.entrypoint, "/", 2)[0]))
	if !strings.HasSuffix(bin, suffix) {
		return ""
	}
	return strings.TrimSuffix(bin, suffix)
}

// applyManagedPodmanEnv wires a MANAGED podman (one bashy fetched, not a host
// install) to its own helpers before exec. A host podman on $PATH is left alone.
//
// linux: podman-static expects to be unpacked at /. It is not, so a generated
// containers.conf (CONTAINERS_CONF_OVERRIDE) names the conmon / crun / netavark
// paths inside the cache, the shipped registries.conf and storage.conf are
// pointed at through their env vars, and a signature policy is written into
// the user's containers config dir if none exists (podman refuses to build
// without one). Rootless prerequisites that only the host can provide —
// newuidmap/newgidmap + /etc/subuid — are NOT papered over: podman reports them.
//
// darwin: helper_binaries_dir → the dir holding gvproxy + vfkit.
func applyManagedPodmanEnv(bin string) {
	if !strings.HasPrefix(bin, engineCacheDir()) {
		return
	}
	switch runtime.GOOS {
	case "linux":
		root := managedPodmanRoot(bin)
		if root == "" {
			return
		}
		conf, err := writePodmanConfOverride(root)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bashy podman: %v\n", err)
			return
		}
		setenvDefault("CONTAINERS_CONF_OVERRIDE", conf)
		setenvDefault("CONTAINERS_REGISTRIES_CONF", filepath.Join(root, "etc", "containers", "registries.conf"))
		setenvDefault("CONTAINERS_STORAGE_CONF", filepath.Join(root, "etc", "containers", "storage.conf"))
		os.Setenv("PATH", filepath.Join(root, "usr", "local", "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
		ensureUserPolicyJSON(filepath.Join(root, "etc", "containers", "policy.json"))
		if hint := apparmorUsernsHint(os.Geteuid(), readTrimmed("/proc/sys/kernel/apparmor_restrict_unprivileged_userns")); hint != "" {
			fmt.Fprintln(os.Stderr, hint)
		}
	case "darwin":
		conf, err := writeDarwinConfOverride()
		if err != nil {
			fmt.Fprintf(os.Stderr, "bashy podman: %v\n", err)
			return
		}
		setenvDefault("CONTAINERS_CONF_OVERRIDE", conf)
	}
}

// apparmorUsernsHint explains the one Linux host fact that makes a rootless
// managed podman fail with the cryptic "failed to reexec: Permission denied":
// Ubuntu 24.04+ (kernel.apparmor_restrict_unprivileged_userns=1) moves an
// unconfined process that creates a user namespace into the `unprivileged_userns`
// AppArmor profile, which denies exec of /proc/self/exe. The distro's own podman
// escapes it through its packaged AppArmor profile; ours has none. Measured on a
// GitHub ubuntu-24.04 runner (audit: apparmor="DENIED" operation="exec"
// name="/proc/self/exe" profile="unprivileged_userns"). Root is unaffected.
func apparmorUsernsHint(euid int, sysctl string) string {
	if euid == 0 || sysctl != "1" {
		return ""
	}
	return "bashy podman: this host restricts unprivileged user namespaces (kernel.apparmor_restrict_unprivileged_userns=1);\n" +
		"  if podman answers \"failed to reexec: Permission denied\", either run it as root, or once:\n" +
		"    sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0   # persist in /etc/sysctl.d/"
}

func readTrimmed(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func setenvDefault(k, v string) {
	if strings.TrimSpace(os.Getenv(k)) == "" {
		os.Setenv(k, v)
	}
}

// podmanConfOverrideLinux renders the override for a podman-static tree at root.
func podmanConfOverrideLinux(root string) string {
	q := func(p ...string) string { return fmt.Sprintf("%q", path.Join(append([]string{root}, p...)...)) }
	return "# generated by bashy podman — paths into the managed podman-static tree\n" +
		"[engine]\n" +
		"cgroup_manager = \"cgroupfs\"\n" +
		"events_logger = \"file\"\n" +
		"runtime = \"crun\"\n" +
		"conmon_path = [" + q("usr", "local", "lib", "podman", "conmon") + "]\n" +
		"helper_binaries_dir = [" + q("usr", "local", "lib", "podman") + ", " + q("usr", "local", "bin") + "]\n" +
		"[engine.runtimes]\n" +
		"crun = [" + q("usr", "local", "bin", "crun") + "]\n" +
		"runc = [" + q("usr", "local", "bin", "runc") + "]\n" +
		"[network]\n" +
		"network_backend = \"netavark\"\n"
}

func writePodmanConfOverride(root string) (string, error) {
	p := filepath.Join(root, podmanConfOverrideName)
	want := podmanConfOverrideLinux(root)
	if b, err := os.ReadFile(p); err == nil && string(b) == want {
		return p, nil
	}
	return p, os.WriteFile(p, []byte(want), 0o644)
}

func writeDarwinConfOverride() (string, error) {
	dir := darwinHelperDir()
	if dir == "" {
		return "", fmt.Errorf("no cache dir")
	}
	p := filepath.Join(dir, podmanConfOverrideName)
	want := "# generated by bashy podman — machine helpers bashy fetched\n[engine]\nhelper_binaries_dir = [" + fmt.Sprintf("%q", dir) + "]\n"
	if b, err := os.ReadFile(p); err == nil && string(b) == want {
		return p, nil
	}
	return p, os.WriteFile(p, []byte(want), 0o644)
}

// ensureUserPolicyJSON copies the shipped signature policy into the user's
// containers config dir when neither it nor the system policy exists — podman
// cannot build or pull without one, and a fresh host has none.
func ensureUserPolicyJSON(shipped string) {
	if _, err := os.Stat("/etc/containers/policy.json"); err == nil {
		return
	}
	cfg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if cfg == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		cfg = filepath.Join(home, ".config")
	}
	dest := filepath.Join(cfg, "containers", "policy.json")
	if _, err := os.Stat(dest); err == nil {
		return
	}
	b, err := os.ReadFile(shipped)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(dest, b, 0o644)
}
