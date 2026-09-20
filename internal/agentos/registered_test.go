package agentos

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	"github.com/qiangli/yoke/pkg/fleet"
)

// ringDir points the registered ring at a fresh scratch directory for one
// test and drops the process cache, so records written here are the whole
// ring. TestMain already isolates the package; this narrows it per test.
func ringDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "commands")
	t.Setenv("BASHY_COMMANDS_DIR", dir)
	t.Setenv("BASHY_COMMANDS_PATH", "")
	t.Setenv("VSC_PROFILE", "")
	resetRegisteredIndex()
	t.Cleanup(resetRegisteredIndex)
	return dir
}

// shPath is an executable every unix test host has.
func shPath(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on this host")
	}
	return p
}

func writeRecord(t *testing.T, rec fleet.Command) {
	t.Helper()
	if err := registeredCatalog().SaveCommand(rec); err != nil {
		t.Fatal(err)
	}
	resetRegisteredIndex()
}

// The collision filter names the HOLDER: a registered command may shadow a
// PATH program, never anything bashy ships.
func TestReservedCommandNameNamesTheHolder(t *testing.T) {
	cases := map[string]string{
		"ls":     "applet",
		"cd":     "bash builtin",
		"weave":  "yoke command",
		"doctl":  "bin-managed external",
		"docker": "yoke command",
		"add":    "keeps for itself",
		"bashy":  "embedded skill",
		"serve":  "yoke command",
	}
	for name, want := range cases {
		holder, taken := reservedCommandName(name)
		if !taken || !strings.Contains(holder, want) {
			t.Errorf("%s: (%q, %v), want a holder containing %q", name, holder, taken, want)
		}
	}
	for _, free := range []string{"", "gl", "my-tool", "witr"} {
		if holder, taken := reservedCommandName(free); taken {
			t.Errorf("%q must be free, got %q", free, holder)
		}
	}
	// Refusal is enforced at save, with no --force.
	ringDir(t)
	err := registeredCatalog().SaveCommand(fleet.Command{Name: "ls", Exec: []string{"/bin/sh"}})
	if err == nil || !strings.Contains(err.Error(), "applet") {
		t.Errorf("add ls: %v", err)
	}
}

