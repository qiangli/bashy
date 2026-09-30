// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/qiangli/bashy/internal/cli"
	"github.com/qiangli/yoke/pkg/binmgr"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
)

// remoteInstallDir is owned by bashy. Bootstrap must not replace a user's
// PATH binary or otherwise modify existing work on the remote host.
const remoteInstallDir = ".bashy/remote/bin"

var errRemoteNotInstalled = errors.New("bashy is not installed in the remote workspace")

func remoteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote",
		Short: "Remote host operations",
		Long:  `Manage bashy on remote hosts. The first step is remote install: push the self-contained bashy binary to a remote host via the host OS transport (ssh/scp on Unix), idempotently and version-matched, from the local release cache.`,
	}
	cmd.AddCommand(remoteInstallCmd(), peerServeCmd())
	return cmd
}

// peerServeCmd is the long-running endpoint started by remote install. Its
// keys are provisioned once by the host-OS bootstrap transport.
func peerServeCmd() *cobra.Command {
	var authorized, hostKey string
	cmd := &cobra.Command{Use: "peer-serve", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()
		server, err := StartPeerSSHServer(ctx, PeerSSHServerConfig{AuthorizedKeysPath: authorized, HostKeyPath: hostKey})
		if err != nil {
			return err
		}
		defer server.Close()
		return server.Wait()
	}}
	cmd.Flags().StringVar(&authorized, "authorized-keys", "", "Bashy peer authorized_keys")
	cmd.Flags().StringVar(&hostKey, "host-key", "", "Bashy peer host private key")
	_ = cmd.MarkFlagRequired("authorized-keys")
	_ = cmd.MarkFlagRequired("host-key")
	return cmd
}

