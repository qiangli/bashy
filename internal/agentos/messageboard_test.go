package agentos

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/meet"
	"github.com/qiangli/coreutils/pkg/weave"
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
		{"CurrentSessionClaim", func() { bus.CurrentSessionClaim = nil }, func() bool { return bus.CurrentSessionClaim != nil }},
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
// marker table is registry DATA (`bashy tool add` extends it), so a second
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
			if bus.FleetNames == nil || bus.FleetSelect == nil || bus.FleetResolveName == nil || bus.CurrentSessionClaim == nil || bus.DetectHarness == nil {
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

// The web console serves other verbs' surfaces IN ITS OWN PROCESS, so it owes
// their host wiring too. `bashy app` wired meet and not the board, which is
// invisible at every level that usually catches things: it compiles, it serves,
// and a post from the browser returns 200. What it loses is fleet resolution —
// a name that the CLI canonicalizes through the catalog falls through to
// reader/person instead, so the post lands addressed to something else while
// reporting success. That is the board's own founding defect, reintroduced by a
// second front door.
func TestWireWebConsole_ConnectsBothTheRoomAndTheBoard(t *testing.T) {
	bus.FleetResolveName = nil
	bus.FleetSelect = nil
	bus.CurrentSessionClaim = nil
	bus.DetectHarness = nil
	meet.FetchMB = nil

	wireWebConsole()

	for _, c := range []struct {
		name string
		got  func() bool
	}{
		{"bus.FleetResolveName", func() bool { return bus.FleetResolveName != nil }},
		{"bus.FleetSelect", func() bool { return bus.FleetSelect != nil }},
		{"bus.CurrentSessionClaim", func() bool { return bus.CurrentSessionClaim != nil }},
		{"bus.DetectHarness", func() bool { return bus.DetectHarness != nil }},
		{"meet.FetchMB", func() bool { return meet.FetchMB != nil }},
	} {
		if !c.got() {
			t.Errorf("%s is nil after wireWebConsole — the console mounts the panel but not its host wiring", c.name)
		}
	}
}

// seedSprintBoard writes the lease table this test selects against. The
// records are queue.json's `stories` — the same store `bashy sprint` owns —
// written directly so the test pins the READER side of the seam (what
// LiveSprintManagers accepts) without coupling to any sprint CLI behavior.
func seedSprintBoard(t *testing.T, stories []map[string]any) {
	t.Helper()
	dir := t.TempDir()
	b, err := json.Marshal(map[string]any{"next_id": 1, "next_story_id": len(stories) + 1, "stories": stories})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "queue.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASHY_SPRINT_DIR", dir)
}

func seededStory(id int, holder string, at time.Time) map[string]any {
	s := map[string]any{"id": id, "title": "t", "column": "doing", "created": at}
	if holder != "" {
		s["lease"] = map[string]any{"holder": holder, "at": at}
	}
	return s
}

