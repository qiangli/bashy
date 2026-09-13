// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/kb"
)

func isolateKBCommandTest(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("BASHY_KB_DIR", filepath.Join(root, "host-kb"))
	t.Setenv("BASHY_HOME", filepath.Join(root, "home"))
	t.Setenv("BASHY_SKILLS_DIR", filepath.Join(root, "skills"))
	t.Setenv("YCODE_DATA_DIR", filepath.Join(root, "agent-data"))
	if err := os.MkdirAll(filepath.Join(root, "host-kb"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestKBContextRefusesSingleStoreFlags(t *testing.T) {
	for _, flag := range []string{"--dir", "--repo", "--user", "--base-dir"} {
		t.Run(strings.TrimPrefix(flag, "--"), func(t *testing.T) {
			root := isolateKBCommandTest(t)
			t.Chdir(root)
			cmd := kb.NewKBCmd()
			cmd.AddCommand(newKBContextCmd())
			args := []string{"context", "--for", "task", flag}
			if flag == "--dir" || flag == "--base-dir" {
				args = append(args, root)
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "selects ONE kb store") {
				t.Fatalf("%s error = %v, want explicit single-store refusal", flag, err)
			}
		})
	}
}

func TestKBContextSuppressesSingleStoreScopeHeader(t *testing.T) {
	root := isolateKBCommandTest(t)
	t.Chdir(root)
	cmd := kb.NewKBCmd()
	cmd.AddCommand(newKBContextCmd())
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"context", "--for", "missing topic", "--rings", "host", "--forms", "note", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stderr.String(), "kb [") {
		t.Fatalf("context leaked kb single-store scope header: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"context_version": 1`) {
		t.Fatalf("context output missing frozen envelope version: %s", stdout.String())
	}
}

func TestConductorPlanEmitsRepoKBContextOnScratchSprint(t *testing.T) {
	root := isolateKBCommandTest(t)
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	pages := filepath.Join(root, "docs", "kb", "pages")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	page := "---\nform: page\ntype: gotcha\ntitle: Scratch sprint context\ndescription: scratch sprint planning context\nstatus: validated\n---\n\nKeep the PLAN context bounded.\n"
	if err := os.WriteFile(filepath.Join(pages, "scratch-sprint-context.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	cmd := kb.NewKBCmd()
	cmd.AddCommand(newKBContextCmd())
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"context", "--for", "scratch sprint context", "--rings", "repo,host", "--budget", "700"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("conductor PLAN context: %v\nstderr=%s", err, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "- [repo/page]") || !strings.Contains(got, "Scratch sprint context") {
		t.Fatalf("PLAN did not emit the seeded kb context block:\n%s", got)
	}
}

func TestConductorAndHarnessRecipesUseStageVerbs(t *testing.T) {
	isolateKBCommandTest(t)
	repoRoot, ok := findBashySourceRoot(mustGetwd())
	if !ok {
		t.Fatal("cannot locate bashy source root")
	}

	read := func(rel string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(repoRoot, rel))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	conductor := read("skills/conductor/SKILL.md")
	for _, want := range []string{
		`bashy kb context --for "<story title>" --rings repo,host --budget 700`,
		`bashy kb note add --candidate --ring agent --episode "<sprint-run>"`,
		`bashy kb observe --ring agent --episode "<sprint-run>" --kind gate`,
		`bashy kb validate <slug> --ring agent --from-gate <event-id>`,
	} {
		if !strings.Contains(conductor, want) {
			t.Errorf("conductor skill missing staged command %q", want)
		}
	}
	if strings.Contains(conductor, "kb validate <slug> --evidence") {
		t.Fatal("conductor skill still permits runtime promotion without a gate event")
	}

	recipe := read("skills/recipes/kb-stage-wiring.md")
	for _, want := range []string{
		`bashy kb context --for "$PROMPT" --rings repo,host --budget 700`,
		`bashy kb note add --candidate --ring agent --episode "$SESSION" --title "$TITLE" --body "$BODY"`,
		"## Claude Code hooks",
		"## Codex hooks",
		"## Plain `bashy chat` agent",
		"`examples/agent.yaml` is the Y2 reference wiring",
	} {
		if !strings.Contains(recipe, want) {
			t.Errorf("harness recipe missing %q", want)
		}
	}
	if strings.Contains(recipe, "memories:") || strings.Contains(recipe, "stage: bashy.run") {
		t.Fatal("harness recipe duplicated the ycode YAML instead of referencing it")
	}
}
