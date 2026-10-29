package agentos

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeHostDir(t *testing.T) {
	cases := []struct {
		goos, in, want string
	}{
		// bashy's own Windows shell prints paths this way, so --dir has to
		// read them back.
		{"windows", "/c/Users/liqia/.bashy/linux/workspaces/cloudbox-1234abcd", "C:/Users/liqia/.bashy/linux/workspaces/cloudbox-1234abcd"},
		{"windows", "/c", "C:"},
		{"windows", "/c/", "C:/"},
		{"windows", `C:\Users\liqia`, "C:/Users/liqia"},
		{"windows", "D:/src", "D:/src"},
		// Not a drive: more than one character before the next slash.
		{"windows", "/usr/local", "/usr/local"},
		{"windows", "", ""},
		// A real Linux host's /c IS a directory, so nothing is rewritten.
		{"linux", "/c/Users/liqia", "/c/Users/liqia"},
		{"darwin", "/Users/qiangli/src", "/Users/qiangli/src"},
	}
	for _, c := range cases {
		if got := normalizeHostDir(c.goos, c.in); got != c.want {
			t.Errorf("normalizeHostDir(%q, %q) = %q, want %q", c.goos, c.in, got, c.want)
		}
	}
}

func TestLinuxRemoteBashyIsACommandNotJustAPath(t *testing.T) {
	if got := linuxRemoteBashy(""); strings.Join(got, " ") != "bashy" {
		t.Errorf("default = %v, want [bashy]", got)
	}
	// The bootstrap case: a host whose PATH bashy predates this verb, pointed
	// at a pushed build with the env its toolchain step needs.
	got := linuxRemoteBashy("env BASHY_SCRATCH_BIN=/c/b/linux /c/b/bashy.exe")
	want := "env BASHY_SCRATCH_BIN=/c/b/linux /c/b/bashy.exe"
	if strings.Join(got, " ") != want {
		t.Errorf("= %v, want %q", got, want)
	}
}

func TestRunLinuxHostHopUsesRemoteBashyAndDir(t *testing.T) {
	env, _, _ := fakeEnv(t, "darwin", newFakeHost())
	var got []string
	code := runLinux(context.Background(), env,
		[]string{"--host", "liqiang@elephant.local:2222",
			"--remote-bashy", "env BASHY_SCRATCH_BIN=/c/b/linux /c/b/bashy.exe",
			"--dir", "/c/Users/liqia/src/cloudbox/hub",
			"--", "go", "test", "-short", "./..."},
		func(_ context.Context, _ linuxEnv, argv []string) int { got = argv; return 0 })
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := "ssh -p 2222 liqiang@elephant.local env BASHY_SCRATCH_BIN=/c/b/linux /c/b/bashy.exe linux " +
		"--dir /c/Users/liqia/src/cloudbox/hub run -- go test -short ./..."
	if strings.Join(got, " ") != want {
		t.Errorf("host argv =\n%s\nwant\n%s", strings.Join(got, " "), want)
	}
}

// --dir replaces the cwd outright, so the /mnt translation, the bind mount and
// the child's own directory cannot disagree.
func TestRunLinuxDirReplacesTheWorkingDirectory(t *testing.T) {
	h := elephantHost()
	h.reply("wsl.exe --status", utf16le("Default Version: 2\r\n"), 0, nil)
	h.reply("wsl.exe --list --quiet", utf16le("Ubuntu-24.04\r\n"), 0, nil)
	h.reply("wsl.exe -d Ubuntu-24.04 -u root -- sh -lc command -v go", []byte("/usr/local/bin/go\n"), 0, nil)
	h.reply("dism.exe /online /Get-FeatureInfo /FeatureName:VirtualMachinePlatform", []byte("State : Enabled\r\n"), 0, nil)
	h.reply("dism.exe /online /Get-FeatureInfo /FeatureName:Microsoft-Windows-Subsystem-Linux", []byte("State : Enabled\r\n"), 0, nil)
	env, _, _ := fakeEnv(t, "windows", h, "wsl.exe")

	var got []string
	code := runLinux(context.Background(), env,
		[]string{"--dir", "/c/Users/liqia/.bashy/linux/workspaces/cloudbox-aa/hub", "--", "go", "test", "-short", "./..."},
		func(_ context.Context, e linuxEnv, argv []string) int { got = argv; return 0 })
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := "wsl.exe -d Ubuntu-24.04 -u root --cd /mnt/c/Users/liqia/.bashy/linux/workspaces/cloudbox-aa/hub -- go test -short ./..."
	if strings.Join(got, " ") != want {
		t.Errorf("argv =\n%s\nwant\n%s", strings.Join(got, " "), want)
	}
}

