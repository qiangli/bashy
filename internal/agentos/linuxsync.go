package agentos

// Workspace sync for `bashy linux --host`.
//
// `bashy linux` run ON a host needs no sync: WSL already sees the Windows
// filesystem under /mnt and podman bind-mounts the cwd, so the files are
// already there. A --host HOP is the case where they are not — the workspace
// is on the peer that issued the command, and the Linux environment is on the
// host. --sync is that one missing edge: it copies the workspace across and
// tells the remote side which directory inside it to run in.
//
// What gets copied is deliberately narrow. In a git worktree only the files
// git itself would show — tracked, plus untracked that are not ignored, plus
// each submodule's own such files — are sent. A gate result has to be a
// property of the committed tree; shipping somebody's build outputs would
// both dwarf the transfer (cloudbox: 17 MB of sources under 810 MB of bin/)
// and let a stale artifact decide the answer. Outside a git worktree there is
// no such signal, so everything is sent.

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// linuxSyncPrefix is the one directory on a host that --sync writes, under the
// remote $HOME. It is fixed so the stale-tree removal below can refuse to
// delete anything else.
const linuxSyncPrefix = ".bashy/linux/workspaces"

// linuxWorkspace is where a hop's files come from and where they land.
type linuxWorkspace struct {
	root string // local workspace root: the git worktree root, else the cwd
	rel  string // cwd relative to root, in slash form; "" when cwd IS the root
	name string // the directory name chosen on the host
}

// localLinuxWorkspace answers what the cwd belongs to. The root is the whole
// worktree rather than the cwd because a module's own gate often needs its
// siblings: cloudbox's hub/ resolves through a go.work two levels up, so
// syncing hub/ alone cannot build.
func localLinuxWorkspace(ctx context.Context, env linuxEnv) (linuxWorkspace, error) {
	if env.cwd == "" {
		return linuxWorkspace{}, errors.New("no working directory to sync")
	}
	// Both sides go through EvalSymlinks before they are compared. On macOS
	// /tmp IS a symlink to /private/tmp, so an unresolved cwd of
	// /tmp/cb-dev/hub against a git root of /private/tmp/cb-dev produced the
	// relative path ../../../tmp/cb-dev/hub and a chdir off the end of the
	// synced tree (measured 2026-10-28).
	cwd := resolveDir(env.cwd)
	ws := linuxWorkspace{root: cwd}
	if top, err := gitTopLevel(ctx, env, cwd); err == nil {
		ws.root = resolveDir(top)
	}
	rel, err := filepath.Rel(ws.root, cwd)
	if err != nil {
		return ws, fmt.Errorf("locate %s inside %s: %w", cwd, ws.root, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ws, fmt.Errorf("%s is not inside its own workspace root %s", cwd, ws.root)
	}
	if rel != "." {
		ws.rel = filepath.ToSlash(rel)
	}
	// The name carries the base for a human reading `ls` and a digest of the
	// absolute path so two same-named checkouts on one peer cannot collide.
	sum := sha256.Sum256([]byte(ws.root))
	ws.name = filepath.Base(ws.root) + "-" + hex.EncodeToString(sum[:4])
	return ws, nil
}

// resolveDir follows symlinks when it can and otherwise answers what it was
// given: a path that cannot be resolved is still the best name available, and
// failing the sync over it would be worse than comparing it as written.
func resolveDir(dir string) string {
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		return real
	}
	return dir
}

