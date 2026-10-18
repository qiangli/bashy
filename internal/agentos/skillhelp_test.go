package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/atlas"
	"github.com/qiangli/yoke/pkg/fleet"
)

// Exercise the real dispatcher, including its os.Exit boundary.
func TestSkillHelpProcess(t *testing.T) {
	if os.Getenv("BASHY_TEST_SKILLHELP") != "1" {
		return
	}
	exe := os.Args[0]
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"bashy"}, os.Args[i+1:]...)
			break
		}
	}
	classicHelpCommand = func(ctx context.Context, args []string) *exec.Cmd {
		return exec.CommandContext(ctx, exe, append([]string{"-test.run=^TestSkillHelpProcess$", "--"}, args...)...)
	}
	dispatch()
	os.Exit(0)
}

func skillHelpProcess(t *testing.T, format string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestSkillHelpProcess$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "BASHY_TEST_SKILLHELP=1", "BASHY_HELP_FORMAT="+format)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v: %v (%s)", args, err, string(out)+stderr.String())
	}
	return out
}

func TestSkillHelpExplicit(t *testing.T) {
	out := skillHelpProcess(t, "classic", "help", "--format", "skill", "sprint")
	if !bytes.HasPrefix(out, []byte("---\nname: bashy-sprint\n")) || !bytes.Contains(out, []byte("```json mcp\n")) {
		t.Fatalf("not skill help: %s", out)
	}
}

func TestSkillHelpClassicSnapshots(t *testing.T) {
	for _, name := range []string{"sprint", "weave", "model"} {
		t.Run(name, func(t *testing.T) {
			got := skillHelpProcess(t, "classic", name, "--help")
			path := filepath.Join("testdata", "skillhelp", name+".classic")
			if os.Getenv("BASHY_UPDATE_SKILL_GOLDEN") == "1" {
				if err := os.WriteFile(path, got, 0644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
			clearHelpAgentMarkers(t)
			human := skillHelpProcess(t, "", name, "--help")
			if !bytes.Equal(human, want) {
				t.Fatalf("human classic %s changed", name)
			}
			explicit := skillHelpProcess(t, "skill", "help", "--format", "classic", name)
			if !bytes.Equal(explicit, want) {
				t.Fatalf("explicit classic %s changed", name)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("classic %s changed", name)
			}
		})
	}
}

func TestSkillHelpMCP(t *testing.T) {
	out := skillHelpProcess(t, "classic", "help", "--format=mcp", "sprint")
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["name"] != "sprint" || got["inputSchema"] == nil {
		t.Fatalf("bad MCP object %s", out)
	}
	// Explicit help discloses schemas; discovery keeps only the bounded summary.
	description := got["description"].(string)
	summary, schemas, ok := strings.Cut(description, "\nCLI flag schemas by command path (pflag types/defaults; invoke via args): ")
	if !ok || !strings.Contains(schemas, "sprint add") {
		t.Fatal("explicit MCP help lacks per-path schemas")
	}
	got["description"] = summary
	advertised := skillHelpProcess(t, "classic", "mcp", "tools", "--tools", "sprint", "--json")
	var env struct {
		Definitions []json.RawMessage `json:"definitions"`
	}
	if err := json.Unmarshal(advertised, &env); err != nil {
		t.Fatal(err)
	}
	for _, raw := range env.Definitions {
		var item map[string]any
		_ = json.Unmarshal(raw, &item)
		if item["name"] == "sprint" {
			a, _ := json.Marshal(item)
			b, _ := json.Marshal(got)
			if !bytes.Equal(a, b) {
				t.Fatal("MCP object differs")
			}
			return
		}
	}
	t.Fatal("sprint definition absent")
}

func TestSkillHelpSelectionEnv(t *testing.T) {
	for _, format := range []string{"skill", "mcp"} {
		out := skillHelpProcess(t, format, "sprint", "--help")
		if format == "skill" && !strings.HasPrefix(string(out), "---\n") {
			t.Fatal("skill env ignored")
		}
		if format == "mcp" && !json.Valid(out) {
			t.Fatal("mcp env ignored")
		}
	}
}

// Clear only tool-detection markers in this test process. BASHY_AGENT remains
// the injected identity; it is not a tool-detection input.
func clearHelpAgentMarkers(t *testing.T) {
	t.Helper()
	for _, key := range fleet.MarkerEnvs() {
		t.Setenv(key, "")
	}
	t.Setenv("BASHY_AGENTIC", "")
}

func TestSkillHelpDetection(t *testing.T) {
	clearHelpAgentMarkers(t)
	for _, key := range []string{"BASHY_AGENTIC", "AI_AGENT"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, "1")
			got := skillHelpProcess(t, "", "sprint", "--help")
			if !bytes.HasPrefix(got, []byte("---\nname: bashy-sprint\n")) {
				t.Fatalf("%s did not select skill", key)
			}
		})
	}
}