// --role is the one selector bus cannot resolve alone: Role is a SPRINT fact,
// not a binding fact, and pkg/bus is transport that must not read the sprint
// store. fleetSelectAudience is the host half that joins them, and this test
// is the proof the join happened — the wired selector (not the private
// function) must address exactly the seated managers and refuse what it
// cannot honestly answer.
func TestFleetSelectAnswersRoleFromTheSprintRecords(t *testing.T) {
	fresh := time.Now().UTC()
	stale := fresh.Add(-weave.SprintLeaseTTL - time.Minute)
	seedSprintBoard(t, []map[string]any{
		seededStory(1, "zoe", fresh),
		seededStory(2, "zoe", fresh),
		seededStory(3, "ghost", stale),
		seededStory(4, "", fresh),
		{"id": 5, "title": "unowned", "column": "backlog", "created": fresh},
	})

	wireMessageBoard()

	t.Run("conductor addresses only live holders", func(t *testing.T) {
		got, err := bus.FleetSelect(bus.Audience{Role: "conductor"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0] != "zoe" {
			t.Fatalf("FleetSelect(role conductor) = %v, want [zoe] — stale leases, blank holders, and duplicate seats must not become addressees", got)
		}
	})

	t.Run("unknown role is refused by name", func(t *testing.T) {
		_, err := bus.FleetSelect(bus.Audience{Role: "reviewer"})
		if err == nil {
			t.Fatal("an unknown role must not resolve to an empty audience and report success — that is a broadcast pretending to be an answer")
		}
		if !strings.Contains(err.Error(), "reviewer") {
			t.Fatalf("refusal must name the selector it refused: %v", err)
		}
	})

	for _, tc := range []struct {
		field string
		aud   bus.Audience
	}{
		{"band", bus.Audience{Role: "conductor", Band: 4}},
		{"tool", bus.Audience{Role: "conductor", Tool: "ycode"}},
		{"provider", bus.Audience{Role: "conductor", Provider: "p"}},
		{"family", bus.Audience{Role: "conductor", Family: "f"}},
		{"version", bus.Audience{Role: "conductor", Version: "v"}},
	} {
		t.Run("role rejects "+tc.field, func(t *testing.T) {
			got, err := bus.FleetSelect(tc.aud)
			if err == nil || !strings.Contains(err.Error(), "role") || !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("FleetSelect(role+%s) = %v, %v; want named refusal", tc.field, got, err)
			}
			if len(got) != 0 {
				t.Fatalf("invalid selector returned recipients: %v", got)
			}
		})
	}
}

// Drive an actual stage command through the host's sprint-roster resolver and
// then read as a second live manager with no sprint topic subscription. Field-only
// announcement tests cannot prove this combined path or its uncapped routing.
func TestStageAnnouncementsReachUnsubscribedLivePeer(t *testing.T) {
	isolateStageBoard(t)
	fresh := time.Now().UTC()
	seedSprintBoard(t, []map[string]any{
		seededStory(1, "stage-author", fresh),
		seededStory(2, "stage-peer", fresh),
		seededStory(3, "stage-ghost", fresh.Add(-weave.SprintLeaseTTL-time.Minute)),
		seededStory(4, "", fresh),
	})
	wireMessageBoard()
	if _, err := bus.EnsureSubscription("stage-peer"); err != nil {
		t.Fatal(err)
	}
	for _, concern := range bus.DeclaredConcerns("stage-peer") {
		if concern == "sprint" || concern == "*" {
			t.Fatalf("peer unexpectedly subscribed to %q", concern)
		}
	}
	for i := range 5 {
		column := "backlog"
		if i%2 == 1 {
			column = "doing"
		}
		runStageMove(t, column)
	}
	posts, err := bus.Posts()
	if err != nil || len(posts) != 5 {
		t.Fatalf("stage history = %v, %v; want five posts", posts, err)
	}
	for _, p := range posts {
		if p.Topic != "sprint" || p.Audience == nil || p.Audience.Role != "conductor" {
			t.Fatalf("stage announcement lost routing: %+v", p)
		}
	}
	directed, other, older, err := bus.Unseen("stage-peer", 2)
	if err != nil || len(directed)+len(other) != 5 || older != 0 {
		t.Fatalf("peer read: directed=%d other=%d older=%d err=%v; want all five uncapped", len(directed), len(other), older, err)
	}
	for _, excluded := range []string{"stage-ghost", "unowned-reader"} {
		directed, other, _, err := bus.Unseen(excluded, 2)
		if err != nil || len(directed)+len(other) != 0 {
			t.Fatalf("excluded reader %s received %d posts: %v", excluded, len(directed)+len(other), err)
		}
	}
}