func gitTopLevel(ctx context.Context, env linuxEnv, dir string) (string, error) {
	out, exit, err := env.run(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel")
	if err != nil || exit != 0 {
		return "", fmt.Errorf("%s is not a git worktree", dir)
	}
	top := strings.TrimSpace(string(out))
	if top == "" {
		return "", fmt.Errorf("%s is not a git worktree", dir)
	}
	return top, nil
}

// linuxSyncFiles is the file list, in slash-relative form, for one workspace.
func linuxSyncFiles(ctx context.Context, env linuxEnv, root string) ([]string, error) {
	if _, err := gitTopLevel(ctx, env, root); err != nil {
		return walkAllFiles(root)
	}
	files, err := gitCleanFiles(ctx, env, root, "")
	if err != nil {
		return nil, err
	}
	// A submodule is a gitlink in the parent's index, so the parent's own
	// ls-files never names its contents. go.work `use ./external/k3s` needs
	// them, so each one is listed in its own right.
	out, exit, err := env.run(ctx, "git", "-C", root, "submodule", "--quiet", "foreach", "--recursive", "echo $displaypath")
	if err == nil && exit == 0 {
		for _, sub := range strings.Fields(strings.TrimSpace(string(out))) {
			subFiles, err := gitCleanFiles(ctx, env, filepath.Join(root, filepath.FromSlash(sub)), sub)
			if err != nil {
				return nil, err
			}
			files = append(files, subFiles...)
		}
	}
	return files, nil
}

// gitCleanFiles is "what git would show here": tracked plus untracked that is
// not ignored, which is the working tree minus build output.
func gitCleanFiles(ctx context.Context, env linuxEnv, dir, prefix string) ([]string, error) {
	out, exit, err := env.run(ctx, "git", "-C", dir, "ls-files", "-z",
		"--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}
	if exit != 0 {
		return nil, fmt.Errorf("list %s: git ls-files exited %d: %s", dir, exit, strings.TrimSpace(string(out)))
	}
	var files []string
	for _, name := range strings.Split(string(out), "\x00") {
		if name == "" {
			continue
		}
		if prefix != "" {
			name = prefix + "/" + name
		}
		files = append(files, name)
	}
	return files, nil
}

func walkAllFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// .git is never sent: the remote tree is a workspace to run in,
			// not a checkout to commit from.
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	return files, err
}

// syncLinuxWorkspace copies the cwd's workspace to the host and answers the
// directory the remote side should run in, in the remote shell's own path
// form. The remote normalizes it (normalizeHostDir), so a Windows host may
// answer /c/Users/... and still be handed to wsl.exe correctly.
func syncLinuxWorkspace(ctx context.Context, env linuxEnv, spec linuxHostSpec, remote string) (string, error) {
	ws, err := localLinuxWorkspace(ctx, env)
	if err != nil {
		return "", err
	}
	files, err := linuxSyncFiles(ctx, env, ws.root)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("%s has nothing to sync", ws.root)
	}
	home, err := remoteHome(ctx, env, spec)
	if err != nil {
		return "", err
	}
	dest := home + "/" + linuxSyncPrefix + "/" + ws.name
	if !strings.Contains(dest, "/"+linuxSyncPrefix+"/") {
		// Unreachable by construction; the guard is here because the next
		// step removes dest recursively.
		return "", fmt.Errorf("refusing to sync to %q: not under %s", dest, linuxSyncPrefix)
	}
	fmt.Fprintf(env.stderr, "bashy linux: sync %d file(s) from %s to %s:%s\n",
		len(files), ws.root, spec.host, dest)

	// The tree is replaced, not merged: a file deleted locally must not decide
	// a gate result from a previous run.
	if err := sshRunRemote(ctx, env, spec, "rm -rf '"+dest+"' && mkdir -p '"+dest+"'"); err != nil {
		return "", fmt.Errorf("prepare %s on %s: %w", dest, spec.host, err)
	}
	if err := tarToRemote(ctx, env, spec, remote, ws.root, files, dest); err != nil {
		return "", err
	}
	if ws.rel != "" {
		return dest + "/" + ws.rel, nil
	}
	return dest, nil
}

// remoteHome asks the host where its $HOME is. The sync needs a writable
// directory whose path the host itself agrees on, and the login shell is the
// one thing every host in the fleet has.
func remoteHome(ctx context.Context, env linuxEnv, spec linuxHostSpec) (string, error) {
	argv := append(linuxSSHArgv(spec), `printf %s "$HOME"`)
	out, exit, err := env.run(ctx, argv[0], argv[1:]...)
	if err != nil {
		return "", fmt.Errorf("ask %s for $HOME: %w", spec.host, err)
	}
	home := strings.TrimSpace(strings.TrimRight(string(out), "\r\n"))
	if exit != 0 || home == "" || strings.ContainsAny(home, "\r\n'") {
		return "", fmt.Errorf("ask %s for $HOME: exited %d: %s", spec.host, exit, strings.TrimSpace(string(out)))
	}
	return strings.TrimSuffix(filepath.ToSlash(home), "/"), nil
}

