package agentos

import (
	"bytes"
	"strings"
	"testing"
)

// Sprint 413, stories 4a18c41ef997 (#1811) and 491a03436cbb (#1834): the
// shipped `bashy sprint` dispatch mapped every Execute() error to a bare
// exit 1 and printed nothing. yoke's weave tree sets SilenceErrors on every
// sprint command (so a subverb's own envelope is never double-printed) and
// installs three reporters at tree-construction time to compensate —
// flagerr/argerr/runerr. bashy then EDITS that finished tree: attachSprintWatch
// replaces start/take's RunE, and inbox-ack / monitor / wait are added after
// the reporters walked it. Errors from those bashy-owned paths are therefore
// reported by nobody, and `--watch` / `inbox-ack` exited 1 with zero bytes on
// both streams in an external manager harness.
//
// These drive the exact dispatch shape bashy ships.

func TestSprintDispatchNeverExitsSilently(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		// attachSprintWatch's own guard, reached only with --watch. Without
		// --watch the same argument reaches yoke's reporting RunE and is loud,
		// which is why the bug looked like "--watch specifically".
		{"start watch bad id", []string{"start", "notanumber", "--watch"}, "notanumber"},
		{"take watch bad id", []string{"take", "notanumber", "--watch"}, "notanumber"},
		// inbox-ack is a bashy-owned subcommand added after yoke's reporters ran.
		{"inbox-ack bad id", []string{"inbox-ack", "notanumber"}, "notanumber"},
		{"unknown subcommand", []string{"no-such-subverb-xyz"}, "no-such-subverb-xyz"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := runSprintDispatch(tc.args, &out, &errOut)
			if code == 0 {
				t.Fatalf("%v must fail", tc.args)
			}
			both := out.String() + errOut.String()
			if strings.TrimSpace(both) == "" {
				t.Fatalf("%v exited %d with ZERO output on both streams", tc.args, code)
			}
			if !strings.Contains(both, tc.want) {
				t.Fatalf("%v diagnostic does not name %q:\n%s", tc.args, tc.want, both)
			}
		})
	}
}

// Exactly once: a path yoke already reported must not be printed a second time
// by the host.
func TestSprintDispatchReportsEachErrorExactlyOnce(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runSprintDispatch([]string{"inbox-ack", "notanumber"}, &out, &errOut)
	if code == 0 {
		t.Fatal("bad sprint id must fail")
	}
	if n := strings.Count(out.String()+errOut.String(), "notanumber"); n != 1 {
		t.Fatalf("diagnostic names the bad id %d times, want exactly 1:\n%s%s", n, out.String(), errOut.String())
	}
}

// The reporter prefixes every message with the command's own path
// (weavecli.EmitError does), so a message that ALSO opens with its command name
// renders "sprint inbox-ack: sprint inbox-ack: …". Measured on the candidate
// binary before this was pinned. It is cosmetic, but the first line of a
// refusal is the one an agent reads, and a doubled prefix reads like two
// stacked failures.
func TestSprintDispatchDoesNotDoubleTheCommandPrefix(t *testing.T) {
	sprintWatchIsolate(t)
	for _, args := range [][]string{
		{"inbox-ack", "notanumber"},
		{"start", "notanumber", "--watch"},
		{"take", "notanumber", "--watch"},
	} {
		var out, errOut bytes.Buffer
		if code := runSprintDispatch(args, &out, &errOut); code == 0 {
			t.Fatalf("%v must fail", args)
		}
		first, _, _ := strings.Cut(errOut.String(), "\n")
		path := "sprint " + args[0] + ": "
		if rest, ok := strings.CutPrefix(first, path); ok && strings.HasPrefix(rest, path) {
			t.Errorf("%v doubles its own prefix:\n%s", args, first)
		}
	}
}

// `weave` has the same host contract and the same SilenceErrors tree. bashy
// replaces no RunE and adds no subcommand there, so yoke's reporters cover it
// and the silence never showed — but the exit code was still thrown away,
// turning every weave usage failure into an indistinguishable `exit 1`.
func TestWeaveDispatchReportsOnceAndKeepsTheSubverbExitCode(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runWeaveDispatch([]string{"no-such-weave-subverb-xyz"}, &out, &errOut)
	if code != 2 {
		t.Errorf("unknown weave subverb exit = %d, want weavecli.ExitInvalidArg (2)", code)
	}
	both := out.String() + errOut.String()
	if !strings.Contains(both, "no-such-weave-subverb-xyz") {
		t.Fatalf("unknown weave subverb was not reported:\n%s", both)
	}
	// yoke's argerr.go already printed it; the host must not print it again.
	if n := strings.Count(both, "no-such-weave-subverb-xyz"); n != 1 {
		t.Errorf("weave diagnostic printed %d times, want exactly 1:\n%s", n, both)
	}
}
