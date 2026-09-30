package skills

import (
	"os"
	"slices"
	"testing"
)

func TestSupervisorSkillAvailable(t *testing.T) {
	if !slices.Contains(Names(), "supervisor") {
		t.Fatal("supervisor missing from embedded skill index")
	}
	for _, name := range []string{"conductor", "supervisor"} {
		want, err := os.ReadFile(name + "/SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		got, ok := Body(name)
		if !ok || got != string(want) {
			t.Fatalf("%s: embedded skill differs from canonical source", name)
		}
	}
}
