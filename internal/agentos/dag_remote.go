package agentos

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"github.com/qiangli/outpost/pkg/sshclient"
	"github.com/qiangli/yoke/pkg/dag"
)

// extractDagHost removes the front-door -H spelling while preserving all
// ordinary dag arguments. It deliberately rejects repeated host selectors.
func extractDagHost(args []string) (host string, rest []string, found bool, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-H" || a == "--host":
			if found || i+1 == len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", nil, false, fmt.Errorf("%s requires one host", a)
			}
			host, found = strings.TrimSpace(args[i+1]), true
			i++
		case strings.HasPrefix(a, "-H=") || strings.HasPrefix(a, "--host="):
			if found {
				return "", nil, false, fmt.Errorf("host may be specified only once")
			}
			host = strings.TrimSpace(a[strings.IndexByte(a, '=')+1:])
			if host == "" {
				return "", nil, false, fmt.Errorf("%s requires a host", a[:strings.IndexByte(a, '=')])
			}
			found = true
		case strings.HasPrefix(a, "-H") && len(a) > 2:
			if found {
				return "", nil, false, fmt.Errorf("host may be specified only once")
			}
			host, found = strings.TrimSpace(a[2:]), true
			if host == "" {
				return "", nil, false, fmt.Errorf("-H requires a host")
			}
		default:
			rest = append(rest, a)
		}
	}
	return
}

