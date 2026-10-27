package agentos

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The mailbox store must follow the same isolation ladder as the other
// bashy stores (BASHY_MAILBOX_DIR > BASHY_HOME > HOME), never the passwd
// home of the OS user: a guarded run that points BASHY_HOME and HOME at
// temp dirs must not create or chmod the operator's real ~/.bashy/inbox.
// The resolver runs in a child test process so the parent's environment
// (and any inherited BASHY_MAILBOX_DIR) cannot leak into the verdict.
func TestMailboxDirHelperProcess(t *testing.T) {
	if os.Getenv("BASHY_MAILBOX_DIR_HELPER") != "1" {
		t.Skip("helper process")
	}
	d, err := mailboxDir()
	if err != nil {
		os.Stdout.WriteString("ERR " + err.Error() + "\n")
		os.Exit(3)
	}
	os.Stdout.WriteString("DIR " + d + "\n")
	os.Exit(0)
}

func runMailboxDirHelper(t *testing.T, home, bashyHome string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestMailboxDirHelperProcess$")
	var env []string
	for _, kv := range os.Environ() {
		switch strings.SplitN(kv, "=", 2)[0] {
		case "HOME", "BASHY_HOME", "BASHY_MAILBOX_DIR", "USERPROFILE":
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "BASHY_MAILBOX_DIR_HELPER=1", "HOME="+home, "USERPROFILE="+home)
	if bashyHome != "" {
		env = append(env, "BASHY_HOME="+bashyHome)
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helper: %v\n%s", err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "DIR ") {
			return strings.TrimPrefix(line, "DIR ")
		}
	}
	t.Fatalf("helper printed no DIR:\n%s", out)
	return ""
}

func TestMailboxDirFollowsBashyHomeThenIsolatedHome(t *testing.T) {
	home := t.TempDir()
	bashyHome := t.TempDir()
	// Decoy: an isolated-HOME inbox with a distinct mode. With BASHY_HOME
	// set it must be neither chosen nor chmodded.
	decoy := filepath.Join(home, ".bashy", "inbox")
	if err := os.MkdirAll(decoy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(decoy, 0o755); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(bashyHome, "inbox")
	if got := runMailboxDirHelper(t, home, bashyHome); got != want {
		t.Fatalf("with BASHY_HOME: mailboxDir=%q, want %q", got, want)
	}
	if fi, err := os.Stat(want); err != nil || !permIs(fi, 0o700) {
		t.Fatalf("BASHY_HOME inbox not created 0700: %v %v", fi, err)
	}
	if fi, err := os.Stat(decoy); err != nil || !permIs(fi, 0o755) {
		t.Fatalf("decoy HOME inbox touched: %v %v", fi, err)
	}

	home2 := t.TempDir()
	want2 := filepath.Join(home2, ".bashy", "inbox")
	if got := runMailboxDirHelper(t, home2, ""); got != want2 {
		t.Fatalf("with HOME only: mailboxDir=%q, want %q", got, want2)
	}
	if fi, err := os.Stat(want2); err != nil || !permIs(fi, 0o700) {
		t.Fatalf("HOME inbox not created 0700: %v %v", fi, err)
	}
}

// permIs compares unix mode bits; Windows has none, so any existing entry passes.
func permIs(fi os.FileInfo, want os.FileMode) bool {
	return runtime.GOOS == "windows" || fi.Mode().Perm() == want
}