func TestParseLinuxArgsSyncNeedsAHost(t *testing.T) {
	if _, err := parseLinuxArgs([]string{"--sync", "--", "true"}); err == nil ||
		!strings.Contains(err.Error(), "--sync needs --host") {
		t.Errorf("--sync without --host: err = %v", err)
	}
	if _, err := parseLinuxArgs([]string{"--remote-bashy", "/x/bashy", "status"}); err == nil ||
		!strings.Contains(err.Error(), "--remote-bashy needs --host") {
		t.Errorf("--remote-bashy without --host: err = %v", err)
	}
	// --sync picks the directory, so naming one too is a contradiction and
	// must not be resolved silently.
	if _, err := parseLinuxArgs([]string{"--host", "h", "--sync", "--dir", "/x", "--", "true"}); err == nil ||
		!strings.Contains(err.Error(), "cannot be combined with --dir") {
		t.Errorf("--sync with --dir: err = %v", err)
	}
	o, err := parseLinuxArgs([]string{"--host", "h", "--sync", "--", "go", "test"})
	if err != nil || !o.sync || o.sub != "run" {
		t.Fatalf("o = %+v, err = %v", o, err)
	}
}

// ------------------------------------------------------------ the file list

func TestLocalLinuxWorkspaceUsesTheWorktreeRoot(t *testing.T) {
	h := newFakeHost().reply("git -C /src/cloudbox/hub rev-parse --show-toplevel",
		[]byte("/src/cloudbox\n"), 0, nil)
	env, _, _ := fakeEnv(t, "linux", h)
	env.cwd = "/src/cloudbox/hub"

	ws, err := localLinuxWorkspace(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	// The root is the whole worktree because hub/'s own gate resolves through
	// a go.work above it; syncing hub/ alone cannot build.
	if ws.root != "/src/cloudbox" || ws.rel != "hub" {
		t.Errorf("root=%q rel=%q, want /src/cloudbox and hub", ws.root, ws.rel)
	}
	if !strings.HasPrefix(ws.name, "cloudbox-") || len(ws.name) != len("cloudbox-")+8 {
		t.Errorf("name = %q, want cloudbox-<8 hex>", ws.name)
	}
	// The digest is of the absolute path, so two same-named checkouts on one
	// peer land in different directories.
	env.cwd = "/other/cloudbox"
	h.reply("git -C /other/cloudbox rev-parse --show-toplevel", []byte("/other/cloudbox\n"), 0, nil)
	other, err := localLinuxWorkspace(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if other.name == ws.name {
		t.Errorf("two checkouts named %q collide on the host", ws.name)
	}
	if other.rel != "" {
		t.Errorf("rel = %q, want empty when the cwd IS the root", other.rel)
	}
}

func TestLinuxSyncFilesIncludesSubmodulesAndSkipsIgnored(t *testing.T) {
	h := newFakeHost()
	h.reply("git -C /src/cloudbox rev-parse --show-toplevel", []byte("/src/cloudbox\n"), 0, nil)
	// hub/bin is gitignored, so --exclude-standard never names it: that is
	// what keeps 810 MB of build output out of a 17 MB transfer.
	h.reply("git -C /src/cloudbox ls-files -z --cached --others --exclude-standard",
		[]byte("go.work\x00hub/go.mod\x00docs/todo/new.md\x00"), 0, nil)
	h.reply("git -C /src/cloudbox submodule --quiet foreach --recursive echo $displaypath",
		[]byte("external/appstore\nexternal/k3s\n"), 0, nil)
	h.reply("git -C "+filepath.FromSlash("/src/cloudbox/external/k3s")+" ls-files -z --cached --others --exclude-standard",
		[]byte("go.mod\x00pkg/server.go\x00"), 0, nil)
	h.reply("git -C "+filepath.FromSlash("/src/cloudbox/external/appstore")+" ls-files -z --cached --others --exclude-standard",
		[]byte("app.json\x00"), 0, nil)
	env, _, _ := fakeEnv(t, "linux", h)

	files, err := linuxSyncFiles(context.Background(), env, "/src/cloudbox")
	if err != nil {
		t.Fatal(err)
	}
	// go.work `use ./external/k3s` needs the submodule's own files, which the
	// parent's ls-files never names (a gitlink has no contents).
	for _, want := range []string{"go.work", "hub/go.mod", "docs/todo/new.md",
		"external/k3s/go.mod", "external/k3s/pkg/server.go", "external/appstore/app.json"} {
		if !sliceHas(files, want) {
			t.Errorf("file list is missing %q: %v", want, files)
		}
	}
	for _, never := range []string{"hub/bin", "hub/bin/cloudbox", ".git"} {
		if sliceHas(files, never) {
			t.Errorf("file list carries %q", never)
		}
	}
}

func TestLinuxSyncFilesSendsEverythingOutsideAGitWorktree(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	mustWrite(t, filepath.Join(root, "sub", "b.txt"), "b")
	// No git signal here, so there is nothing to tell build output apart from
	// sources, and everything goes — except the checkout metadata.
	mustWrite(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main")

	h := newFakeHost() // every git call fails: not a worktree
	env, _, _ := fakeEnv(t, "linux", h)
	files, err := linuxSyncFiles(context.Background(), env, root)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(files, " ") != "a.txt sub/b.txt" {
		t.Errorf("files = %v, want [a.txt sub/b.txt]", files)
	}
}

// ---------------------------------------------------------------- the stream

func TestWriteTarStreamSendsExactlyTheNamedFiles(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.work"), "use ./hub\n")
	mustWrite(t, filepath.Join(root, "hub", "go.mod"), "module hub\n")
	mustWrite(t, filepath.Join(root, "hub", "bin", "huge"), strings.Repeat("x", 4096))
	if err := os.Chmod(filepath.Join(root, "hub", "go.mod"), 0o755); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	// "hub/gone" is in the list but not on disk: git names an index entry for
	// a file deleted in the worktree, and the tree is what is there now.
	if err := writeTarStream(&buf, root, []string{"go.work", "hub/go.mod", "hub/gone"}); err != nil {
		t.Fatal(err)
	}

	got := map[string]string{}
	modes := map[string]os.FileMode{}
	tr := tar.NewReader(&buf)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		got[hdr.Name] = string(body)
		modes[hdr.Name] = hdr.FileInfo().Mode()
		if hdr.Uid != 0 || hdr.Gid != 0 || hdr.Uname != "" || hdr.Gname != "" {
			t.Errorf("%s carries the sending peer's ownership (%d/%d %q/%q)",
				hdr.Name, hdr.Uid, hdr.Gid, hdr.Uname, hdr.Gname)
		}
	}
	if len(got) != 2 {
		t.Fatalf("entries = %v, want go.work and hub/go.mod only", got)
	}
	if got["go.work"] != "use ./hub\n" || got["hub/go.mod"] != "module hub\n" {
		t.Errorf("contents = %v", got)
	}
	// Windows accepts Chmod but does not expose Unix execute bits in Stat.
	// Compare against the mode the sending filesystem actually reports.
	source, err := os.Stat(filepath.Join(root, "hub", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := modes["hub/go.mod"].Perm(), source.Mode().Perm(); got != want {
		t.Errorf("hub/go.mod mode = %v, want source mode %v", got, want)
	}
	if _, bad := got["hub/bin/huge"]; bad {
		t.Error("an unnamed file was sent")
	}
}

func TestSyncLinuxWorkspaceRefusesWithNothingToSend(t *testing.T) {
	root := t.TempDir()
	h := newFakeHost()
	env, _, _ := fakeEnv(t, "linux", h)
	env.cwd = root
	if _, err := syncLinuxWorkspace(context.Background(), env,
		linuxHostSpec{host: "elephant.local"}, ""); err == nil ||
		!strings.Contains(err.Error(), "nothing to sync") {
		t.Errorf("err = %v, want a refusal", err)
	}
	// Nothing may be removed on the host before there is something to put
	// there.
	if h.ran("ssh") {
		t.Errorf("the host was touched for an empty sync: %v", h.calls)
	}
}

func TestRemoteHomeRejectsAnUnusableAnswer(t *testing.T) {
	env, _, _ := fakeEnv(t, "darwin", newFakeHost().
		reply("ssh elephant.local", []byte("/c/Users/liqia\r\n"), 0, nil))
	home, err := remoteHome(context.Background(), env, linuxHostSpec{host: "elephant.local"})
	if err != nil || home != "/c/Users/liqia" {
		t.Errorf("home = %q, err = %v", home, err)
	}
	// A quote in the answer would escape the single-quoting of the rm -rf
	// below it, so it is refused rather than interpolated.
	env, _, _ = fakeEnv(t, "darwin", newFakeHost().
		reply("ssh elephant.local", []byte("/c/Users/o'brien\n"), 0, nil))
	if _, err := remoteHome(context.Background(), env, linuxHostSpec{host: "elephant.local"}); err == nil {
		t.Error("a quoted $HOME was accepted")
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ssh has no argv: it concatenates its trailing arguments and the remote LOGIN
// SHELL re-parses them. An unquoted hop therefore loses the command's own
// quoting — on elephant 2026-10-28 `run -- sh -lc 'uname -a; free -g'` ran
// `free -g` on the Windows host instead of inside the Linux environment.
func TestHostHopQuotesTheRemoteCommand(t *testing.T) {
	env, _, _ := fakeEnv(t, "darwin", newFakeHost())
	var got []string
	code := runLinux(context.Background(), env,
		[]string{"--host", "elephant.local", "--", "sh", "-lc", "uname -a; free -g"},
		func(_ context.Context, _ linuxEnv, argv []string) int { got = argv; return 0 })
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	// Everything after the target is ONE word, so the remote shell hands the
	// whole thing to bashy rather than running half of it itself.
	if len(got) != 3 || got[0] != "ssh" || got[1] != "elephant.local" {
		t.Fatalf("argv = %v, want ssh + target + one command word", got)
	}
	want := `bashy linux run -- sh -lc 'uname -a; free -g'`
	if got[2] != want {
		t.Errorf("remote command =\n%s\nwant\n%s", got[2], want)
	}
}

func TestShellJoinLeavesSafeWordsBareAndEscapesQuotes(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"go", "test", "-short", "./..."}, "go test -short ./..."},
		{[]string{"env", "BASHY_SCRATCH_BIN=C:/b/linux", "/c/b/bashy.exe"},
			"env BASHY_SCRATCH_BIN=C:/b/linux /c/b/bashy.exe"},
		{[]string{"sh", "-lc", "a; b"}, "sh -lc 'a; b'"},
		{[]string{"echo", "it's"}, `echo 'it'\''s'`},
		{[]string{"echo", ""}, "echo ''"},
		{[]string{"echo", "a b"}, "echo 'a b'"},
		{[]string{"echo", "$HOME"}, "echo '$HOME'"},
		{[]string{"echo", "*"}, "echo '*'"},
	}
	for _, c := range cases {
		if got := shellJoin(c.in); got != c.want {
			t.Errorf("shellJoin(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The toolchain step must leave `go` ON PATH inside the distro: a gate is
// written `go test ./...`, and `command -v go` is this very step's own Done
// predicate. Without the link `up` reported "wsl is set up" while `status`
// still said "1 step(s) left: toolchain" (elephant, 2026-10-28) — so the plan
// never converged and nothing was idempotent.
func TestToolchainPutsTheManagedGoOnPath(t *testing.T) {
	h := newFakeHost()
	h.reply("wsl.exe -d Ubuntu-24.04 -u root -- "+linuxDistroBashy+" go env GOROOT",
		[]byte("/root/.cache/bashy/bin/go/1.27.2/go\n"), 0, nil)
	h.reply("wsl.exe -d Ubuntu-24.04 -u root -- ln -sfn", nil, 0, nil)
	env, _, _ := fakeEnv(t, "windows", h, "wsl.exe")

	if err := linkDistroGo(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	want := "wsl.exe -d Ubuntu-24.04 -u root -- ln -sfn /root/.cache/bashy/bin/go/1.27.2/go/bin/go /usr/local/bin/go"
	if !h.ran(want) {
		t.Errorf("the managed go was not linked onto PATH; calls:\n%v", h.calls)
	}
	// A GOROOT that could not be read is a failure, not a silent skip: the
	// step would otherwise report success and leave the plan unconverged.
	h2 := newFakeHost()
	h2.reply("wsl.exe -d Ubuntu-24.04 -u root -- "+linuxDistroBashy+" go env GOROOT", nil, 1, nil)
	env2, _, _ := fakeEnv(t, "windows", h2, "wsl.exe")
	if err := linkDistroGo(context.Background(), env2); err == nil {
		t.Error("an unreadable GOROOT was reported as success")
	}
}

// The WSL component comes from Microsoft's MSI because the inbox stub cannot
// install itself: every route through it (--update, --install, --install
// --no-launch, --install --no-distribution) answered "The Windows Subsystem
// for Linux is not installed. You can install by running 'wsl.exe --install'."
// and exited 1 on elephant, Windows 11 build 26300, 2026-10-28.
func TestWSLComponentStepUsesTheMSINotTheStub(t *testing.T) {
	h := elephantHost()
	h.reply("dism.exe /online /Get-FeatureInfo /FeatureName:VirtualMachinePlatform", []byte("State : Enabled\r\n"), 0, nil)
	h.reply("dism.exe /online /Get-FeatureInfo /FeatureName:Microsoft-Windows-Subsystem-Linux", []byte("State : Enabled\r\n"), 0, nil)
	env, _, _ := fakeEnv(t, "windows", h, "wsl.exe")

	plan := planLinuxRunner(env, probeLinuxHost(context.Background(), env), "")
	pending := plan.pending()
	if len(pending) == 0 || pending[0].ID != "wsl-component" {
		t.Fatalf("first pending step = %v", pending)
	}
	got := strings.Join(pending[0].Argv, " ")
	if !strings.HasPrefix(got, "msiexec.exe /i ") || !strings.Contains(got, "/quiet") {
		t.Errorf("component step = %q, want a quiet msiexec install", got)
	}
	// /norestart: the optional features already took their restart, and a
	// second one here would be outside what `up` was authorized to do.
	if !sliceHas(pending[0].Argv, "/norestart") {
		t.Errorf("component step may restart the host: %q", got)
	}
	if pending[0].Reboot || pending[0].Restarts {
		t.Error("the component step must not claim a reboot it does not take")
	}
	if !pending[0].Privileged {
		t.Error("installing the component needs administrator rights")
	}
	for _, s := range plan.Steps {
		for _, a := range s.Argv {
			if a == "--update" || a == "--no-distribution" {
				t.Errorf("step %q still routes the component install through the stub (%q)", s.ID, a)
			}
		}
	}
}

func TestWSLMSIAssetMatchPicksThisArchitecture(t *testing.T) {
	// One release carries x64 and arm64 MSIs plus an msixbundle and a nupkg;
	// both MSIs end in ".msi", so the arch token has to be in the suffix.
	assets := []string{
		"Microsoft.WSL.Containers.3.0.3.nupkg",
		"Microsoft.WSL_3.0.3.0_x64_ARM64.msixbundle",
		"wsl.3.0.3.0.arm64.msi",
		"wsl.3.0.3.0.x64.msi",
	}
	for goarch, want := range map[string]string{"amd64": "wsl.3.0.3.0.x64.msi", "arm64": "wsl.3.0.3.0.arm64.msi"} {
		match := wslMSIAssetMatch(goarch)
		var got []string
		for _, a := range assets {
			if match(a, "windows", goarch) {
				got = append(got, a)
			}
		}
		if len(got) != 1 || got[0] != want {
			t.Errorf("%s matched %v, want exactly [%s]", goarch, got, want)
		}
	}
}

// On macOS /tmp IS a symlink to /private/tmp, so an unresolved cwd and a git
// root resolved by git disagree: /tmp/cb-dev/hub against /private/tmp/cb-dev
// yielded the relative path ../../../tmp/cb-dev/hub and a chdir off the end of
// the synced tree on elephant (2026-10-28).
func TestLocalLinuxWorkspaceResolvesSymlinkedRoots(t *testing.T) {
	real := t.TempDir()
	if err := os.MkdirAll(filepath.Join(real, "cb", "hub"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	resolvedRoot := resolveDir(filepath.Join(real, "cb"))

	h := newFakeHost()
	// git answers with the REAL path, as it does on a Mac.
	h.reply("git -C "+resolveDir(filepath.Join(link, "cb", "hub"))+" rev-parse --show-toplevel",
		[]byte(resolvedRoot+"\n"), 0, nil)
	env, _, _ := fakeEnv(t, "darwin", h)
	env.cwd = filepath.Join(link, "cb", "hub") // the aliased path

	ws, err := localLinuxWorkspace(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if ws.rel != "hub" {
		t.Errorf("rel = %q, want hub (a .. here chdirs off the end of the synced tree)", ws.rel)
	}
	if ws.root != resolvedRoot {
		t.Errorf("root = %q, want %q", ws.root, resolvedRoot)
	}
}

// A cwd genuinely outside the root it was given is an error, not a path with
// .. in it: the remote has no such directory and the gate would run nowhere.
func TestLocalLinuxWorkspaceRefusesACwdOutsideItsRoot(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	h := newFakeHost().reply("git -C "+resolveDir(other)+" rev-parse --show-toplevel",
		[]byte(resolveDir(root)+"\n"), 0, nil)
	env, _, _ := fakeEnv(t, "linux", h)
	env.cwd = other
	if _, err := localLinuxWorkspace(context.Background(), env); err == nil ||
		!strings.Contains(err.Error(), "not inside its own workspace root") {
		t.Errorf("err = %v, want a refusal", err)
	}
}
