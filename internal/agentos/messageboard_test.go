package agentos

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/meet"
)

// THE HOP THAT KEEPS GETTING MISSED.
//
// Every hook here defaults to nil and every nil default is SILENT: the board
// keeps working, it just answers a fleet question wrongly — or, for
// DetectHarness, signs an agent's post with the operator's login name. There is
// no crash to notice and no log line to grep, which is precisely why the same
// mistake has been made five times in this tree.
//
// pkg/bus tests prove each mechanism works when wired. This test proves it IS
// wired, which is a different claim and the one that was false.
func TestWireMessageBoard_ConnectsEveryFleetSeam(t *testing.T) {
	for _, h := range []struct {
		name string
		set  func()
		got  func() bool
	}{
		{"FleetNames", func() { bus.FleetNames = nil }, func() bool { return bus.FleetNames != nil }},
		{"FleetSelect", func() { bus.FleetSelect = nil }, func() bool { return bus.FleetSelect != nil }},
		{"FleetResolveName", func() { bus.FleetResolveName = nil }, func() bool { return bus.FleetResolveName != nil }},
		// The identity seam. Unwired, `bashy mb` misattributes every post made
		// from a third-party TUI to whoever owns the login session.
		{"DetectHarness", func() { bus.DetectHarness = nil }, func() bool { return bus.DetectHarness != nil }},
		{"PrepareTurnInbox", func() { bus.PrepareTurnInbox = nil }, func() bool { return bus.PrepareTurnInbox != nil }},
	} {
		t.Run(h.name, func(t *testing.T) {
			h.set()
			wireMessageBoard()
			if !h.got() {
				t.Fatalf("bus.%s is nil after wireMessageBoard — the seam exists but nothing connects it", h.name)
			}
		})
	}
}

// DetectHarness must be the catalog's own detector, not a private copy. The
// marker table is registry DATA (`bashy tools add` extends it), so a second
// implementation would drift the moment a harness is added — and the drift
// would show up as an agent silently posting under the operator's name again.
func TestWireMessageBoard_HarnessDetectionAnswersForThisProcess(t *testing.T) {
	wireMessageBoard()
	// Under the agent harness running these tests this reports true; on a bare
	// CI runner it reports false. Either is correct — what must not happen is a
	// panic or a hook that cannot answer at all.
	tool, ok := bus.DetectHarness()
	if ok && tool == "" {
		t.Fatal("DetectHarness reported an agent with no tool name — an identity that resolves to nothing is the bug, not the fix")
	}
}

// Meet deliberately cannot import bus, so these two callbacks are the only
// path from the shipped command to its already-built message-board support.
// A unit test in either package cannot detect this hop going missing.
func TestWireMeet_ConnectsEveryMessageBoardSeam(t *testing.T) {
	for _, h := range []struct {
		name string
		set  func()
		got  func() bool
	}{
		{"Notify", func() { meet.Notify = nil }, func() bool { return meet.Notify != nil }},
		{"FetchMB", func() { meet.FetchMB = nil }, func() bool { return meet.FetchMB != nil }},
		{"PostMB", func() { meet.PostMB = nil }, func() bool { return meet.PostMB != nil }},
	} {
		t.Run(h.name, func(t *testing.T) {
			h.set()
			wireMeet()
			if !h.got() {
				t.Fatalf("meet.%s is nil after wireMeet — the seam exists but nothing connects it", h.name)
			}
		})
	}
}

func TestMeetMessageBoardSeamPreservesDeliveryAndAuthors(t *testing.T) {
	t.Setenv("BASHY_MB_DIR", t.TempDir())

	for _, post := range []bus.Post{
		{From: "alice", Topic: "first", Body: "alpha"},
		{From: "bob", Topic: "second", Body: "beta"},
	} {
		if err := bus.PostMessage(post); err != nil {
			t.Fatalf("seed mb: %v", err)
		}
	}

	wireMeet()
	got, err := meet.FetchMB([]int64{2, 1})
	if err != nil {
		t.Fatalf("FetchMB: %v", err)
	}
	if len(got) != 2 || got[0].From != "bob" || got[0].Body != "beta" || got[1].From != "alice" || got[1].Body != "alpha" {
		t.Fatalf("FetchMB lost requested order, author, or text: %+v", got)
	}

	delivered, reason, err := meet.Notify("test-agent", meet.Invitation{
		Topic: "seam proof", Join: "bashy meet read durable-id --as test-agent",
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if !delivered || !strings.Contains(reason, "posted to mb") {
		t.Fatalf("Notify receipt = delivered %v, reason %q; want a truthful durable delivery", delivered, reason)
	}
	posts, err := bus.Posts()
	if err != nil {
		t.Fatalf("read notified post: %v", err)
	}
	last := posts[len(posts)-1]
	if last.To != "test-agent" || last.From != "meet" || !strings.Contains(last.Body, "bashy meet read durable-id --as test-agent") {
		t.Fatalf("notification post = %+v", last)
	}

	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("block board creation"), 0o600); err != nil {
		t.Fatalf("create blocked board path: %v", err)
	}
	t.Setenv("BASHY_MB_DIR", blocked)
	delivered, _, err = meet.Notify("test-agent", meet.Invitation{Join: "literal join command"})
	if err == nil || delivered {
		t.Fatalf("failed board append = delivered %v, err %v; invite must not claim notification", delivered, err)
	}
}

func TestMessageBoardFrontDoorResolvesInboxAndNotify(t *testing.T) {
	for _, verb := range []string{"inbox", "notify"} {
		t.Run(verb, func(t *testing.T) {
			bus.FleetNames = nil
			bus.FleetSelect = nil
			bus.FleetResolveName = nil
			bus.DetectHarness = nil

			cmd, label, ok := newBusFrontDoorCmd(verb)
			if !ok || cmd == nil {
				t.Fatalf("bashy %s is not mounted in the AgentOS bus front door", verb)
			}
			if label != verb {
				t.Fatalf("bashy %s resolved with label %q", verb, label)
			}
			if bus.FleetNames == nil || bus.FleetSelect == nil || bus.FleetResolveName == nil || bus.DetectHarness == nil {
				t.Fatalf("bashy %s resolved without wiring the fleet seams", verb)
			}

			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{"--help"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("bashy %s --help failed: %v", verb, err)
			}
			if !strings.Contains(out.String(), verb) {
				t.Fatalf("bashy %s --help did not render the mounted command help: %q", verb, out.String())
			}
		})
	}
}
