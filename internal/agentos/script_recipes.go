// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"fmt"
	"strings"
)

type scriptRecipeTask struct {
	name, intent, command string
}

// scriptRecipeTasks projects S9's trigger rows. Both files use these exact
// commands; there is no second list of triggers or examples here.
func scriptRecipeTasks() ([]scriptRecipeTask, error) {
	var tasks []scriptRecipeTask
	seen := make(map[string]bool)
	for _, row := range TriggerRows() {
		command := strings.TrimSpace(row.Recipe)
		if command == "" {
			command = strings.TrimSpace(row.Example)
		}
		if command == "" || strings.ContainsAny(command, "\r\n") {
			return nil, fmt.Errorf("invalid recipe command for %q", row.Intent)
		}
		name := recipeTaskName(row.Intent)
		if name == "" || seen[name] {
			return nil, fmt.Errorf("invalid or duplicate recipe task %q", row.Intent)
		}
		seen[name] = true
		tasks = append(tasks, scriptRecipeTask{name: name, intent: row.Intent, command: command})
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("no trigger rows for recipes")
	}
	return tasks, nil
}

func recipeTaskName(intent string) string {
	var b strings.Builder
	separator := false
	for _, r := range strings.ToLower(intent) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if separator && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			separator = false
		} else {
			separator = true
		}
	}
	return b.String()
}

// scriptRecipeDAG emits a dag.md file. RECIPE_* inputs are supplied as
// environment variables, so its task bodies match the Bash recipe exactly.
func scriptRecipeDAG(name string) (string, error) {
	tasks, err := scriptRecipeTasks()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "---\nname: bashy-%s-recipes\ndescription: S9 trigger recipes for bashy %s\ntype: dag\ndefault: %s\n---\n\n", name, name, tasks[0].name)
	fmt.Fprintf(&b, "# bashy %s recipes\n\nSource: `bashy help --format=dag %s`\n\n## Tasks\n\n", name, name)
	for _, task := range tasks {
		fmt.Fprintf(&b, "### %s\n%s\n\n```bash\n%s\n```\n\n", task.name, task.intent, task.command)
	}
	return strings.TrimRight(b.String(), "\n") + "\n", nil
}
