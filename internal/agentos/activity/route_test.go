package activity

import (
	"strings"
	"testing"
)

func routed(t *testing.T, e Event, in []Interest) map[string]Recipient {
	t.Helper()
	out := map[string]Recipient{}
	for _, r := range Route(e, in) {
		out[r.Subscriber] = r
	}
	return out
}

func TestRoutePrecedenceIsDeterministicAndStrongestWins(t *testing.T) {
	e := Event{
		Source: SourceWeave, Actor: "conductor", Action: ActionFail, Noun: "run",
		Object: "weave:run/42", Status: StatusFailed,
		Scope:     Scope{Repo: "bashy", Sprint: "88"},
		Mentions:  []string{"meridian"},
		Owner:     "steward",
		Assignees: []string{"meridian", "atlas"},
		Members:   []string{"meridian", "atlas", "steward", "bystander"},
	}
	// meridian is mentioned AND assigned AND a member: the strongest reason
	// must be the one reported, because it is the honest explanation.
	got := routed(t, e, nil)
	if got["meridian"].Reason != ReasonMention {
		t.Fatalf("meridian matched by %q, want %q", got["meridian"].Reason, ReasonMention)
	}
	if got["atlas"].Reason != ReasonAssignment {
		t.Fatalf("atlas matched by %q, want %q", got["atlas"].Reason, ReasonAssignment)
	}
	if got["steward"].Reason != ReasonOwnership {
		t.Fatalf("steward matched by %q, want %q", got["steward"].Reason, ReasonOwnership)
	}
	if got["bystander"].Reason != ReasonMembership {
		t.Fatalf("bystander matched by %q, want %q", got["bystander"].Reason, ReasonMembership)
	}
	// The actor is never told what it just did.
	if _, ok := got["conductor"]; ok {
		t.Fatalf("the event echoed back to its own actor")
	}

	// Deterministic order: strongest reason first, then subscriber name.
	var order []string
	for _, r := range Route(e, nil) {
		order = append(order, r.Subscriber+":"+r.Reason)
	}
	want := []string{"meridian:mention", "atlas:assignment", "steward:ownership", "bystander:membership"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", order, want)
	}

	// Same inputs, same answer — twice, so two agents agree about who was told.
	for i := 0; i < 5; i++ {
		var again []string
		for _, r := range Route(e, nil) {
			again = append(again, r.Subscriber+":"+r.Reason)
		}
		if strings.Join(again, ",") != strings.Join(order, ",") {
			t.Fatalf("routing is not deterministic: %v vs %v", again, order)
		}
	}
}

func TestRouteExplainsWhyEachRecipientMatched(t *testing.T) {
	e := Event{
		Source: SourceSprint, Actor: "conductor", Action: ActionUpdate, Noun: "story",
		Object: "sprint:88/story/7", Scope: Scope{Sprint: "88"},
		Owner: "steward", Members: []string{"atlas"}, DependsOn: []string{"weave:run/42"},
	}
	in := []Interest{
		{Subscriber: "watcher", Objects: []string{"weave:run/42"}, Wake: true},
		{Subscriber: "sprinter", Sprints: []string{"88"}, Wake: true},
	}
	got := routed(t, e, in)
	checks := map[string]string{
		"steward":  "owns sprint:88/story/7",
		"atlas":    "member of sprint 88",
		"watcher":  "depends on watched weave:run/42",
		"sprinter": "subscribed to sprint 88",
	}
	for who, want := range checks {
		r, ok := got[who]
		if !ok {
			t.Fatalf("%s was not routed", who)
		}
		if !strings.Contains(r.Why, want) {
			t.Fatalf("%s: why = %q, want it to contain %q", who, r.Why, want)
		}
	}
}

func TestReadEventsAreNeverBroadBroadcast(t *testing.T) {
	e := Event{
		Source: SourceTodo, Actor: "conductor", Action: ActionRead, Noun: "task",
		Object: "todo:task/3", Scope: Scope{Repo: "bashy"},
		Owner: "steward", Mentions: []string{"meridian"},
		Members: []string{"atlas", "auditor"},
	}
	// With nobody opted in, a read reaches NOBODY — not even the owner or a
	// mentioned identity. Read traffic is the highest-volume, lowest-value
	// stream there is.
	if got := Route(e, nil); len(got) != 0 {
		t.Fatalf("a read event broadcast to %v", got)
	}

	in := []Interest{
		{Subscriber: "auditor", Audit: true, Repos: []string{"bashy"}, Wake: true},
		{Subscriber: "atlas", Repos: []string{"bashy"}, Wake: true},
	}
	got := routed(t, e, in)
	if _, ok := got["auditor"]; !ok {
		t.Fatalf("an explicit audit interest did not receive the read event")
	}
	if _, ok := got["atlas"]; ok {
		t.Fatalf("a non-audit subscriber received a read event")
	}
	// Even an audit subscriber must not match a read on the membership axis:
	// membership is the broadcast axis wearing a different hat.
	only := []Interest{{Subscriber: "atlas", Audit: true, Wake: true}}
	if r := routed(t, e, only); r["atlas"].Reason == ReasonMembership {
		t.Fatalf("a read event routed on membership")
	}
}

func TestWakeTierFollowsDirectnessAndPolicyWins(t *testing.T) {
	e := Event{
		Source: SourceWeave, Actor: "conductor", Action: ActionFail, Noun: "run",
		Object: "weave:run/42", Owner: "steward", Members: []string{"atlas"},
	}
	got := routed(t, e, nil)
	if !got["steward"].Wake {
		t.Fatalf("ownership should justify a wake")
	}
	if got["atlas"].Wake {
		t.Fatalf("membership must not justify a wake")
	}
	// An operator who declared "never interrupt this agent" outranks even the
	// strongest match reason.
	quiet := []Interest{{Subscriber: "steward", Wake: false}}
	if routed(t, e, quiet)["steward"].Wake {
		t.Fatalf("Wake:false was overridden by a strong reason")
	}
}

func TestMuteAndFiltersBindOnEveryReason(t *testing.T) {
	e := Event{
		Source: SourceMeet, Actor: "conductor", Action: ActionCreate, Noun: "message",
		Object: "meet:room/1#9", Mentions: []string{"muted", "filtered"},
	}
	in := []Interest{
		{Subscriber: "muted", Mute: true, Wake: true},
		// A declared filter binds even when the match came from a mention: an
		// interest that says "only weave" means it.
		{Subscriber: "filtered", Sources: []string{SourceWeave}, Wake: true},
	}
	if got := Route(e, in); len(got) != 0 {
		t.Fatalf("mute/filter did not bind: %v", got)
	}
}

func TestLoopPreventionFloors(t *testing.T) {
	reserved := Event{Source: SourceActivity, Actor: "x", Action: ActionCreate, Noun: "n", Object: "o", Owner: "steward"}
	if got := Route(reserved, nil); len(got) != 0 {
		t.Fatalf("the reserved source routed to %v", got)
	}
	deep := Event{Source: SourceWeave, Actor: "x", Action: ActionCreate, Noun: "n", Object: "o", Owner: "steward", Hop: MaxHop}
	if got := Route(deep, nil); len(got) != 0 {
		t.Fatalf("an over-hop event routed to %v", got)
	}
}
