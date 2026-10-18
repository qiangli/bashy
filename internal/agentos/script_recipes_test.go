package agentos

import (
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/atlas"
)

func TestScriptRecipeProjectsSourceExamples(t *testing.T) {
	entry, _ := atlas.Lookup("sprint")
	script, err := scriptHelp("sprint", "Usage:\n  sprint [flags]\n", entry, "")
	if err != nil {
		t.Fatal(err)
	}
	recipe, err := scriptRecipeMarkdown("sprint", script)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(recipe, "Source: `bashy help --format=script sprint`") {
		t.Fatal("source command identity missing")
	}
	if strings.Contains(recipe, "Usage:") || strings.Contains(recipe, "# Examples derived") {
		t.Fatal("script scaffolding leaked into recipe")
	}
	for _, row := range TriggerRows() {
		command := strings.TrimSpace(row.Example)
		if command == "" {
			continue
		}
		if got := strings.Count(recipe, "\n"+command+"\n"); got != 1 {
			t.Errorf("command %q occurs %d times, want one", command, got)
		}
	}
	if got := strings.Count(recipe, "```bash\n"); got != len(TriggerRows()) {
		t.Errorf("got %d tasks, want %d", got, len(TriggerRows()))
	}
	if !strings.Contains(recipe, "bashy adapt --no-input -- apt install jq") {
		t.Fatal("source --no-input command was not preserved")
	}
}

func TestScriptRecipeRejectsMissingExamples(t *testing.T) {
	if _, err := scriptRecipeMarkdown("sprint", "# Usage\n"); err == nil {
		t.Fatal("missing examples accepted")
	}
}
