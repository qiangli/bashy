package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// remoteInstallDir is owned by bashy. Bootstrap must not replace a user's
// PATH binary or otherwise modify existing work on the remote host.
const remoteInstallDir = ".bashy/remote/bin"

var errRemoteNotInstalled = errors.New("bashy is not installed in the remote workspace")

func binaryNameForGOOS(goos string) string {
	if goos == "windows" {
		return "bashy.exe"
	}
	return "bashy"
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
		return atomicWriteFile(dst, in, 0o755)
	}
	return t.scpCopy(host, localPath, remoteTmp)
}

func (t *transport) chmodAndMove(host, tmp, dst string) error {
	if t.isFake {
		// dst and tmp are absolute local paths already (fake)
		if err := os.Chmod(tmp, 0o755); err != nil {
			return err
		}
		return replaceFile(tmp, dst)
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

// atomicWriteFile writes r to a temp file beside path, fsyncs, chmods and
// renames it over path, so a running copy of path is never opened for writing.
func atomicWriteFile(path string, r io.Reader, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".new-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = io.Copy(f, r)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, mode)
	}
	if err == nil {
		err = replaceFile(tmp, path)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}

// replaceFile renames src over dst. Windows cannot replace a running exe, so
// the old file is first renamed aside to dst+".old".
func replaceFile(src, dst string) error {
	if runtime.GOOS == "windows" {
		old := dst + ".old"
		_ = os.Remove(old)
		if err := os.Rename(dst, old); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(src, dst); err != nil {
			_ = os.Rename(old, dst)
			return err
		}
		_ = os.Remove(old)
		return nil
	}
	return os.Rename(src, dst)
}