func remoteInstallCmd() *cobra.Command {
	var fakeRoot string
	cmd := &cobra.Command{
		Use:   "install <host>",
		Short: "Install bashy on a remote host (idempotent, version-matched)",
		Long: `Push the self-contained bashy to a remote host if missing, via whatever the host OS offers (ssh/scp on Unix hosts where available; the OS-native equivalent elsewhere), idempotently and version-matched.

The local binary for the remote os/arch is taken from the local release cache (binmgr) when running a release, or from the current executable when running a dev build — no internet is needed on the remote. After this one bootstrap nothing depends on the remote's sshd. Re-running is a no-op when versions match; a version mismatch upgrades the remote.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			host := strings.TrimSpace(args[0])
			if host == "" {
				return fmt.Errorf("host is required")
			}
			return runRemoteInstall(cmd, host, fakeRoot)
		},
	}
	cmd.Flags().StringVar(&fakeRoot, "fake-root", "", "Fake remote root directory (hidden, for tests)")
	_ = cmd.Flags().MarkHidden("fake-root")
	return cmd
}

func runRemoteInstall(cmd *cobra.Command, host, fakeRoot string) error {
	stdout := cmd.OutOrStdout()
	stderr := cmd.ErrOrStderr()

	t := newTransport(host, fakeRoot)

	// 1. Detect remote OS/arch.
	goos, goarch, err := t.detectOSArch(host)
	if err != nil {
		return fmt.Errorf("detect remote os/arch: %w", err)
	}
	// 2. Find local binary for that platform.
	localBin, localLauncher, err := findLocalBinaryForHost(goos, goarch)
	if err != nil {
		return err
	}
	// 3. Determine version strings.
	localVer, err := localVersionString(localBin)
	if err != nil {
		return fmt.Errorf("local version: %w", err)
	}
	if localVer == "" {
		return fmt.Errorf("could not determine local version from %s", localBin)
	}
	remoteVer, err := t.remoteVersion(host, goos)
	if err != nil && !errors.Is(err, errRemoteNotInstalled) {
		return fmt.Errorf("inspect remote installation: %w", err)
	}

	alreadyInstalled := remoteVer != "" && remoteVer == localVer
	if alreadyInstalled && !remotePeerServeAvailable(t, host) {
		alreadyInstalled = false
	}
	if alreadyInstalled {
		fmt.Fprintf(stdout, "bashy %s already installed on %s\n", localVer, host)
	} else if remoteVer != "" {
		fmt.Fprintf(stderr, "upgrading %s: %s -> %s\n", host, remoteVer, localVer)
	} else {
		fmt.Fprintf(stdout, "installing bashy %s to %s (%s/%s)\n", localVer, host, goos, goarch)
	}

	if !alreadyInstalled {
		if err := t.ensureRemoteDir(host); err != nil {
			return fmt.Errorf("create remote dir: %w", err)
		}

		// For Unix hosts with a launcher pair, install both files; otherwise single binary.
		if localLauncher != "" && needsLauncher(goos) {
			remoteReal := t.remotePath(host, "bashy.real")
			remoteLauncher := t.remotePath(host, "bashy")
			// install real first so launcher never points to missing payload
			if err := t.copyFile(host, localBin, remoteReal+".tmp"); err != nil {
				return err
			}
			if err := t.chmodAndMove(host, remoteReal+".tmp", remoteReal); err != nil {
				return err
			}
			if err := t.copyFile(host, localLauncher, remoteLauncher+".tmp"); err != nil {
				return err
			}
			if err := t.chmodAndMove(host, remoteLauncher+".tmp", remoteLauncher); err != nil {
				return err
			}
		} else {
			remoteBin := t.remotePath(host, binaryNameForGOOS(goos))
			if err := t.copyFile(host, localBin, remoteBin+".tmp"); err != nil {
				return err
			}
			if err := t.chmodAndMove(host, remoteBin+".tmp", remoteBin); err != nil {
				return err
			}
		}

		// Verify installed version.
		newVer, err := t.remoteVersion(host, goos)
		if err != nil {
			return fmt.Errorf("verify remote install: %w", err)
		}
		if newVer != localVer {
			return fmt.Errorf("remote version mismatch after install: got %q want %q", newVer, localVer)
		}
		fmt.Fprintf(stdout, "installed bashy %s on %s\n", localVer, host)
	}
	if err := provisionPeerIdentity(t, host); err != nil {
		return fmt.Errorf("provision peer identity: %w", err)
	}
	if !t.isFake {
		if err := startRemotePeerServer(t, host); err != nil {
			return err
		}
	}
	return nil
}

const remotePeerDir = ".bashy/remote"

func peerLocalDir(host string) string {
	return filepath.Join(os.Getenv("HOME"), ".bashy", "remote", "peers", safePeerName(host))
}

func safePeerName(host string) string {
	var b strings.Builder
	for _, r := range host {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func provisionPeerIdentity(t *transport, host string) error {
	dir := peerLocalDir(host)
	if t.isFake {
		root := t.fakeRoot
		if strings.HasPrefix(host, "fake://") {
			root = strings.TrimPrefix(host, "fake://")
		}
		dir = filepath.Join(root, ".bashy", "remote", "peers", safePeerName(host))
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for _, ownedDir := range []string{dir, filepath.Dir(dir), filepath.Dir(filepath.Dir(dir))} {
		if err := os.Chmod(ownedDir, 0o700); err != nil {
			return err
		}
	}
	localPriv := filepath.Join(dir, "id_ed25519")
	localPub := filepath.Join(dir, "id_ed25519.pub")
	_, pub, err := loadOrCreatePeerKey(localPriv, localPub)
	if err != nil {
		return err
	}

	remoteBase := remotePeerDir
	if t.isFake {
		root := t.fakeRoot
		if strings.HasPrefix(host, "fake://") {
			root = strings.TrimPrefix(host, "fake://")
		}
		remoteBase = filepath.Join(root, remotePeerDir)
		if err := os.MkdirAll(remoteBase, 0o700); err != nil {
			return err
		}
	} else if _, err := t.sshRun(host, "mkdir -p \"$HOME/"+remotePeerDir+"\" && chmod 700 \"$HOME/"+remotePeerDir+"\""); err != nil {
		return err
	}

	remoteAuthorized := filepath.Join(remoteBase, "authorized_keys")
	remoteHostKey := filepath.Join(remoteBase, "host_ed25519")
	if !t.isFake {
		remoteAuthorized = "~/" + remotePeerDir + "/authorized_keys"
		remoteHostKey = "~/" + remotePeerDir + "/host_ed25519"
	}
	pubKey, err := ssh.NewPublicKey(pub)
	if err != nil {
		return err
	}
	if err := putRemoteIfMissing(t, host, remoteAuthorized, ssh.MarshalAuthorizedKey(pubKey), 0o600); err != nil {
		return err
	}
	if err := secureRemoteFile(t, host, remoteAuthorized, 0o600); err != nil {
		return err
	}
	knownHostPath := filepath.Join(dir, "host_key.pub")
	known, err := os.ReadFile(knownHostPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if os.IsNotExist(err) {
		tmpDir, err := os.MkdirTemp("", "bashy-peer-hostkey-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)
		hostPriv, hostPub, err := loadOrCreatePeerKey(filepath.Join(tmpDir, "host_ed25519"), filepath.Join(tmpDir, "host_ed25519.pub"))
		if err != nil {
			return err
		}
		if err := putRemoteIfMissing(t, host, remoteHostKey, hostPriv, 0o600); err != nil {
			return err
		}
		if err := secureRemoteFile(t, host, remoteHostKey, 0o600); err != nil {
			return err
		}
		hostPubKey, err := ssh.NewPublicKey(hostPub)
		if err != nil {
			return err
		}
		known = ssh.MarshalAuthorizedKey(hostPubKey)
		if err := writeNewFile(knownHostPath, known, 0o600); err != nil {
			return err
		}
	} else {
		// A trust pin with no remote host key means the remote state was changed.
		if t.isFake {
			if _, statErr := os.Stat(remoteHostKey); statErr != nil {
				return errors.New("pinned peer host key exists but remote key is missing")
			}
		} else {
			if _, statErr := t.sshRun(host, "test -f \"$HOME/"+remotePeerDir+"/host_ed25519\""); statErr != nil {
				return errors.New("pinned peer host key exists but remote key is missing")
			}
		}
	}
	if err := secureRemoteFile(t, host, remoteHostKey, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(knownHostPath, 0o600); err != nil {
		return err
	}
	_ = pub
	return nil
}

func loadOrCreatePeerKey(privatePath, publicPath string) ([]byte, ed25519.PublicKey, error) {
	priv, err := os.ReadFile(privatePath)
	if err == nil {
		key, parseErr := ssh.ParsePrivateKey(priv)
		if parseErr != nil {
			return nil, nil, parseErr
		}
		cryptoKey, ok := key.PublicKey().(ssh.CryptoPublicKey)
		if !ok {
			return nil, nil, errors.New("peer key is not a crypto public key")
		}
		pub, ok := cryptoKey.CryptoPublicKey().(ed25519.PublicKey)
		if !ok {
			return nil, nil, errors.New("peer key is not ed25519")
		}
		if _, statErr := os.Stat(publicPath); os.IsNotExist(statErr) {
			if err := writeNewFile(publicPath, ssh.MarshalAuthorizedKey(key.PublicKey()), 0o600); err != nil {
				return nil, nil, err
			}
		} else if statErr != nil {
			return nil, nil, statErr
		} else {
			existing, readErr := os.ReadFile(publicPath)
			if readErr != nil {
				return nil, nil, readErr
			}
			pinned, _, _, _, parseErr := ssh.ParseAuthorizedKey(existing)
			if parseErr != nil || !bytes.Equal(pinned.Marshal(), key.PublicKey().Marshal()) {
				return nil, nil, errors.New("peer public key does not match persisted private key")
			}
		}
		if err := os.Chmod(privatePath, 0o600); err != nil {
			return nil, nil, err
		}
		if err := os.Chmod(publicPath, 0o600); err != nil {
			return nil, nil, err
		}
		return priv, pub, nil
	}
	if !os.IsNotExist(err) {
		return nil, nil, err
	}
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	block, err := ssh.MarshalPrivateKey(private, "bashy remote peer")
	if err != nil {
		return nil, nil, err
	}
	priv = pem.EncodeToMemory(block)
	if err := writeNewFile(privatePath, priv, 0o600); err != nil {
		return nil, nil, err
	}
	pubKey, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, nil, err
	}
	line := ssh.MarshalAuthorizedKey(pubKey)
	if err := writeNewFile(publicPath, line, 0o600); err != nil {
		return nil, nil, err
	}
	return priv, pub, nil
}

func writeNewFile(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func putRemoteIfMissing(t *transport, host, path string, data []byte, mode os.FileMode) error {
	if t.isFake {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		tmp, err := os.CreateTemp(filepath.Dir(path), ".peer-key-*")
		if err != nil {
			return err
		}
		if _, err = tmp.Write(data); err != nil {
			_ = tmp.Close()
			return err
		}
		if err = tmp.Close(); err != nil {
			return err
		}
		if err = os.Chmod(tmp.Name(), mode); err != nil {
			return err
		}
		if err = os.Link(tmp.Name(), path); err != nil && !os.IsExist(err) {
			return err
		}
		_ = os.Remove(tmp.Name())
		return nil
	}
	// SCP to a unique temporary then atomically move only if absent.
	local, err := os.CreateTemp("", "bashy-peer-key-*")
	if err != nil {
		return err
	}
	name := local.Name()
	defer os.Remove(name)
	if _, err = local.Write(data); err != nil {
		_ = local.Close()
		return err
	}
	if err = local.Close(); err != nil {
		return err
	}
	tmp := path + ".bashy-tmp"
	if err := t.scpCopy(host, name, tmp); err != nil {
		return err
	}
	cmd := "chmod 600 \"$HOME/" + strings.TrimPrefix(tmp, "~/") + "\" && (test -e \"$HOME/" + strings.TrimPrefix(path, "~/") + "\" || mv \"$HOME/" + strings.TrimPrefix(tmp, "~/") + "\" \"$HOME/" + strings.TrimPrefix(path, "~/") + "\") && rm -f \"$HOME/" + strings.TrimPrefix(tmp, "~/") + "\""
	_, err = t.sshRun(host, cmd)
	return err
}

func secureRemoteFile(t *transport, host, path string, mode os.FileMode) error {
	if t.isFake {
		return os.Chmod(path, mode)
	}
	_, err := t.sshRun(host, fmt.Sprintf("chmod %o \"$HOME/%s\"", mode.Perm(), strings.TrimPrefix(path, "~/")))
	return err
}

func startRemotePeerServer(t *transport, host string) error {
	user, err := t.sshRun(host, "id -un")
	if err != nil {
		return fmt.Errorf("identify remote peer user: %w", err)
	}
	user = strings.TrimSpace(user)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	ch, dialErr := DialInstalledPeerChannel(ctx, host, user)
	cancel()
	if dialErr == nil {
		_ = ch.Close()
		return nil
	}
	if !errors.Is(dialErr, syscall.ECONNREFUSED) {
		return fmt.Errorf("verify existing Bashy peer listener on %s: %w", host, dialErr)
	}
	cmd := "nohup \"$HOME/" + remoteInstallDir + "/bashy\" remote peer-serve --authorized-keys \"$HOME/" + remotePeerDir + "/authorized_keys\" --host-key \"$HOME/" + remotePeerDir + "/host_ed25519\" </dev/null >/dev/null 2>&1 &"
	if _, err := t.sshRun(host, cmd); err != nil {
		return fmt.Errorf("start Bashy peer server: %w", err)
	}
	var last error
	for i := 0; i < 50; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		ch, err := DialInstalledPeerChannel(ctx, host, user)
		cancel()
		if err == nil {
			_ = ch.Close()
			return nil
		}
		last = err
		if !errors.Is(err, syscall.ECONNREFUSED) {
			return fmt.Errorf("verify Bashy peer listener on %s: %w", host, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("Bashy peer listener on %s: %w", host, last)
}

func remotePeerServeAvailable(t *transport, host string) bool {
	if t.isFake {
		return true
	}
	_, err := t.sshRun(host, remoteShellPath(remoteInstallDir+"/bashy")+" remote peer-serve --help >/dev/null 2>&1")
	return err == nil
}

func needsLauncher(goos string) bool {
	return goos == "linux" || goos == "darwin"
}

func binaryNameForGOOS(goos string) string {
	if goos == "windows" {
		return "bashy.exe"
	}
	return "bashy"
}

// findLocalBinaryForHost returns the file that holds the bashy payload for the
// given remote os/arch, plus the launcher path when the local host build uses
// a native launcher (darwin/linux). For release builds the payload comes from
// the binmgr cache (~/.cache/bashy/bin/bashy/<tag>/bashy); for dev builds it
// is the current executable's real binary.
func findLocalBinaryForHost(goos, goarch string) (binPath, launcherPath string, err error) {
	// Release path: BashyVersion set => cached release binary.
	if v := cli.BashyVersion(); v != "" {
		tag := "v" + v
		// binmgr cache: <CacheDir>/bashy/<tag>/bashy[.exe]
		root, cerr := binmgr.CacheDir()
		if cerr != nil {
			return "", "", cerr
		}
		name := "bashy"
		if goos == "windows" {
			name = "bashy.exe"
		}
		cached := filepath.Join(root, "bashy", tag, name)
		if _, serr := os.Stat(cached); serr == nil {
			// For same-platform releases the cached binary is already the right arch;
			// for cross-platform we would need a different layout, but the cache
			// as currently populated only holds binaries for the builder's platform.
			// If remote differs, report clearly.
			if goos != runtime.GOOS || goarch != runtime.GOARCH {
				return "", "", fmt.Errorf("cross-platform release install for %s/%s: cached binary at %s is %s/%s; fetch the release for the remote platform first (e.g. on a %s/%s host run: bashy self fetch --version %s)", goos, goarch, cached, runtime.GOOS, runtime.GOARCH, goos, goarch, tag)
			}
			return cached, "", nil
		}
		// Fallback: try bin/dist layout for cross-platform releases produced locally.
		distName := fmt.Sprintf("bashy-%s-%s", goos, goarch)
		if goos == "windows" {
			distName += ".exe"
		}
		distPath := filepath.Join("bin", "dist", distName)
		if _, serr := os.Stat(distPath); serr == nil {
			return distPath, "", nil
		}
		return "", "", fmt.Errorf("release binary for %s/%s not in local cache %s; run: bashy self fetch --version %s", goos, goarch, cached, tag)
	}
	// Dev path: use current executable. When running under go test the executable is the test binary,
	// so fall back to locating bashy on PATH / at the well-known install locations.
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return "", "", fmt.Errorf("cannot resolve current executable")
	}
	// If exe looks like a Go test binary, ignore it and locate bashy via PATH/install dir.
	if strings.HasSuffix(exe, ".test") || strings.Contains(filepath.Base(exe), ".test") {
		if p, lerr := exec.LookPath("bashy"); lerr == nil {
			exe = p
		} else {
			// Fallback to the installer default locations.
			cands := []string{
				filepath.Join(os.Getenv("HOME"), ".local", "bin", "bashy"),
				"bin/bashy",
				"bin/bashy.real",
			}
			found := ""
			for _, c := range cands {
				if _, serr := os.Stat(c); serr == nil {
					found = c
					break
				}
			}
			if found == "" {
				return "", "", fmt.Errorf("cannot locate bashy binary for dev install (test mode)")
			}
			exe = found
		}
	}
	// Resolve symlink if any (macOS launcher may be at ~/.local/bin/bashy -> bashy.real sibling).
	if resolved, rerr := filepath.EvalSymlinks(exe); rerr == nil {
		exe = resolved
	}
	realBin := exe
	launcher := ""
	if strings.HasSuffix(exe, ".real") {
		launcher = strings.TrimSuffix(exe, ".real")
		if _, serr := os.Stat(launcher); serr != nil {
			launcher = ""
		}
	} else {
		candidate := exe + ".real"
		if _, serr := os.Stat(candidate); serr == nil {
			realBin = candidate
			launcher = exe
		} else {
			// No .real sibling — pure-Go single binary (e.g. cached release run as dev? or windows)
			launcher = ""
		}
	}
	// When LookPath resolved to the launcher (small 34KB Mach-O), the above correctly picks .real as payload.
	// If LookPath resolved to a test binary path that had no .real, we may have realBin == launcher path (small);
	// verify the chosen realBin is actually a bashy payload by checking its size/--version, otherwise try the .real sibling at the install dir.
	if fi, serr := os.Stat(realBin); serr == nil && fi.Size() < 1<<20 {
		// Likely the launcher, not the payload. Try explicit install-dir payload.
		for _, cand := range []string{
			filepath.Join(filepath.Dir(realBin), "bashy.real"),
			filepath.Join(os.Getenv("HOME"), ".local", "bin", "bashy.real"),
		} {
			if _, cerr := os.Stat(cand); cerr == nil {
				realBin = cand
				if strings.HasSuffix(cand, ".real") {
					launcher = strings.TrimSuffix(cand, ".real")
				}
				break
			}
		}
	}
	if _, serr := os.Stat(realBin); serr != nil {
		return "", "", fmt.Errorf("local binary not found: %s", realBin)
	}
	if goos != runtime.GOOS || goarch != runtime.GOARCH {
		// Cross dev: need prebuilt dist.
		distName := fmt.Sprintf("bashy-%s-%s", goos, goarch)
		if goos == "windows" {
			distName += ".exe"
		}
		distPath := filepath.Join("bin", "dist", distName)
		if _, serr := os.Stat(distPath); serr == nil {
			return distPath, "", nil
		}
		return "", "", fmt.Errorf("cross-platform dev install for %s/%s: local is %s/%s; build with: make dist", goos, goarch, runtime.GOOS, runtime.GOARCH)
	}
	return realBin, launcher, nil
}

func localVersionString(binPath string) (string, error) {
	// Run the binary directly; the launcher is a tiny C shim that passes through --version.
	out, err := exec.Command(binPath, "--version").CombinedOutput()
	if err != nil {
		return "", err
	}
	return parseVersion(string(out)), nil
}

func parseVersion(out string) string {
	// Example: "bashy, GNU Bash 5.3 compatible, version 5.3.0(1)-bashy-dev (a5d5934)\n"
	// or "GNU bash, version 5.3.0(1)-bashy-0.20.0\n"
	if idx := strings.Index(out, "version "); idx >= 0 {
		rest := strings.TrimSpace(out[idx+len("version "):])
		if nl := strings.Index(rest, "\n"); nl >= 0 {
			rest = rest[:nl]
		}
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(out)
}

// transport abstracts local vs ssh/scp.
type transport struct {
	fakeRoot string
	isFake   bool
	fakeHost string // when host itself is a directory path
}

func newTransport(host, fakeRoot string) *transport {
	if fakeRoot != "" {
		return &transport{fakeRoot: fakeRoot, isFake: true}
	}
	if isFakeHostPath(host) {
		return &transport{fakeRoot: host, isFake: true, fakeHost: host}
	}
	// Also allow env-driven fake for tests that set BASHY_REMOTE_FAKE_ROOT
	if fr := strings.TrimSpace(os.Getenv("BASHY_REMOTE_FAKE_ROOT")); fr != "" {
		return &transport{fakeRoot: fr, isFake: true}
	}
	return &transport{isFake: false}
}

func isFakeHostPath(host string) bool {
	if host == "" {
		return false
	}
	if strings.HasPrefix(host, "/") || strings.HasPrefix(host, "./") || strings.HasPrefix(host, "../") {
		if fi, err := os.Stat(host); err == nil && fi.IsDir() {
			return true
		}
	}
	// explicit fake scheme
	if strings.HasPrefix(host, "fake://") {
		return true
	}
	return false
}

func (t *transport) remotePath(host, name string) string {
	if t.isFake {
		root := t.fakeRoot
		if strings.HasPrefix(host, "fake://") {
			root = strings.TrimPrefix(host, "fake://")
		}
		if strings.HasSuffix(root, remoteInstallDir) {
			return filepath.Join(root, name)
		}
		return filepath.Join(root, remoteInstallDir, name)
	}
	// Keep this relative for scp's SFTP mode; unlike a remote shell it does not
	// expand '~'. SSH commands use remoteShellPath below.
	return remoteInstallDir + "/" + name
}

func remoteShellPath(path string) string { return "$HOME/" + path }

func (t *transport) detectOSArch(host string) (string, string, error) {
	if t.isFake {
		return runtime.GOOS, runtime.GOARCH, nil
	}
	out, err := t.sshRun(host, "uname -s; echo ---; uname -m")
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(strings.TrimSpace(out), "---")
	if len(parts) != 2 {
		// fallback: split lines
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) < 2 {
			return "", "", fmt.Errorf("unexpected uname output: %q", out)
		}
		return normalizeGOOS(lines[0]), normalizeGOARCH(lines[1]), nil
	}
	goos := normalizeGOOS(strings.TrimSpace(parts[0]))
	goarch := normalizeGOARCH(strings.TrimSpace(parts[1]))
	if goos == "" || goarch == "" {
		return "", "", fmt.Errorf("unknown remote platform: uname -s=%q uname -m=%q", parts[0], parts[1])
	}
	return goos, goarch, nil
}

func normalizeGOOS(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "darwin":
		return "darwin"
	case "linux":
		return "linux"
	case "windows", "mingw64_nt", "msys_nt", "cygwin_nt":
		return "windows"
	default:
		low := strings.ToLower(s)
		if strings.Contains(low, "darwin") {
			return "darwin"
		}
		if strings.Contains(low, "linux") {
			return "linux"
		}
		if strings.Contains(low, "mingw") || strings.Contains(low, "msys") || strings.Contains(low, "windows") || strings.Contains(low, "cygwin") {
			return "windows"
		}
		return ""
	}
}

func normalizeGOARCH(m string) string {
	switch strings.ToLower(strings.TrimSpace(m)) {
	case "x86_64", "amd64", "x64":
		return "amd64"
	case "arm64", "aarch64":
		return "arm64"
	case "386", "i386", "i686", "x86":
		return "386"
	case "arm":
		return "arm"
	default:
		return strings.ToLower(strings.TrimSpace(m))
	}
}

func (t *transport) remoteVersion(host, goos string) (string, error) {
	if t.isFake {
		// Try to run the fake binary directly.
		root := t.fakeRoot
		if strings.HasPrefix(host, "fake://") {
			root = strings.TrimPrefix(host, "fake://")
		}
		candidates := []string{
			filepath.Join(root, remoteInstallDir, binaryNameForGOOS(goos)),
			filepath.Join(root, remoteInstallDir, "bashy.real"),
			filepath.Join(root, binaryNameForGOOS(goos)),
			filepath.Join(root, "bashy"),
			filepath.Join(root, "bashy.real"),
		}
		if strings.HasSuffix(root, remoteInstallDir) {
			candidates = []string{filepath.Join(root, binaryNameForGOOS(goos)), filepath.Join(root, "bashy.real")}
		}
		for _, p := range candidates {
			if _, err := os.Stat(p); err != nil {
				continue
			}
			out, err := exec.Command(p, "--version").CombinedOutput()
			if err != nil {
				continue
			}
			return parseVersion(string(out)), nil
		}
		return "", errRemoteNotInstalled
	}
	// Probe only the directory owned by this bootstrap. In particular, do not
	// treat an unrelated PATH bashy as ours.
	script := remoteShellPath(remoteInstallDir+"/bashy") + " --version 2>&1 || true; echo ___BASHY_VER_SEP___; " + remoteShellPath(remoteInstallDir+"/bashy.real") + " --version 2>&1 || true"
	out, err := t.sshRun(host, script)
	if err != nil {
		// ssh itself failed -> host unreachable
		return "", err
	}
	parts := strings.Split(out, "___BASHY_VER_SEP___")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, "command not found") || strings.Contains(p, "No such file") || strings.Contains(p, "not found") {
			continue
		}
		if strings.Contains(strings.ToLower(p), "version") {
			return parseVersion(p), nil
		}
	}
	return "", errRemoteNotInstalled
}

func (t *transport) ensureRemoteDir(host string) error {
	if t.isFake {
		root := t.fakeRoot
		if strings.HasPrefix(host, "fake://") {
			root = strings.TrimPrefix(host, "fake://")
		}
		dir := filepath.Join(root, remoteInstallDir)
		if strings.HasSuffix(root, remoteInstallDir) {
			dir = root
		}
		return os.MkdirAll(dir, 0o755)
	}
	_, err := t.sshRun(host, "mkdir -p "+remoteShellPath(remoteInstallDir))
	return err
}

func (t *transport) copyFile(host, localPath, remoteTmp string) error {
	if t.isFake {
		root := t.fakeRoot
		if strings.HasPrefix(host, "fake://") {
			root = strings.TrimPrefix(host, "fake://")
		}
		var dst string
		if strings.HasPrefix(remoteTmp, "~/") {
			rel := strings.TrimPrefix(remoteTmp, "~/")
			// rel is .local/bin/bashy.tmp etc.
			dst = filepath.Join(root, rel)
			// handle .tmp suffix already in remoteTmp
			// root's home is root itself, so Join root + rel
			// If remoteTmp is ~/.local/bin/bashy.tmp -> dst = <root>/.local/bin/bashy.tmp
		} else if filepath.IsAbs(remoteTmp) {
			dst = remoteTmp
			// if fake root is not filesystem root, don't interpret as absolute on local; treat as relative to fake root if needed
		} else {
			// already expanded remotePath from remotePath() which for fake returns absolute local path
			dst = remoteTmp
		}
		// remotePath() for fake returns absolute already, so use that directly when copyFile is called with that absolute
		// But our caller passes t.remotePath(...) + ".tmp" which for fake is absolute local path + ".tmp"
		// So handle both.
		if !filepath.IsAbs(dst) {
			dst = remoteTmp
		}
		// Ensure parent exists
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		in, err := os.Open(localPath)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		_, cerr := io.Copy(out, in)
		_ = out.Close()
		return cerr
	}
	return t.scpCopy(host, localPath, remoteTmp)
}

func (t *transport) chmodAndMove(host, tmp, dst string) error {
	if t.isFake {
		// dst and tmp are absolute local paths already (fake)
		if err := os.Chmod(tmp, 0o755); err != nil {
			return err
		}
		return os.Rename(tmp, dst)
	}
	// Use ssh to chmod and mv atomically.
	// Quote paths for shell.
	// tmp and dst are generated by remotePath, never supplied by the host, so
	// the $HOME form is both safe and necessary for shell expansion.
	cmd := fmt.Sprintf("chmod +x %s && mv %s %s", remoteShellPath(tmp), remoteShellPath(tmp), remoteShellPath(dst))
	_, err := t.sshRun(host, cmd)
	return err
}

func (t *transport) sshRun(host, command string) (string, error) {
	// Use ssh with batchMode and timeouts. Host may be user@host.
	args := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		host,
		command,
	}
	c := exec.Command("ssh", args...)
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	if err := c.Run(); err != nil {
		return buf.String(), fmt.Errorf("ssh %s: %w: %s", host, err, buf.String())
	}
	return buf.String(), nil
}

func (t *transport) scpCopy(host, localPath, remotePath string) error {
	// remotePath is like ~/.local/bin/bashy.tmp ; scp needs host:remotePath
	dest := host + ":" + remotePath
	args := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		localPath, dest,
	}
	c := exec.Command("scp", args...)
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	if err := c.Run(); err != nil {
		return fmt.Errorf("scp %s -> %s: %w: %s", localPath, dest, err, buf.String())
	}
	return nil
}