func sshRunRemote(ctx context.Context, env linuxEnv, spec linuxHostSpec, command string) error {
	argv := append(linuxSSHArgv(spec), command)
	out, exit, err := env.run(ctx, argv[0], argv[1:]...)
	if err != nil {
		return err
	}
	if exit != 0 {
		return fmt.Errorf("exited %d: %s", exit, strings.TrimSpace(string(out)))
	}
	return nil
}

// tarToRemote streams the file list as a tar into the host's own tar. The
// archive is built in-process (archive/tar) rather than by shelling out, so
// the sync needs no tar on the SENDING side and sends exactly the named files.
func tarToRemote(ctx context.Context, env linuxEnv, spec linuxHostSpec, remote, root string, files []string, dest string) error {
	remoteArgv := append(linuxRemoteBashy(remote), "tar", "-xf", "-", "-C", dest)
	argv := append(linuxSSHArgv(spec), shellJoin(remoteArgv))
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	cmd.Stdout = env.stderr
	cmd.Stderr = env.stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	writeErr := writeTarStream(stdin, root, files)
	// Close before Wait either way: the remote tar only finishes at EOF.
	closeErr := stdin.Close()
	waitErr := cmd.Wait()
	switch {
	case writeErr != nil:
		return fmt.Errorf("send %s: %w", root, writeErr)
	case waitErr != nil:
		return fmt.Errorf("extract on %s: %w", spec.host, waitErr)
	case closeErr != nil:
		return fmt.Errorf("send %s: %w", root, closeErr)
	}
	return nil
}

func writeTarStream(w io.Writer, root string, files []string) error {
	tw := tar.NewWriter(w)
	for _, rel := range files {
		if err := writeTarEntry(tw, root, rel); err != nil {
			return err
		}
	}
	return tw.Close()
}

func writeTarEntry(tw *tar.Writer, root, rel string) error {
	full := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(full)
	switch {
	case errors.Is(err, os.ErrNotExist):
		// git lists an index entry for a file deleted in the worktree. The
		// tree is what is there now, so skip it rather than fail the sync.
		return nil
	case err != nil:
		return err
	}
	link := ""
	if info.Mode()&os.ModeSymlink != 0 {
		if link, err = os.Readlink(full); err != nil {
			return err
		}
	}
	hdr, err := tar.FileInfoHeader(info, link)
	if err != nil {
		return err
	}
	hdr.Name = path.Clean(rel)
	// A gate must not see a timestamp or owner from the sending peer's
	// account, so the metadata that cannot matter is dropped.
	hdr.Uid, hdr.Gid, hdr.Uname, hdr.Gname = 0, 0, "", ""
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	f, err := os.Open(full)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("copy %s: %w", rel, err)
	}
	return nil
}

// normalizeHostDir reads a directory the way the host's own shell writes it.
// bashy's Windows shell prints C:\Users\x as /c/Users/x, so `--dir` has to
// accept that form back — and wsl.exe and os/exec both need the drive form.
func normalizeHostDir(goos, dir string) string {
	d := strings.TrimSpace(dir)
	if d == "" || goos != "windows" {
		return d
	}
	// Not filepath.ToSlash: the separator that matters is the REMOTE host's,
	// and this may well run on a peer whose own separator is /.
	d = strings.ReplaceAll(d, `\`, "/")
	// /c or /c/... — one letter between slashes is a drive, never a directory
	// at the root of a Windows filesystem.
	if len(d) >= 2 && d[0] == '/' && isDriveLetter(d[1]) && (len(d) == 2 || d[2] == '/') {
		return strings.ToUpper(d[1:2]) + ":" + d[2:]
	}
	return d
}

func isDriveLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