func TestSkillHelpSelection(t *testing.T) {
	for _, tc := range []struct {
		explicit, env     string
		agentic, detected bool
		want              string
	}{
		{"", "", false, false, "classic"},
		{"", "", false, true, "skill"},
		{"", "", true, false, "skill"},
		{"", "classic", true, true, "classic"},
		{"", "mcp", false, false, "mcp"},
		{"classic", "skill", true, true, "classic"},
		{"skill", "classic", false, false, "skill"},
		{"mcp", "invalid", true, true, "mcp"},
		{"invalid", "classic", false, false, ""},
		{"", "invalid", false, false, ""},
	} {
		got, err := helpFormat(tc.explicit, tc.env, tc.agentic, func() (string, bool) { return "test-tool", tc.detected })
		if got != tc.want || (err != nil) != (tc.want == "") {
			t.Fatalf("%+v: got %q, %v", tc, got, err)
		}
	}
}

func TestSkillHelpGoldens(t *testing.T) {
	for _, name := range []string{"sprint", "mcp"} {
		t.Run(name, func(t *testing.T) {
			got := skillHelpProcess(t, "classic", "help", "--format", "skill", name)
			path := filepath.Join("testdata", "skillhelp", name+".skill")
			if os.Getenv("BASHY_UPDATE_SKILL_GOLDEN") == "1" {
				if err := os.WriteFile(path, got, 0644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
			if !bytes.Equal(got, want) {
				t.Fatalf("skill golden %s changed:\n%s", name, got)
			}
		})
	}
}

// The canonical doc leads; the command's own preamble still follows it
// rather than being dropped, since a command's preamble can carry
// runtime-assembled content (sprint's owner-accountability contract) that a
// static moved doc does not repeat — see skillHelp's doc comment.
func TestSkillHelpLongDocument(t *testing.T) {
	entry, _ := atlas.Lookup("sprint")
	got, err := skillHelp("sprint", "Old preamble.\n\nUsage:\n  sprint [flags]\n\nAvailable Commands:\n  show  Read a card\n\nFlags:\n  -h help\n", entry, "Canonical long doc.")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Canonical long doc.", "Old preamble.", "## Usage\n  sprint [flags]", "## Commands\n  show", "## Flags\n  -h"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if strings.Index(got, "Canonical long doc.") > strings.Index(got, "Old preamble.") {
		t.Fatal("canonical long doc must lead, ahead of the command's own preamble")
	}
}

func TestSkillHelpLeavesClassicSurfaces(t *testing.T) {
	t.Setenv("BASHY_HELP_FORMAT", "skill")
	for _, args := range [][]string{{"echo", "--help"}, {"printf", "--help"}, {"ls", "--help"}, {"cd", "--help"}, {"sprint", "--", "--help"}, {"sprint", "show", "--help"}} {
		if _, handled := dispatchFormattedHelp(args); handled {
			t.Fatalf("intercepted %v", args)
		}
	}
}

func TestSkillHelpMCPMatchesServer(t *testing.T) {
	ringDir(t)
	ctx, cs, _ := mcpFrontClient(t, mcpVerbOptions(t, "", "all"))
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sprint", "weave", "model"} {
		entry, _ := atlas.Lookup(name)
		got, err := helpMCPTool(name, entry)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range list.Tools {
			if item.Name == name {
				found = true
				a, _ := json.Marshal(item)
				b, _ := json.Marshal(got)
				if !bytes.Equal(a, b) {
					t.Fatalf("%s differs from live server", name)
				}
			}
		}
		if !found {
			t.Fatalf("%s absent from live tools/list", name)
		}
	}
}