// runRemoteDag mirrors the requested target closure into a stable per-project
// directory owned by Bashy, then invokes the installed Bashy DAG runner over
// the authenticated peer channel. The file list is restricted to Sources,
// the DAG description, and declared Generates fetched after the run.
func runRemoteDag(ctx context.Context, host string, args []string, stdout, stderr io.Writer) (int, error) {
	file := ""
	var targets []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--file" || a == "-f":
			if i+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a path", a)
			}
			i++
			file = args[i]
		case strings.HasPrefix(a, "--file="):
			file = strings.TrimPrefix(a, "--file=")
		case strings.HasPrefix(a, "-f="):
			file = strings.TrimPrefix(a, "-f=")
		case strings.HasPrefix(a, "-"):
			return 0, fmt.Errorf("remote dag currently does not accept option %s", a)
		case strings.Contains(a, "="):
			// Preserve make-style variable overrides by forwarding them.
		default:
			targets = append(targets, a)
		}
	}
	if len(targets) != 1 {
		return 0, fmt.Errorf("-H requires exactly one target")
	}
	if file == "" {
		discovered, err := dag.Discover(".")
		if err != nil {
			return 0, err
		}
		file = discovered
	}
	absFile, err := filepath.Abs(file)
	if err != nil {
		return 0, err
	}
	doc, err := dag.ParseFile(absFile)
	if err != nil {
		return 0, err
	}
	g, err := dag.BuildGraph(doc)
	if err != nil {
		return 0, err
	}
	sub, err := g.Subgraph(targets...)
	if err != nil {
		return 0, err
	}
	root, err := os.Getwd()
	if err != nil {
		return 0, err
	}
	projectHash := sha256.Sum256([]byte(root))
	user, err := peerSSHUser(ctx, host)
	if err != nil {
		return 0, err
	}
	channel, err := DialInstalledPeerChannel(ctx, host, user)
	if err != nil {
		return 0, err
	}
	defer channel.Close()
	remoteHome, err := peerHome(ctx, channel)
	if err != nil {
		return 0, err
	}
	workspace := filepath.ToSlash(filepath.Join(remoteHome, ".bashy", "remote", "workspaces", hex.EncodeToString(projectHash[:12])))
	if _, err := channel.Exec(ctx, sshclient.ExecOptions{Command: "mkdir -p " + shellQuote(workspace)}); err != nil {
		return 0, err
	}
	sftp, err := channel.SFTP()
	if err != nil {
		return 0, err
	}
	defer sftp.Close()
	files := map[string]bool{absFile: true}
	for _, n := range sub.Order {
		t := sub.Nodes[n].Task
		for _, source := range t.Sources {
			matches, e := filepath.Glob(filepath.Join(root, source))
			if e != nil {
				return 0, e
			}
			for _, match := range matches {
				if e := addDagSource(files, root, match); e != nil {
					return 0, e
				}
			}
		}
	}
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, local := range paths {
		rel, e := filepath.Rel(root, local)
		if e != nil {
			return 0, e
		}
		remote := filepath.ToSlash(filepath.Join(workspace, filepath.ToSlash(rel)))
		if e = syncDagFile(sftp, local, remote, rel, stderr); e != nil {
			return 0, e
		}
	}
	remoteDag := filepath.ToSlash(filepath.Join(workspace, mustRel(root, absFile)))
	argv := []string{"dag", "--file", remoteDag, targets[0]}
	for _, a := range args {
		if strings.Contains(a, "=") {
			argv = append(argv, a)
		}
	}
	quoted := make([]string, len(argv))
	for i, a := range argv {
		quoted[i] = shellQuote(a)
	}
	command := "cd " + shellQuote(workspace) + " && \"$HOME/" + remoteInstallDir + "/bashy\" " + strings.Join(quoted, " ")
	runID := fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
	logDir := path.Join(workspace, ".bashy-runs", runID)
	if err := sftp.MkdirAll(logDir); err != nil {
		return 0, err
	}
	defer sftp.RemoveAll(logDir)
	stdoutPath, stderrPath, statusPath := path.Join(logDir, "stdout"), path.Join(logDir, "stderr"), path.Join(logDir, "status")
	command = "cd " + shellQuote(workspace) + " && (" + command + " >" + shellQuote(stdoutPath) + " 2>" + shellQuote(stderrPath) + "; rc=$?; printf '%s' \"$rc\" >" + shellQuote(statusPath) + "; exit \"$rc\")"
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	remoteDone := make(chan struct {
		result *sshclient.ExecResult
		err    error
	}, 1)
	go func() {
		r, e := channel.Exec(runCtx, sshclient.ExecOptions{Command: command, MaxStdout: 4096, MaxStderr: 4096})
		remoteDone <- struct {
			result *sshclient.ExecResult
			err    error
		}{r, e}
	}()
	var stdoutOffset, stderrOffset int64
	var execResult *sshclient.ExecResult
	var execErr error
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		stdoutOffset, err = streamRemoteFile(sftp, stdoutPath, stdoutOffset, stdout)
		if err != nil && !os.IsNotExist(err) {
			return 0, err
		}
		stderrOffset, err = streamRemoteFile(sftp, stderrPath, stderrOffset, stderr)
		if err != nil && !os.IsNotExist(err) {
			return 0, err
		}
		select {
		case done := <-remoteDone:
			execResult, execErr = done.result, done.err
			// Drain the final bytes after exec completion before deciding.
			stdoutOffset, err = streamRemoteFile(sftp, stdoutPath, stdoutOffset, stdout)
			if err != nil && !os.IsNotExist(err) {
				return 0, err
			}
			stderrOffset, err = streamRemoteFile(sftp, stderrPath, stderrOffset, stderr)
			if err != nil && !os.IsNotExist(err) {
				return 0, err
			}
			goto remoteFinished
		case <-runCtx.Done():
			return 124, runCtx.Err()
		case <-ticker.C:
		}
	}

remoteFinished:
	if execErr != nil {
		return 0, execErr
	}
	if execResult == nil {
		return 0, fmt.Errorf("remote dag returned no result")
	}
	result := execResult
	if statusFile, e := sftp.Open(statusPath); e == nil {
		b, readErr := io.ReadAll(statusFile)
		_ = statusFile.Close()
		if readErr == nil {
			if code, parseErr := strconv.Atoi(strings.TrimSpace(string(b))); parseErr == nil {
				result.ExitCode = code
			}
		}
	}
	for _, n := range sub.Order {
		for _, generated := range sub.Nodes[n].Task.Generates {
			matches, e := remoteGeneratedMatches(sftp, workspace, generated)
			if e != nil {
				return result.ExitCode, e
			}
			for _, remote := range matches {
				rel, e := filepath.Rel(workspace, remote)
				if e != nil {
					return result.ExitCode, e
				}
				local := filepath.Join(root, rel)
				if e = os.MkdirAll(filepath.Dir(local), 0o755); e != nil {
					return result.ExitCode, e
				}
				rf, e := sftp.Open(filepath.ToSlash(remote))
				if e != nil {
					return result.ExitCode, e
				}
				lf, e := os.OpenFile(local, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
				if e != nil {
					_ = rf.Close()
					return result.ExitCode, e
				}
				_, e = io.Copy(lf, rf)
				info, statErr := rf.Stat()
				_ = rf.Close()
				ce := lf.Close()
				if e != nil {
					return result.ExitCode, e
				}
				if ce != nil {
					return result.ExitCode, ce
				}
				if statErr == nil {
					_ = os.Chmod(local, info.Mode().Perm())
				}
			}
		}
	}
	return result.ExitCode, nil
}

