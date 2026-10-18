// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"fmt"
	"strings"
)

// scriptRecipeMarkdown projects the existing help=script examples into a
// bounded, copyable recipe. The script renderer remains the sole owner of
// command spellings; this function only pairs its task and command lines.
func scriptRecipeMarkdown(name, script string) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "# bashy %s recipes\n\nSource: `bashy help --format=script %s`\n\n", name, name)
	lines := strings.Split(script, "\n")
	count := 0
	inExamples := false
	for i := 0; i+1 < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "# Examples derived from ") {
			inExamples = true
			continue
		}
		if line == "# Usage" {
			break
		}
		if !inExamples {
			continue
		}
		if !strings.HasPrefix(line, "# ") || !strings.HasPrefix(strings.TrimSpace(lines[i+1]), "# ") {
			continue
		}
		task := strings.TrimPrefix(line, "# ")
		command := strings.TrimPrefix(strings.TrimSpace(lines[i+1]), "# ")
		// Task lines are emitted by scriptHelp as "intent (gloss)". The
		// next line must be one command, never another comment or choice.
		if !strings.HasSuffix(task, ")") || !strings.Contains(task, " (") ||
			command == "" || strings.Contains(command, "\n") {
			continue
		}
		intent, _, _ := strings.Cut(task, " (")
		if intent == "" {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n```bash\n%s\n```\n\n", intent, command)
		count++
		i++
	}
	if count == 0 {
		return "", fmt.Errorf("no script examples for %s", name)
	}
	return b.String(), nil
}
