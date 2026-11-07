package agentos

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// fakeConverterDir is a PATH directory holding a cygpath that prints its last
// operand with a C: prefix — the fixture examples/outpost-profile uses to
// simulate a Windows host (todo 9f947a58).
func fakeConverterDir(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	body := "#!/bin/sh\nlast=\"\"; for a in \"$@\"; do last=\"$a\"; done\nprintf 'C:%s\\n' \"$last\"\n"
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// runHostConvWire runs script through the real wireExec chain with path as the
// only PATH, and returns stdout+stderr. HOME and the skills dir are redirected
// so the chain's best-effort session/audit writes stay in scratch.
func runHostConvWire(t *testing.T, posix bool, path, script string) (stdout, stderr string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BASHY_SKILLS_DIR", t.TempDir())
	t.Setenv("BASHY_HINTS", "off")
	t.Setenv("PATH", path)
	var out, errOut bytes.Buffer
	r, err := interp.New(interp.Env(nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, opt := range wireExec(nil, posix, os.Environ(), nil, &out, &errOut, false) {
		if err := opt(r); err != nil {
			t.Fatal(err)
		}
	}
	prog, perr := syntax.NewParser().Parse(strings.NewReader(script), "")
	if perr != nil {
		t.Fatal(perr)
	}
	_ = r.Run(context.Background(), prog)
	return out.String(), errOut.String()
}

// The regression: a bare `cygpath` ran bashy's own applet while `type -a` and
// `command -v` named the PATH file, so a script that probes before calling got
// two different answers. The PATH file must win, in posix mode too, and
// introspection must still agree with it.
func TestHostCygpathOnPathWinsOverApplet(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake converters are POSIX shell fixtures")
	}
	dir := fakeConverterDir(t, "cygpath", "wslpath")
	for _, posix := range []bool{false, true} {
		out, errOut := runHostConvWire(t, posix, dir, "cygpath -m /tmp/x/a\nwslpath -m /tmp/x/a\ntype -a cygpath\ncommand -v cygpath\n")
		if want := "C:/tmp/x/a\n"; !strings.HasPrefix(out, want+want) {
			t.Errorf("posix=%v: cygpath/wslpath did not run the PATH file: stdout=%q stderr=%q", posix, out, errOut)
		}
		if want := filepath.Join(dir, "cygpath"); !strings.Contains(out, want) {
			t.Errorf("posix=%v: type/command -v no longer report %q: stdout=%q", posix, want, out)
		}
		if strings.Contains(out, "builtin") || strings.Contains(out, "applet") {
			t.Errorf("posix=%v: type reported something other than the PATH file: stdout=%q", posix, out)
		}
	}
}

// With no host converter on PATH, bashy's own applet still answers: the carve
// out defers to a host file, it does not remove the portable stand-in. The
// applet's own spelling is not asserted (it is the frozen userland's), only
// that the name resolved in process and succeeded.
func TestAppletServesConverterWithoutHostFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a real cygpath may be on PATH")
	}
	out, errOut := runHostConvWire(t, false, t.TempDir(), "cygpath -m /c/Users/a\necho status=$?\n")
	if !strings.Contains(out, "status=0") {
		t.Fatalf("the applet did not serve cygpath: stdout=%q stderr=%q", out, errOut)
	}
	if strings.Contains(out, "C:/tmp") {
		t.Fatalf("unexpected fixture output: %q", out)
	}
}

// Every other userland name keeps the documented pure-Go-first precedence: a
// PATH file named ls does NOT displace the applet.
func TestOtherAppletsKeepPureGoFirst(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake ls is a POSIX shell fixture")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ls"), []byte("#!/bin/sh\necho host-ls\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, errOut := runHostConvWire(t, false, dir, "cd "+dir+" && ls\n")
	if strings.Contains(out, "host-ls") {
		t.Errorf("the PATH file displaced the ls applet: stdout=%q stderr=%q", out, errOut)
	}
}

// findHostConverter picks the first executable file on PATH and skips what is
// not one: a directory, a non-executable file, and this binary itself (a
// multicall link in a BASHY_ROOT userland layout).
func TestFindHostConverter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the execute bit and symlink cases are unix shapes")
	}
	list := func(dirs ...string) string { return strings.Join(dirs, string(os.PathListSeparator)) }

	asDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(asDir, "cygpath"), 0o755); err != nil {
		t.Fatal(err)
	}
	noExec := t.TempDir()
	if err := os.WriteFile(filepath.Join(noExec, "cygpath"), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	real := fakeConverterDir(t, "cygpath")

	if got := findHostConverter(list(asDir, noExec, real), "cygpath"); got != filepath.Join(real, "cygpath") {
		t.Errorf("findHostConverter = %q, want the executable file in %q", got, real)
	}
	if got := findHostConverter(list(asDir, noExec), "cygpath"); got != "" {
		t.Errorf("findHostConverter = %q, want \"\" (no host converter)", got)
	}
	selfDir := t.TempDir()
	self, err := os.Executable()
	if err != nil {
		t.Skip("no path to this test binary")
	}
	if err := os.Symlink(self, filepath.Join(selfDir, "cygpath")); err != nil {
		t.Fatal(err)
	}
	if got := findHostConverter(list(selfDir), "cygpath"); got != "" {
		t.Errorf("findHostConverter = %q, want \"\" (a link to this binary is not a host converter)", got)
	}
	if got := findHostConverter(list(selfDir, real), "cygpath"); got != filepath.Join(real, "cygpath") {
		t.Errorf("findHostConverter = %q, want the host file after skipping the self link", got)
	}
}
