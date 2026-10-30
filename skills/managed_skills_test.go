package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
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
	want := []string{"account", "adapt", "collaborate", "common-options", "conductor", "desk", "dks", "feed", "feed-setup", "force-agent-shell", "inbox", "knowledge-transfer", "lens", "sprint", "steward", "supervisor", "transcribe"}
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

	for _, name := range []string{"conductor", "steward", "force-agent-shell", "transcribe"} {
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

// TestDeskSkillDocumentedVerbsValid ensures that every desk command and verb
// documented in desk.md is accepted by the respective tool's CLI/API,
// verified via --help=mcp and --dry-run, and that unknown verbs are rejected.
// TestDeskSkillDocumentedVerbsValid ensures that every desk command, verb,
// and flag documented in desk.md is accepted by the respective tool's CLI/API,
// verified via --help=mcp and --dry-run, and that unknown verbs and undefined
// flags (such as a planted -f or -o) are rejected.
func TestDeskSkillDocumentedVerbsValid(t *testing.T) {
	doc, ok := CommandDoc("desk")
	if !ok {
		t.Fatal("desk command doc not found in embedded skills")
	}

	// Match `bashy <tool> <verb>` invocations
	re := regexp.MustCompile(`(?m)^\s*bashy\s+([a-z0-9_-]+)\s+([a-z0-9_-]+)`)
	matches := re.FindAllStringSubmatch(doc, -1)
	if len(matches) == 0 {
		t.Fatal("no documented desk commands found in desk.md")
	}

	// Runnable tool CLI targets
	type cliTarget struct {
		dir  string
		args []string
	}
	cliTargets := map[string]cliTarget{
		"notepad": {"../examples/desk/notepad", []string{"go", "run", "."}},
		"chart":   {"../examples/desk/chart", []string{"go", "run", "."}},
		"form":    {"../examples/desk/form", []string{"go", "run", "./server"}},
		"viewer":  {"../examples/desk/viewer", []string{"go", "run", "."}},
	}

	mcpVerbs := make(map[string]map[string]bool)
	for tool, target := range cliTargets {
		cmd := exec.Command(target.args[0], append(target.args[1:], "--help=mcp")...)
		cmd.Dir = target.dir
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("failed to run --help=mcp for %s: %v", tool, err)
		}
		var help struct {
			Verbs []string `json:"verbs"`
		}
		if err := json.Unmarshal(out, &help); err != nil {
			t.Fatalf("failed to parse --help=mcp output for %s: %v", tool, err)
		}
		mcpVerbs[tool] = make(map[string]bool)
		for _, v := range help.Verbs {
			mcpVerbs[tool][v] = true
		}

		// Negative test: verify --dry-run rejects an unknown verb with non-zero exit
		badCmd := exec.Command(target.args[0], append(target.args[1:], "--dry-run", "nonexistent_verb_xyz")...)
		badCmd.Dir = target.dir
		if err := badCmd.Run(); err == nil {
			t.Fatalf("%s CLI unexpectedly accepted unknown verb 'nonexistent_verb_xyz' with --dry-run", tool)
		}
	}

	staticVerbs := map[string]map[string]bool{
		"whiteboard": {"add": true, "show": true, "set": true, "list": true, "open": true, "share": true, "close": true, "serve": true, "status": true, "backup": true, "stop": true},
		"brainstorm": {"add": true, "show": true, "set": true, "list": true, "open": true, "share": true, "close": true, "serve": true, "status": true, "backup": true, "restore": true, "stop": true},
	}

	deskTools := map[string]bool{
		"notepad":    true,
		"chart":      true,
		"form":       true,
		"whiteboard": true,
		"brainstorm": true,
		"viewer":     true,
	}

	testedDryRun := make(map[string]bool)
	verifiedCount := 0

	for _, m := range matches {
		tool := m[1]
		verb := m[2]
		if !deskTools[tool] {
			continue
		}

		if target, hasCLI := cliTargets[tool]; hasCLI {
			validVerbs := mcpVerbs[tool]
			if !validVerbs[verb] {
				t.Errorf("desk.md documents unknown verb %q for %s; accepted: %v", verb, tool, validVerbs)
			}
			key := tool + ":" + verb
			if !testedDryRun[key] {
				testedDryRun[key] = true
				dryCmd := exec.Command(target.args[0], append(target.args[1:], "--dry-run", verb)...)
				dryCmd.Dir = target.dir
				out, err := dryCmd.CombinedOutput()
				if err != nil {
					t.Errorf("--dry-run %s on %s failed: %v: %s", verb, tool, err, string(out))
				}
			}
		} else if allowed, ok := staticVerbs[tool]; ok {
			if !allowed[verb] {
				t.Errorf("desk.md documents unknown verb %q for %s", verb, tool)
			}
		}
		verifiedCount++
	}

	if verifiedCount == 0 {
		t.Fatal("expected to verify at least one documented desk command, got 0")
	}

	// Validate documented flags in desk.md
	if errs := validateDeskDocFlags(doc); len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("documented flag error: %s", e)
		}
	}

	// Red/green check: verify that a planted -f flag causes validation failure
	for _, tool := range []string{"form", "chart", "notepad", "viewer", "whiteboard", "brainstorm"} {
		planted := fmt.Sprintf("bashy %s add -f planted.json", tool)
		if errs := validateDeskDocFlags(planted); len(errs) == 0 {
			t.Errorf("planted -f flag for %s unexpectedly passed validation", tool)
		}
	}

	// Negative CLI test: verify tools with flag sets directly reject unknown flag -f
	for _, tool := range []string{"form", "chart"} {
		target := cliTargets[tool]
		cmd := exec.Command(target.args[0], append(target.args[1:], "add", "-f")...)
		cmd.Dir = target.dir
		if err := cmd.Run(); err == nil {
			t.Errorf("%s CLI unexpectedly accepted unknown flag '-f'", tool)
		}
	}
}

