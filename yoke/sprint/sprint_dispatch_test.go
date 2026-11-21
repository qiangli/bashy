package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestSprintDispatchNeverExitsSilently(t *testing.T) {
	sprintWatchIsolate(t)
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
			code := runSprint(tc.args, &out, &errOut)
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
	sprintWatchIsolate(t)
	var out, errOut bytes.Buffer
	code := runSprint([]string{"inbox-ack", "notanumber"}, &out, &errOut)
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
		if code := runSprint(args, &out, &errOut); code == 0 {
			t.Fatalf("%v must fail", args)
		}
		first, _, _ := strings.Cut(errOut.String(), "\n")
		path := "sprint " + args[0] + ": "
		if rest, ok := strings.CutPrefix(first, path); ok && strings.HasPrefix(rest, path) {
			t.Errorf("%v doubles its own prefix:\n%s", args, first)
		}
	}
}