func TestScriptHelpFormat(t *testing.T) {
	for _, tc := range []struct {
		explicit string
		want     string
		isErr    bool
	}{
		{"script", "script", false},
		{"skill", "skill", false},
		{"classic", "classic", false},
		{"mcp", "mcp", false},
		{"unknown", "", true},
		{"", "classic", false},
	} {
		env := ""
		if tc.explicit == "" {
			clearHelpAgentMarkers(t)
		}
		got, err := helpFormat(tc.explicit, env, false, func() (string, bool) { return "", false })
		if tc.isErr && err == nil {
			t.Fatalf("helpFormat(%q) expected error", tc.explicit)
		}
		if !tc.isErr && got != tc.want {
			t.Fatalf("helpFormat(%q)=%q want %q err %v", tc.explicit, got, tc.want, err)
		}
	}
}

func TestScriptHelpExplicit(t *testing.T) {
	out := skillHelpProcess(t, "classic", "help", "--format=script", "sprint")
	if !bytes.HasPrefix(out, []byte("#!/usr/bin/env bash\n")) {
		t.Fatalf("script help lacks shebang: %s", out)
	}
	if !bytes.Contains(out, []byte("bashy sprint")) {
		t.Fatalf("script help lacks verb header: %s", out)
	}
	// Explicit skill/classic/mcp must remain exactly as before.
	skill := skillHelpProcess(t, "classic", "help", "--format", "skill", "sprint")
	if !bytes.HasPrefix(skill, []byte("---\nname: bashy-sprint\n")) {
		t.Fatal("skill format regressed")
	}
	classic := skillHelpProcess(t, "classic", "help", "--format", "classic", "sprint")
	if bytes.HasPrefix(classic, []byte("#!/usr/bin/env bash")) {
		t.Fatal("classic format returned script")
	}
	mcp := skillHelpProcess(t, "classic", "help", "--format=mcp", "sprint")
	if !json.Valid(mcp) {
		t.Fatal("mcp format regressed")
	}
}

func TestScriptHelpExampleFirst(t *testing.T) {
	out := string(skillHelpProcess(t, "classic", "help", "--format=script", "weave"))
	// Examples must appear before Usage prose (example-first).
	rows := TriggerRows()
	if len(rows) == 0 {
		t.Fatal("no trigger rows")
	}
	firstExample := rows[0].Example
	idxExample := strings.Index(out, firstExample)
	idxUsage := strings.Index(out, "# Usage")
	if idxExample < 0 {
		t.Fatalf("script output lacks first example %q", firstExample)
	}
	if idxUsage >= 0 && idxExample > idxUsage {
		t.Fatalf("example-first violated: example at %d after usage at %d", idxExample, idxUsage)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "#!/usr/bin/env bash") {
		t.Fatal("script does not start with shebang")
	}
}

func TestScriptHelpOnePerTask(t *testing.T) {
	entry, _ := atlas.Lookup("sprint")
	out, err := scriptHelp("sprint", "Usage:\n  sprint [flags]\n", entry, "")
	if err != nil {
		t.Fatal(err)
	}
	rows := TriggerRows()
	// One exact command per task: each Example appears once as a comment,
	// so executing the help text cannot run every example.
	lines := strings.Split(out, "\n")
	lineSet := map[string]int{}
	for _, l := range lines {
		lineSet[strings.TrimSpace(l)]++
	}
	for _, r := range rows {
		example := strings.TrimSpace(r.Example)
		if lineSet["# "+example] != 1 {
			t.Errorf("example %q appears %d times, want 1", example, lineSet["# "+example])
		}
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			t.Fatalf("script help has executable line %q", line)
		}
	}
}

