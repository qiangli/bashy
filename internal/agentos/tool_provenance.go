package agentos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/qiangli/yoke/pkg/fleet"
)

// bashyToolProvenanceFields are the live source-provenance fields the
// fleet registry record for bashy carries: the same State tier vocabulary
// (pristine, modified, modified-protected) with the base release and local
// HEAD each fetched repo's row in `bashy inspect doctor` reports. The
// registry schema has no such keys, so they are computed fresh on every
// show through src.State — never stored, never assumed.
const (
	bashyToolFieldState = "src_state"
	bashyToolFieldRepos = "src_repos"
)

// ensureBashyToolRecord seeds the fleet registry's kind: tool record for
// bashy itself into the local ring when no ring defines it. It is data
// only (identity and launch contract); provenance stays live-computed at
// show time, so the record can never go stale.
func ensureBashyToolRecord() error {
	if _, ok := fleet.New().Tool("bashy"); ok {
		return nil
	}
	exe, _ := os.Executable()
	return fleet.New().SaveTool(fleet.Tool{
		Name:    "bashy",
		Kind:    fleet.ToolKindCLI,
		Display: "bashy — the agent shell (this binary)",
		CLI:     fleet.ToolCLI{Binary: exe},
	})
}

// bashyToolProvenance builds the live provenance overlay for the bashy
// tool record: the aggregate tier plus one per-repo row with the same
// fields doctor reports.
func bashyToolProvenance(home string) (string, []srcRepoProvenance) {
	rows := srcProvenance(home)
	return aggregateSrcState(rows), rows
}

// tryBashyToolShow intercepts `bashy tool show bashy [...]` and renders
// the registry record enriched with live source provenance. It reports
// false when the invocation is not exactly that form, so every other tool
// noun and argument keeps yoke's behavior byte-identical.
func tryBashyToolShow(args []string) (bool, int) {
	var positionals []string
	var asJSON, asYAML bool
	var field string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json" || a == "--json=true":
			asJSON = true
		case a == "--yaml" || a == "--yaml=true":
			asYAML = true
		case a == "--json=false":
			asJSON = false
		case a == "--yaml=false":
			asYAML = false
		case a == "--field" && i+1 < len(args):
			i++
			field = args[i]
		case strings.HasPrefix(a, "--field="):
			field = strings.TrimPrefix(a, "--field=")
		case a == "-h" || a == "--help" || a == "help":
			return false, 0
		case strings.HasPrefix(a, "-"):
			return false, 0
		default:
			positionals = append(positionals, a)
		}
	}
	if len(positionals) != 2 || positionals[0] != "show" || positionals[1] != "bashy" {
		return false, 0
	}
	if asJSON && asYAML {
		fmt.Fprintln(os.Stderr, "bashy tool show: --json and --yaml are mutually exclusive")
		return true, 2
	}
	if err := ensureBashyToolRecord(); err != nil {
		fmt.Fprintf(os.Stderr, "bashy tool show: %v\n", err)
		return true, 1
	}
	state, rows := bashyToolProvenance("")
	if field != "" {
		return true, emitBashyToolField(field, state, rows)
	}
	// Render yoke's own record first, then the live overlay, so the
	// registry output stays canonical and provenance is additive.
	raw, err := runYokeToolShow(asJSON, asYAML)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy tool show: %v\n", err)
		return true, 1
	}
	switch {
	case asJSON:
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			fmt.Fprintf(os.Stderr, "bashy tool show: %v\n", err)
			return true, 1
		}
		doc[bashyToolFieldState] = state
		doc[bashyToolFieldRepos] = toolReposJSON(rows)
		out, _ := json.MarshalIndent(doc, "", "  ")
		fmt.Println(string(out))
		return true, 0
	case asYAML:
		var doc map[string]any
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			fmt.Fprintf(os.Stderr, "bashy tool show: %v\n", err)
			return true, 1
		}
		if doc == nil {
			doc = map[string]any{}
		}
		doc[bashyToolFieldState] = state
		doc[bashyToolFieldRepos] = toolReposJSON(rows)
		out, err := yaml.Marshal(doc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bashy tool show: %v\n", err)
			return true, 1
		}
		fmt.Print(string(out))
		return true, 0
	default:
		fmt.Print(string(raw))
		printToolProvenance(os.Stdout, state, rows)
		return true, 0
	}
}

func toolReposJSON(rows []srcRepoProvenance) []any {
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{
			"repo": r.Repo, "state": r.State, "base": r.Base, "local": r.Local,
		})
	}
	return out
}

func printToolProvenance(w io.Writer, state string, rows []srcRepoProvenance) {
	fmt.Fprintf(w, "src_state: %s\n", state)
	for _, r := range rows {
		base, local := r.Base, r.Local
		if base == "" {
			base = "unknown"
		}
		if local == "" {
			local = "unknown"
		}
		fmt.Fprintf(w, "src_repo: %s state=%s base=%s local=%s\n", r.Repo, r.State, shortSHA12(base), shortSHA12(local))
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "src_repos: (none fetched)")
	}
}

func emitBashyToolField(field, state string, rows []srcRepoProvenance) int {
	switch field {
	case bashyToolFieldState:
		fmt.Println(state)
		return 0
	case bashyToolFieldRepos:
		out, _ := json.MarshalIndent(toolReposJSON(rows), "", "  ")
		fmt.Println(string(out))
		return 0
	default:
		// Any registry-native field stays yoke's answer.
		raw, code := runYokeToolShowField(field)
		fmt.Print(string(raw))
		return code
	}
}

// runYokeToolShow executes yoke's own `tool show bashy` into a buffer so
// bashy can overlay provenance without reimplementing the renderer.
func runYokeToolShow(asJSON, asYAML bool) ([]byte, error) {
	cmd := fleet.NewToolsCmd()
	var args []string
	if asJSON {
		args = []string{"show", "bashy", "--json"}
	} else if asYAML {
		args = []string{"show", "bashy", "--yaml"}
	} else {
		args = []string{"show", "bashy"}
	}
	return runFleetCapture(cmd, args)
}

func runYokeToolShowField(field string) ([]byte, int) {
	cmd := fleet.NewToolsCmd()
	raw, err := runFleetCapture(cmd, []string{"show", "bashy", "--field", field})
	if err != nil {
		return []byte(err.Error() + "\n"), 1
	}
	return raw, 0
}

func runFleetCapture(cmd *cobra.Command, args []string) ([]byte, error) {
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	setCobraArgs(cmd, args)
	if err := cmd.Execute(); err != nil {
		return buf.Bytes(), err
	}
	return buf.Bytes(), nil
}
