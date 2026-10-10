package skills

import (
	"os"
	"slices"
	"testing"
)

// TestBashyIsTheOnlyEmbeddedSkill: moving the other skill folders' bodies
// into bashy/commands/ (Sprint 406) must leave bashy as the sole top-level
// skill directory — a second one would just be a second way to say "every
// command is a skill".
func TestBashyIsTheOnlyEmbeddedSkill(t *testing.T) {
	if got := Names(); len(got) != 1 || got[0] != "bashy" {
		t.Fatalf("Names() = %v, want [bashy]", got)
	}
}

// TestCommandDocsAvailable: every front-door command doc moved under
// bashy/commands/ is embedded byte-identical to its canonical source, along
// with its reference.md/skill.dhnt companions where it has them.
func TestCommandDocsAvailable(t *testing.T) {
	want := []string{"conductor", "force-agent-shell", "inbox", "knowledge-transfer", "sprint", "steward", "supervisor"}
	if got := CommandNames(); !slices.Equal(got, want) {
		t.Fatalf("CommandNames() = %v, want %v", got, want)
	}

	for _, name := range want {
		wantDoc, err := os.ReadFile("bashy/commands/" + name + ".md")
		if err != nil {
			t.Fatal(err)
		}
		gotDoc, ok := CommandDoc(name)
		if !ok || gotDoc != string(wantDoc) {
			t.Fatalf("%s: embedded command doc differs from canonical source", name)
		}
	}

	for _, name := range []string{"conductor", "knowledge-transfer"} {
		wantRef, err := os.ReadFile("bashy/reference/" + name + ".md")
		if err != nil {
			t.Fatal(err)
		}
		gotRef, ok := CommandReference(name)
		if !ok || gotRef != string(wantRef) {
			t.Fatalf("%s: embedded reference differs from canonical source", name)
		}
	}

	for _, name := range []string{"conductor", "steward", "force-agent-shell"} {
		wantDhnt, err := os.ReadFile("bashy/commands/" + name + ".dhnt")
		if err != nil {
			t.Fatal(err)
		}
		gotDhnt, ok := CommandDhnt(name)
		if !ok || gotDhnt != string(wantDhnt) {
			t.Fatalf("%s: embedded skill.dhnt differs from canonical source", name)
		}
	}

	if _, ok := CommandReference("sprint"); ok {
		t.Fatal("sprint has no reference.md; CommandReference should report absent")
	}
	if _, ok := CommandDhnt("sprint"); ok {
		t.Fatal("sprint has no skill.dhnt; CommandDhnt should report absent")
	}
}
