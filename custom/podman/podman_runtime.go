// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/podman/winhelper"
	"github.com/qiangli/yoke/pkg/binmgr"
)

const nftHelperEnvName = "BASHY_PODMAN_NFT_HELPER"

func nftHelperDir() string {
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = os.TempDir()
	}
	return filepath.Join(cache, "bashy", "podman-conf", "nft-helper")
}

func ensureNFTHelper() (string, error) {
	target := adapter.BashySelf()
	if target == "" {
		if p, err := exec.LookPath("bashy"); err == nil {
			target = p
		}
	}
	if target == "" {
		return "", fmt.Errorf("cannot resolve bashy executable for nft helper")
	}
	dir := nftHelperDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	link := filepath.Join(dir, "nft")
	if cur, err := os.Readlink(link); err == nil && cur == target {
		return dir, nil
	}
	tmp := fmt.Sprintf("%s.%d", link, os.Getpid())
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, link); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("replace nft helper: %w", err)
	}
	return dir, nil
}

const (
	podmanUpstreamVersion  = "v6.1.2"
	podmanDarwinAmd64Vers  = "v5.8.7"
	gvproxyDarwinVersion   = "v0.8.8"
	vfkitVersion           = "v0.6.4"
	podmanStaticRepo       = "mgoltzsche/podman-static"
	podmanUpstreamRepo     = "containers/podman"
	gvisorTapVsockRepo     = "containers/gvisor-tap-vsock"
	vfkitRepo              = "crc-org/vfkit"
	podmanHelperDirName    = "podman-helpers"
	podmanConfOverrideName = "containers.conf"
)

type podmanAsset struct {
	version, url, sha256, entrypoint string
}

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

var darwinHelperAssets = map[string]podmanAsset{
	"gvproxy": {gvproxyDarwinVersion, ghDownload(gvisorTapVsockRepo, gvproxyDarwinVersion, "gvproxy-darwin"),
		"614d58dd3f8473258fb6ec18f7631000d2e5c9a761ca52748cc714c07d16382f", ""},
	"vfkit": {vfkitVersion, ghDownload(vfkitRepo, vfkitVersion, "vfkit"),
		"0ed83fc8ca7aa708598835480dba1362406aa7cd1dab3b27464eb76327d9652d", ""},
}

func ghDownload(repo, version, asset string) string {
	return "https://github.com/" + repo + "/releases/download/" + version + "/" + asset
}

func engineCacheDir() string {
	d, err := binmgr.CacheDir()
	if err != nil {
		return ""
	}
	return d
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return fi.Mode()&0o111 != 0
}

func sameFile(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	fa, ea := os.Stat(a)
	fb, eb := os.Stat(b)
	return ea == nil && eb == nil && os.SameFile(fa, fb)
}

func resolvePodmanBinary() string {
	self, _ := os.Executable()
	bashySelf := adapter.BashySelf()
	if p, err := exec.LookPath("podman"); err == nil && p != "" && !sameFile(p, self) && !sameFile(p, bashySelf) {
		return p
	}
	dir := engineCacheDir()
	if dir == "" {
		return ""
	}
	exe := "podman" + exeSuffix()
	for _, cand := range []string{filepath.Join(dir, exe), filepath.Join(dir, "podman")} {
		if isExecutable(cand) {
			return cand
		}
	}
	best, bestMod := "", int64(0)
	for _, pat := range []string{filepath.Join(dir, "podman", "*", exe), filepath.Join(dir, "podman", "*", "podman")} {
		matches, _ := filepath.Glob(pat)
		for _, m := range matches {
			fi, err := os.Stat(m)
			if err != nil || !isExecutable(m) {
				continue
			}
			if mt := fi.ModTime().UnixNano(); best == "" || mt > bestMod {
				best, bestMod = m, mt
			}
		}
	}
	return best
}

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

func provisionPodman(ctx context.Context) string {
	p := cachedPodman()
	if p == "" {
		t, ok := podmanTool()
		if !ok {
			return ""
		}
		if !binmgr.Offline() {
			fmt.Fprintf(os.Stderr, "bashy podman: fetching podman %s for %s — first run only…\n", t.Version, binmgr.Platform())
		}
		var err error
		p, err = binmgr.Ensure(ctx, t)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bashy podman: %v\n", err)
			return ""
		}
	}
	if runtime.GOOS == "darwin" {
		if err := provisionDarwinHelpers(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "bashy podman: %v\n", err)
			return ""
		}
	}
	return p
}

