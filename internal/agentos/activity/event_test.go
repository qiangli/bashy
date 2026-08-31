package activity

import (
	"strings"
	"testing"
)

func TestEventIDIsStableAndTimeFree(t *testing.T) {
	a := Event{Source: SourceWeave, Actor: "meridian", Action: ActionUpdate, Noun: "run", Object: "weave:run/42", Token: "merged"}
	b := a
	if a.computeID() != b.computeID() {
		t.Fatalf("same tuple produced different ids")
	}
	// A different transaction boundary is a different event.
	b.Token = "failed"
	if a.computeID() == b.computeID() {
		t.Fatalf("token is not participating in the id")
	}
	// The clock must not. A clock in the key fills the dedup index with
	// singletons and silently degrades at-least-once to once-per-retry.
	c := a
	c.At = c.At.AddDate(0, 0, 1)
	c.Seq = 99
	if a.computeID() != c.computeID() {
		t.Fatalf("At/Seq leaked into the event id")
	}
}

func TestValidateRefusesBodiesSecretsAndUnknownVocabulary(t *testing.T) {
	base := Event{Source: SourceMB, Actor: "meridian", Action: ActionCreate, Noun: "message", Object: "mb:post/7"}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}

	cases := []struct {
		name string
		mut  func(*Event)
		want string
	}{
		{"unknown source", func(e *Event) { e.Source = "nope" }, "unknown source"},
		{"unknown action", func(e *Event) { e.Action = "frobnicate" }, "unknown action"},
		{"unknown status", func(e *Event) { e.Status = "greenish" }, "unknown status"},
		{"no actor", func(e *Event) { e.Actor = "" }, "actor is required"},
		{"no object", func(e *Event) { e.Object = "" }, "object reference is required"},
		{"terminal output", func(e *Event) { e.Object = "mb:post/7\n+ diff line" }, "control character"},
		{"oversized", func(e *Event) { e.Object = strings.Repeat("x", MaxFieldBytes+1) }, "refused, not truncated"},
		{"credential", func(e *Event) { e.Object = "ghp_0123456789abcdef" }, "looks like a credential"},
		{"hop limit", func(e *Event) { e.Hop = MaxHop }, "delivery loop"},
		{"list too long", func(e *Event) {
			for i := 0; i <= MaxListLen; i++ {
				e.Members = append(e.Members, "a")
			}
		}, "refused, not truncated"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := base
			tc.mut(&e)
			err := e.Validate()
			if err == nil {
				t.Fatalf("expected refusal")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestEnvelopeHasNoBodyField(t *testing.T) {
	// The structural control: there is nowhere to put a diff. If a body field
	// is ever added this test is the thing that has to be deliberately deleted.
	for _, forbidden := range []string{"Body", "Text", "Message", "Output", "Diff", "Prompt"} {
		if fieldExists(Event{}, forbidden) {
			t.Fatalf("Event grew a %s field; the envelope must carry references, not content", forbidden)
		}
	}
}

func TestSubjectIsCompactAndAlwaysCarriesTheID(t *testing.T) {
	e := Event{
		Source: SourceWeave, Actor: "meridian", Action: ActionFail, Noun: "run",
		Object: "weave:run/42", Status: StatusFailed,
		Scope: Scope{Repo: "bashy", Sprint: "88", Topic: "conformance", Room: "board"},
	}
	e.ID = e.computeID()
	got := e.Subject()
	if len(got) > MaxSubjectBytes {
		t.Fatalf("subject is %d bytes: %q", len(got), got)
	}
	for _, want := range []string{"weave", "fail", "run", "weave:run/42", "failed", "id=" + e.ID} {
		if !strings.Contains(got, want) {
			t.Fatalf("subject %q is missing %q", got, want)
		}
	}
}

func TestSubjectElidesScopeBeforeTheIDWhenLong(t *testing.T) {
	long := strings.Repeat("s", MaxFieldBytes)
	e := Event{
		Source: SourceWeave, Actor: "a", Action: ActionUpdate, Noun: "run",
		Object: long, Scope: Scope{Repo: long, Sprint: long, Topic: long, Room: long},
	}
	e.ID = e.computeID()
	got := e.Subject()
	if !strings.HasSuffix(got, "id="+e.ID) {
		t.Fatalf("the id must survive elision: %q", got)
	}
	if !strings.Contains(got, long) {
		t.Fatalf("the object must survive elision: %q", got)
	}
}