// validateDeskDocFlags extracts all flags from documented bashy <tool> commands
// and verifies they are defined in each tool's flag set / allowed options.
func validateDeskDocFlags(doc string) []string {
	commonFlags := map[string]bool{
		"json":     true,
		"fields":   true,
		"no-input": true,
		"explain":  true,
		"dry-run":  true,
		"help":     true,
	}

	// Defined flags per tool according to their FlagSet / CLI options on main.
	// Flags like -f or -o are not in these allowed sets.
	toolAllowedFlags := map[string]map[string]bool{
		"notepad": {
			"title": true, "body": true, "to": true, "format": true,
			"id": true, "key": true, "addr": true, "data": true, "prefix": true, "break": true,
		},
		"viewer": {
			"title": true, "body": true, "template": true, "data": true,
			"sprint": true, "session": true, "page": true, "asset": true,
			"to": true, "output": true, "addr": true, "dir": true, "prefix": true, "key": true, "break": true,
		},
		"form": {
			"addr": true, "data": true, "prefix": true, "title": true, "body": true, "id": true,
			"extract": true, "values": true, "refusals": true, "def": true, "definition": true,
			"respondents": true, "anonymous": true, "close-at": true, "results-visible": true,
			"poll": true, "options": true, "key": true, "timeout": true, "interval": true, "answer": true,
		},
		"chart": {
			"addr": true, "data": true, "prefix": true, "title": true, "body": true, "id": true,
			"key": true, "input": true, "format": true, "widget": true, "template": true, "refresh": true,
		},
		"whiteboard": {
			"addr": true, "prefix": true, "title": true, "body": true, "id": true, "key": true,
		},
		"brainstorm": {
			"addr": true, "prefix": true, "title": true, "body": true, "id": true, "key": true,
		},
	}

	var errors []string
	reLine := regexp.MustCompile(`(?m)^\s*bashy\s+([a-z0-9_-]+)\s+([^#\n]+)`)
	reFlag := regexp.MustCompile(`(^|\s)(--?[a-zA-Z0-9_-]+)`)

	matches := reLine.FindAllStringSubmatch(doc, -1)
	for _, m := range matches {
		tool := m[1]
		allowed, ok := toolAllowedFlags[tool]
		if !ok {
			continue
		}
		cmdRest := m[2]
		flagMatches := reFlag.FindAllStringSubmatch(cmdRest, -1)
		for _, fm := range flagMatches {
			rawFlag := fm[2]
			flagName := strings.TrimLeft(rawFlag, "-")
			if !commonFlags[flagName] && !allowed[flagName] {
				errors = append(errors, tool+" does not define flag "+rawFlag+" (in command: bashy "+tool+" "+strings.TrimSpace(cmdRest)+")")
			}
		}
	}
	return errors
}
