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