func darwinHelperDir() string {
	root, err := binmgr.CacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(root, podmanHelperDirName)
}

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
		t := binmgr.Tool{Name: "podman-" + name, Version: a.version, Assets: map[string]binmgr.Asset{
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

func applyPodmanHelperEnv() {
	if runtime.GOOS != "windows" {
		return
	}
	if err := winhelper.Apply(engineCacheDir()); err != nil {
		fmt.Fprintf(os.Stderr, "bashy podman: could not provision Windows helper binaries: %v\n", err)
	}
}

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

func applyManagedPodmanEnv(bin string) {
	if runtime.GOOS == "linux" {
		os.Setenv("PATH", podmanLinuxPath(os.Getenv("PATH")))
	}
	if !strings.HasPrefix(bin, engineCacheDir()) {
		return
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BASHY_PODMAN_SYSTEM"))) {
	case "1", "true", "yes", "on":
		return
	}
	switch runtime.GOOS {
	case "linux":
		root := managedPodmanRoot(bin)
		if root == "" {
			return
		}
		helperDir, err := ensureNFTHelper()
		if err != nil {
			fmt.Fprintf(os.Stderr, "bashy podman: stage nft helper: %v\n", err)
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
		os.Setenv("PATH", helperDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		os.Setenv(nftHelperEnvName, "1")
		ensureUserPolicyJSON(filepath.Join(root, "etc", "containers", "policy.json"))
		if hint := apparmorUsernsHint(os.Geteuid(), readTrimmed("/proc/sys/kernel/apparmor_restrict_unprivileged_userns")); hint != "" {
			fmt.Fprintln(os.Stderr, hint)
		}
	case "darwin":
		if err := provisionDarwinHelpers(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "bashy podman: %v\n", err)
			return
		}
		conf, err := writeDarwinConfOverride()
		if err != nil {
			fmt.Fprintf(os.Stderr, "bashy podman: %v\n", err)
			return
		}
		setenvDefault("CONTAINERS_CONF_OVERRIDE", conf)
	}
}

func podmanLinuxPath(current string) string {
	parts := strings.Split(current, ":")
	seen := make(map[string]bool, len(parts)+3)
	for _, p := range parts {
		seen[p] = true
	}
	for _, p := range []string{"/usr/local/sbin", "/usr/sbin", "/sbin"} {
		if !seen[p] {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ":")
}

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

func podmanConfOverrideLinux(root string) string {
	q := func(p ...string) string { return fmt.Sprintf("%q", path.Join(append([]string{root}, p...)...)) }
	return "# generated by bashy podman — paths into the managed podman-static tree\n" +
		"[engine]\n" +
		"cgroup_manager = \"cgroupfs\"\n" +
		"events_logger = \"file\"\n" +
		"runtime = \"crun\"\n" +
		"conmon_path = [" + q("usr", "local", "lib", "podman", "conmon") + "]\n" +
		"helper_binaries_dir = [" + fmt.Sprintf("%q", nftHelperDir()) + ", " + q("usr", "local", "lib", "podman") + ", " + q("usr", "local", "bin") + "]\n" +
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

func podmanStartsMachine(args []string) bool {
	var words []string
	afterFlag := false
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "-"):
			afterFlag = !strings.Contains(a, "=")
		case afterFlag && a != "machine":
			afterFlag = false
		default:
			words = append(words, a)
			afterFlag = false
		}
	}
	return len(words) >= 2 && words[0] == "machine" && words[1] == "start"
}

func engineDaemonDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return os.TempDir()
}

func execEnginePassthrough(bin string, args []string) int {
	cmd := binmgr.Command(context.Background(), bin, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if podmanStartsMachine(args) {
		cmd.Dir = engineDaemonDir()
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "bashy: %s: %v\n", bin, err)
		return 1
	}
	stop := adapter.ForwardSignals(cmd.Process)
	err := cmd.Wait()
	stop()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "bashy: %s: %v\n", bin, err)
		return 1
	}
	return 0
}