func syncDagFile(sftp *sftp.Client, local, remote, rel string, log io.Writer) error {
	if err := sftp.MkdirAll(filepath.ToSlash(filepath.Dir(remote))); err != nil {
		return err
	}
	data, err := os.ReadFile(local)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	if remoteDigest(sftp, remote) == digest {
		_, err = fmt.Fprintf(log, "dag remote: unchanged %s (%x)\n", rel, digest[:6])
		return err
	}
	f, err := sftp.Create(remote)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	_ = sftp.Chmod(remote, fileMode(local))
	_, err = fmt.Fprintf(log, "dag remote: transferred %s %d bytes sha256=%x\n", rel, len(data), digest[:6])
	return err
}

func streamRemoteFile(sftp *sftp.Client, remote string, offset int64, dst io.Writer) (int64, error) {
	info, err := sftp.Stat(remote)
	if err != nil {
		return offset, err
	}
	if info.Size() <= offset {
		return offset, nil
	}
	f, err := sftp.Open(remote)
	if err != nil {
		return offset, err
	}
	defer f.Close()
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return offset, err
	}
	n, err := io.CopyN(dst, f, info.Size()-offset)
	return offset + n, err
}

func remoteGeneratedMatches(sftp *sftp.Client, workspace, pattern string) ([]string, error) {
	var matches []string
	var walk func(string) error
	walk = func(dir string) error {
		entries, err := sftp.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			name := path.Join(dir, entry.Name())
			rel, err := filepath.Rel(workspace, filepath.FromSlash(name))
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if entry.IsDir() {
				if err := walk(name); err != nil {
					return err
				}
				continue
			}
			if strings.HasPrefix(rel, ".bashy-runs/") {
				continue
			}
			ok, err := path.Match(filepath.ToSlash(pattern), rel)
			if err != nil {
				return err
			}
			if ok {
				matches = append(matches, name)
			}
		}
		return nil
	}
	if err := walk(workspace); err != nil {
		return nil, err
	}
	return matches, nil
}

func peerSSHUser(ctx context.Context, host string) (string, error) {
	if at := strings.LastIndex(host, "@"); at > 0 {
		return host[:at], nil
	}
	// `ssh -G` only resolves the user's local SSH config; it opens no network
	// connection and does not depend on sshd. The peer channel remains the sole
	// transport for remote operations after bootstrap.
	cmd := exec.CommandContext(ctx, "ssh", "-G", host)
	b, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve SSH user for peer %s: %w", host, err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "user" && fields[1] != "" {
			return fields[1], nil
		}
	}
	return "", fmt.Errorf("SSH configuration has no peer user for %s", host)
}

func peerHome(ctx context.Context, c *PeerChannel) (string, error) {
	r, err := c.Exec(ctx, sshclient.ExecOptions{Command: "printf %s \"$HOME\""})
	if err != nil {
		return "", err
	}
	if r.ExitCode != 0 || len(r.Stdout) == 0 {
		return "", fmt.Errorf("peer did not report remote home")
	}
	return strings.TrimSpace(string(r.Stdout)), nil
}

func addDagSource(files map[string]bool, root, path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("source %s is a symlink; remote DAG Sources must name regular workspace files", path)
	}
	if info.IsDir() {
		return filepath.WalkDir(path, func(p string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				return nil
			}
			return addDagSource(files, root, p)
		})
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("source %s is outside the project workspace", path)
	}
	files[path] = true
	return nil
}

func remoteDigest(sftp *sftp.Client, path string) [32]byte {
	var zero [32]byte
	f, err := sftp.Open(path)
	if err != nil {
		return zero
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return zero
	}
	return sha256.Sum256(b)
}

func fileMode(path string) fs.FileMode {
	i, err := os.Stat(path)
	if err != nil {
		return 0o644
	}
	return i.Mode().Perm()
}
func mustRel(root, path string) string { r, _ := filepath.Rel(root, path); return filepath.ToSlash(r) }