func TestStageAnnouncementResolutionFailurePreservesTransition(t *testing.T) {
	for _, failure := range []string{"unavailable", "roster read failed"} {
		t.Run(failure, func(t *testing.T) {
			isolateStageBoard(t)
			seedSprintBoard(t, []map[string]any{seededStory(1, "stage-author", time.Now().UTC())})
			previous := bus.FleetSelect
			t.Cleanup(func() { bus.FleetSelect = previous })
			bus.FleetSelect = nil
			if failure != "unavailable" {
				bus.FleetSelect = func(bus.Audience) ([]string, error) { return nil, errors.New(failure) }
			}
			stderr := runStageMove(t, "backlog")
			if !strings.Contains(stderr, "state change was recorded") || !strings.Contains(stderr, failure) {
				t.Fatalf("announcement failure not visible: %q", stderr)
			}
			posts, err := bus.Posts()
			if err != nil || len(posts) != 0 {
				t.Fatalf("failed announcement posts = %v, %v", posts, err)
			}
			data, err := os.ReadFile(filepath.Join(os.Getenv("BASHY_SPRINT_DIR"), "queue.json"))
			if err != nil {
				t.Fatal(err)
			}
			var board struct {
				Stories []struct {
					Column string `json:"column"`
				} `json:"stories"`
			}
			if err := json.Unmarshal(data, &board); err != nil {
				t.Fatal(err)
			}
			if len(board.Stories) != 1 || board.Stories[0].Column != "backlog" {
				t.Fatalf("stage did not persist: %s", data)
			}
		})
	}
}

func isolateStageBoard(t *testing.T) {
	t.Helper()
	for _, variable := range []string{"BASHY_HOME", "BASHY_MB_DIR", "BASHY_ROOM_DIR", "BASHY_FLEET_DIR", "BASHY_MEET_DIR"} {
		t.Setenv(variable, t.TempDir())
	}
	for _, variable := range []string{"BASHY_AGENTS_DIR", "BASHY_PEOPLE_DIR", "BASHY_AGENTS_PATH", "BASHY_PEOPLE_PATH"} {
		t.Setenv(variable, "")
	}
	t.Setenv("BASHY_PRINCIPAL", "stage-author")
	t.Setenv("BASHY_SPRINT_ANNOUNCE", "1")
}

func runStageMove(t *testing.T, column string) string {
	t.Helper()
	cmd := weave.NewSprintCmd()
	var out, stderr bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"move", "1", column})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("stage move: %v, stderr=%s", err, &stderr)
	}
	return stderr.String()
}

func TestFleetSelectRejectsWhitespaceRole(t *testing.T) {
	isolateStageBoard(t)
	wireMessageBoard()
	names, err := bus.FleetSelect(bus.Audience{Role: " \t\n "})
	if err == nil || !strings.Contains(err.Error(), "role") || len(names) != 0 {
		t.Fatalf("whitespace role resolved to %v, %v; want a named refusal", names, err)
	}
}

func TestFleetSelectRejectsMalformedCatalogBeforeSend(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "zero", true: "partial"}[partial], func(t *testing.T) {
			isolateStageBoard(t)
			dir := filepath.Join(os.Getenv("BASHY_FLEET_DIR"), "agents")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if partial {
				if err := os.WriteFile(filepath.Join(dir, "valid-peer.yaml"), []byte("name: valid-peer\nkind: agent\ntool: sprint-test\nmodel: example\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte("name: ["), 0o600); err != nil {
				t.Fatal(err)
			}
			wireMessageBoard()
			aud := bus.Audience{Tool: "sprint-test"}
			names, err := bus.FleetSelect(aud)
			if err == nil || !strings.Contains(err.Error(), "broken") || len(names) != 0 {
				t.Fatalf("malformed catalog resolved to %v, %v; want no names and the catalog error", names, err)
			}
			_, err = bus.Send(bus.SendRequest{From: "tester", Audience: &aud, Body: "must refuse incomplete roster"})
			if err == nil || !strings.Contains(err.Error(), "broken") {
				t.Fatalf("Send error = %v", err)
			}
			posts, err := bus.Posts()
			if err != nil || len(posts) != 0 {
				t.Fatalf("failed send posts = %v, %v", posts, err)
			}
		})
	}
}