// The index: a record resolves by name and alias, a shadowed ring entry is
// reported and never resolved, a certification run sees nothing.
func TestRegisteredIndexResolvesAndSkipsShadowed(t *testing.T) {
	dir := ringDir(t)
	writeRecord(t, fleet.Command{Name: "gl", Aliases: []string{"glog"}, Script: "echo gl", Effects: []string{"pure"}})
	// A record a newer bashy now ships a command for: written by hand, as an
	// older bashy would have accepted it.
	if err := os.WriteFile(filepath.Join(dir, "cat.yaml"), []byte("name: cat\nkind: command\nexec: [/bin/sh]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resetRegisteredIndex()
	for _, n := range []string{"gl", "glog"} {
		if r, ok := registeredLookup(n); !ok || r.Name != "gl" {
			t.Errorf("%s: (%+v, %v)", n, r, ok)
		}
	}
	if _, ok := registeredLookup("cat"); ok {
		t.Error("a shadowed ring entry must not resolve")
	}
	if got := registeredShadowed(); !strings.Contains(got["cat"], "applet") {
		t.Errorf("shadowed = %v", got)
	}
	if got := registeredNames(); len(got) != 1 || got[0] != "gl" {
		t.Errorf("names = %v", got)
	}
	t.Setenv("VSC_PROFILE", "cert")
	if _, ok := registeredLookup("gl"); ok {
		t.Error("a certification run must never see the ring")
	}
	if registeredNames() != nil {
		t.Error("a certification run must list nothing")
	}
}

func runRegisteredSource(t *testing.T, source string, posix bool) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	opts := []interp.RunnerOption{
		interp.Lang(syntax.LangBash),
		interp.Env(expand.ListEnviron("PATH=" + os.Getenv("PATH"))),
		interp.Dir(t.TempDir()),
	}
	opts = WireExec(opts, posix, os.Environ(), strings.NewReader(""), &out, &errOut)
	r, err := interp.New(opts...)
	if err != nil {
		t.Fatal(err)
	}
	file, err := syntax.NewParser().Parse(strings.NewReader(source), "registered-test")
	if err != nil {
		t.Fatal(err)
	}
	err = r.Run(context.Background(), file)
	return out.String(), errOut.String(), err
}

// The rung: an exec record splices args, env/cwd land on the child, and
// --posix resolves the same ring. A builtin or applet always wins over a
// same-named record; an unknown name still falls through to PATH; a
// certification run never sees the ring. (Script records re-enter bashy and
// are exercised against a built binary in registered_e2e_test.go.)
func TestRegisteredHandlerDispatchesInBothModes(t *testing.T) {
	sh := shPath(t)
	ringDir(t)
	work := t.TempDir()
	writeRecord(t, fleet.Command{Name: "regsay", Exec: []string{sh, "-c", `printf '%s|%s\n' "$1" "$2"`, "regsay", fleet.ArgsToken}})
	writeRecord(t, fleet.Command{Name: "where", Exec: []string{sh, "-c", `printf '%s\n' "$PWD"`}, Cwd: work})
	writeRecord(t, fleet.Command{Name: "envx", Exec: []string{sh, "-c", `printf '%s\n' "$REG_X"`}, Env: []string{"REG_X=from-record"}})

	for _, posix := range []bool{false, true} {
		out, errOut, err := runRegisteredSource(t, "regsay a b\n", posix)
		if err != nil || strings.TrimSpace(out) != "a|b" {
			t.Errorf("posix=%v regsay: out=%q err=%v stderr=%q", posix, out, err, errOut)
		}
		out, errOut, err = runRegisteredSource(t, "where\n", posix)
		if err != nil || filepath.Base(strings.TrimSpace(out)) != filepath.Base(work) {
			t.Errorf("posix=%v where: out=%q (want …/%s) err=%v stderr=%q", posix, out, filepath.Base(work), err, errOut)
		}
		out, _, err = runRegisteredSource(t, "envx\n", posix)
		if err != nil || strings.TrimSpace(out) != "from-record" {
			t.Errorf("posix=%v envx: out=%q err=%v", posix, out, err)
		}
	}
	out, _, _ := runRegisteredSource(t, "printf '%s' ok\n", false)
	if out != "ok" {
		t.Errorf("printf = %q", out)
	}
	_, _, err := runRegisteredSource(t, "no-such-registered-cmd\n", false)
	if st, ok := interp.IsExitStatus(err); !ok || st != 127 {
		t.Errorf("unknown name: %v", err)
	}
	t.Setenv("VSC_PROFILE", "cert")
	_, _, err = runRegisteredSource(t, "regsay a\n", true)
	if st, ok := interp.IsExitStatus(err); !ok || st != 127 {
		t.Errorf("cert profile: %v (want 127)", err)
	}
}

// agentic: a registered name takes the native path (re-enter the shell,
// yield on failure); a bin-managed external takes the front door; an
// unregistered name stays a raw external child.
func TestAgenticClassifiesRegisteredAsNative(t *testing.T) {
	ringDir(t)
	writeRecord(t, fleet.Command{Name: "regx", Exec: []string{"/bin/sh", "-c", "exit 3"}})
	cmd, _, yield := agenticCommand([]string{"regx", "a"})
	if !yield || cmd.Path != bashySelfPath() || len(cmd.Args) < 3 || cmd.Args[1] != "-c" {
		t.Errorf("registered: yield=%v argv=%v", yield, cmd.Args)
	}
	cmd, _, yield = agenticCommand([]string{"doctl", "--help"})
	if !yield || cmd.Path != bashySelfPath() || cmd.Args[1] != "doctl" {
		t.Errorf("bin-managed external: yield=%v argv=%v", yield, cmd.Args)
	}
	cmd, _, yield = agenticCommand([]string{"/bin/sh", "-c", "exit 7"})
	if yield || cmd.Path == bashySelfPath() {
		t.Errorf("unregistered external: yield=%v path=%v", yield, cmd.Path)
	}
}

// dry-run resolution and the front-door predicate both know the ring.
func TestRegisteredKnownToDryRunAndFrontDoor(t *testing.T) {
	ringDir(t)
	writeRecord(t, fleet.Command{Name: "regy", Script: "true", Effects: []string{"pure"}})
	if got, ok := resolveCmd("", "regy", nil); !ok || got != "registered:regy" {
		t.Errorf("resolveCmd = %q %v", got, ok)
	}
	if !isFrontDoorInvocation("regy") {
		t.Error("front door must recognise a registered name")
	}
}

// A record written AFTER the index loaded resolves on the next lookup miss
// and a removed one stops resolving on the next lookup — no time window;
// the tour's 07-commands chapter: add, use, rm, in one shell.
func TestRegisteredIndexSeesAddAndRemoveInTheSameShell(t *testing.T) {
	dir := ringDir(t)
	resetRegisteredIndex()
	if _, ok := registeredLookup("shout"); ok {
		t.Fatal("empty ring resolved shout")
	}
	time.Sleep(20 * time.Millisecond) // a distinct dir mtime on coarse filesystems
	writeRecord(t, fleet.Command{Name: "shout", Script: "echo SHOUT", Effects: []string{"pure"}})
	if _, ok := registeredLookup("shout"); !ok {
		t.Fatal("a record added after the index loaded must resolve on the next miss")
	}
	if err := os.Remove(filepath.Join(dir, "shout.yaml")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, ok := registeredLookup("shout"); ok {
		t.Fatal("a removed record must stop resolving on the next lookup")
	}
}