func TestScriptHelpNoProseDecisions(t *testing.T) {
	out := string(skillHelpProcess(t, "classic", "help", "--format=script", "sprint"))
	lower := strings.ToLower(out)
	// Script must not contain decision prose — single spelling, no "choose"/"or" alternatives.
	for _, phrase := range []string{"missing options exit 2 here", "choose", " or "} {
		// " or " is too broad: allow only the decision-like "Missing options" check.
		if phrase == " or " {
			continue
		}
		if strings.Contains(lower, phrase) {
			t.Fatalf("script help contains prose decision %q", phrase)
		}
	}
	if strings.Contains(out, "Missing options exit 2 here") {
		t.Fatalf("script help must not contain skill footer decision prose")
	}
}

func TestScriptHelpNoInputFilled(t *testing.T) {
	out := string(skillHelpProcess(t, "classic", "help", "--format=script", "sprint"))
	if !strings.Contains(out, "--no-input") {
		t.Fatalf("script help lacks --no-input where appropriate: %s", out)
	}
	// The vocabulary's --no-input example must be present verbatim.
	for _, r := range TriggerRows() {
		if r.Trigger == "--no-input" {
			if !strings.Contains(out, r.Example) {
				t.Fatalf("script lacks vocabulary --no-input example %q", r.Example)
			}
		}
	}
}

func TestScriptHelpDerivedFromVocabulary(t *testing.T) {
	entry, _ := atlas.Lookup("sprint")
	classic := "Usage:\n  sprint [flags]\n"
	before, err := scriptHelp("sprint", classic, entry, "")
	if err != nil {
		t.Fatal(err)
	}
	// Mutating a declaration changes the output — nothing is hand-copied.
	saved := lensTriggerDecls
	defer func() { lensTriggerDecls = saved }()
	lensTriggerDecls = append(append([]TriggerRow(nil), saved...), TriggerRow{Intent: "script task probe", Trigger: "lens -t probe", Word: "lens -t probe", Gloss: "probe", Example: "lens -t probe unique-script-probe-2140"})
	after, err := scriptHelp("sprint", classic, entry, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(before, "unique-script-probe-2140") {
		t.Fatal("before already contains probe")
	}
	if !strings.Contains(after, "unique-script-probe-2140") || !strings.Contains(after, "lens -t probe") {
		t.Fatalf("scriptHelp does not follow lens declarations: %s", after)
	}
	// Also prove CommonOptionsVocabulary mutation propagates (via TriggerRows).
	savedVocab := CommonOptionsVocabulary
	defer func() { CommonOptionsVocabulary = savedVocab }()
	CommonOptionsVocabulary = append(append([]CommonOption(nil), savedVocab...), CommonOption{Option: "--probe-opt", Meaning: "probe", Intent: "probe opt", Prior: "test", Example: "bashy adapt --probe-opt -- cmd probe-vocab-2140", Gloss: "probe"})
	after2, err := scriptHelp("sprint", classic, entry, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(after2, "probe-vocab-2140") {
		t.Fatalf("scriptHelp does not follow CommonOptionsVocabulary: %s", after2)
	}
}

func TestScriptHelpViaEnv(t *testing.T) {
	// Env-driven selection parallels skill/mcp tests.
	out := skillHelpProcess(t, "script", "sprint", "--help")
	if !bytes.HasPrefix(out, []byte("#!/usr/bin/env bash")) {
		t.Fatalf("BASHY_HELP_FORMAT=script not selected via env: %s", out)
	}
}
