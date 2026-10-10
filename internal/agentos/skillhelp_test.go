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
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v: %v (%s)", args, err, out)
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
			if os.Getenv("BASHY_UPDATE_CLASSIC") == "1" {
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, got, 0644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
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
