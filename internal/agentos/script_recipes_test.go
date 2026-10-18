package agentos

import (
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/qiangli/bashy/skills"
	"github.com/qiangli/yoke/pkg/atlas"
	coreskills "github.com/qiangli/yoke/pkg/skills"
)

func TestScriptRecipeProjectsTriggerRows(t *testing.T) {
	entry, _ := atlas.Lookup("sprint")
	bash, err := scriptHelp("sprint", "Usage:\n  sprint [flags]\n", entry, "")
	if err != nil {
		t.Fatal(err)
	}
	dag, err := scriptRecipeDAG("sprint")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(bash, "#!/usr/bin/env bash\n") || !strings.Contains(bash, "case \"${1:-}\" in") {
		t.Fatal("Bash recipe is not a task-selecting script")
	}
	if !strings.HasPrefix(dag, "---\nname: bashy-sprint-recipes\n") || !strings.Contains(dag, "\n## Tasks\n") {
		t.Fatal("dag recipe lacks dag.md structure")
	}
	tasks, err := scriptRecipeTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != len(TriggerRows()) {
		t.Fatalf("got %d tasks for %d rows", len(tasks), len(TriggerRows()))
	}
	for _, task := range tasks {
		if got := strings.Count(bash, "\n    "+task.command+"\n"); got != 1 {
			t.Errorf("Bash command %q occurs %d times", task.command, got)
		}
		if got := strings.Count(dag, "\n"+task.command+"\n"); got != 1 {
			t.Errorf("dag command %q occurs %d times", task.command, got)
		}
		if !strings.Contains(bash, task.name+") # "+task.intent) || !strings.Contains(dag, "### "+task.name+"\n"+task.intent) {
			t.Errorf("task %q not identified in both files", task.name)
		}
	}
	if !strings.Contains(bash, "--dry-run --no-input") || !strings.Contains(dag, "--dry-run --no-input") {
		t.Fatal("prompting recipe did not fill --no-input")
	}
	again, err := scriptRecipeDAG("sprint")
	if err != nil || again != dag {
		t.Fatal("dag generation is not deterministic")
	}
}

func TestScriptRecipeInstalledFilesMatchGenerator(t *testing.T) {
	source := coreskills.EmbedSource(skills.FS, coreskills.RingEmbedded)
	files, err := source.Files("bashy")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"recipes/trigger-rows.sh", "recipes/dag.md"} {
		if !slices.Contains(files, path) {
			t.Fatalf("skill export does not carry %s", path)
		}
	}
	entry, _ := atlas.Lookup("sprint")
	bash, err := scriptHelp("sprint", "Usage:\n", entry, "")
	if err != nil {
		t.Fatal(err)
	}
	dag, err := scriptRecipeDAG("sprint")
	if err != nil {
		t.Fatal(err)
	}
	for path, generated := range map[string]string{
		"bashy/recipes/trigger-rows.sh": bash,
		"bashy/recipes/dag.md":          dag,
	} {
		data, err := fs.ReadFile(skills.FS, path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(path, ".sh") {
			// Classic usage is command-specific; the executable recipe body
			// before it is the installed artifact's source of truth.
			generated, _, _ = strings.Cut(generated, "\n# Usage\n")
			installed, _, _ := strings.Cut(string(data), "\n# Usage\n")
			if installed != generated {
				t.Fatal("installed Bash recipe diverges from trigger rows")
			}
		} else if string(data) != generated {
			t.Fatal("installed dag recipe diverges from trigger rows")
		}
	}
}

func TestScriptRecipeRejectsMissingAndDuplicateRows(t *testing.T) {
	saved := lensTriggerDecls
	defer func() { lensTriggerDecls = saved }()
	lensTriggerDecls = append(append([]TriggerRow(nil), saved...), TriggerRow{Intent: "missing command", Example: "example is not a runnable recipe"})
	if _, err := scriptRecipeTasks(); err == nil {
		t.Fatal("missing recipe command accepted")
	}
	lensTriggerDecls[len(lensTriggerDecls)-1].Recipe = "bashy help"
	lensTriggerDecls[len(lensTriggerDecls)-1].Intent = saved[0].Intent
	if _, err := scriptRecipeTasks(); err == nil {
		t.Fatal("duplicate task name accepted")
	}
}
