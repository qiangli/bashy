// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/qiangli/bashy/skills"
	"github.com/qiangli/coreutils/tool"
	yokemcp "github.com/qiangli/yoke/mcp"
	"github.com/qiangli/yoke/pkg/atlas"
	"github.com/qiangli/yoke/pkg/fleet"
)

// No terminal heuristic: pipes and terminals use the same agent detection.
func helpFormat(explicit, environment string, agentic bool, detect func() (string, bool)) (string, error) {
	format := explicit
	if format == "" {
		format = environment
	}
	if format == "" {
		if band, ok := helpBand(); ok {
			if band >= 1 && band <= 2 {
				return "script", nil
			}
			if band >= 3 {
				return "skill", nil
			}
		}
		_, agent := detect()
		if agentic || agent {
			return "skill", nil
		}
		return "classic", nil
	}
	switch format {
	case "skill", "classic", "mcp", "script", "dag":
		return format, nil
	}
	return "", fmt.Errorf("unknown help format %q (want skill, classic, mcp, script, or dag)", format)
}

// helpBand returns the identified agent's capability band via the existing
// fleet binding/principal lookup. It uses BASHY_AGENT and fleet.New()
// (binding/principal) — no second registry or new global environment policy.
// Missing or unknown band is reported as not ok so the caller falls through
// to the human/agent defaults.
func helpBand() (int, bool) {
	if helpBandLookup != nil {
		return helpBandLookup()
	}
	return fleetHelpBand()
}

var helpBandLookup func() (int, bool)

func fleetHelpBand() (int, bool) {
	name := strings.TrimSpace(os.Getenv("BASHY_AGENT"))
	if name == "" {
		return 0, false
	}
	cat := fleet.New()
	// Principal lookup is the existing fleet identity check (agent vs person).
	if _, kind, ok := cat.CanonicalPrincipal(name); ok {
		if kind == fleet.KindPerson {
			return 0, false
		}
	}
	// Binding is the existing fleet binding lookup (agent -> tool/model).
	a, _, _, err := cat.Binding(name)
	if err != nil {
		return 0, false
	}
	ladder, err := cat.Ladder()
	if err != nil {
		return 0, false
	}
	standings, _ := cat.Standings(ladder)
	standing, ok := standings[a.Name]
	if !ok {
		return 0, false
	}
	band := standing.Band
	if a.BandSource == fleet.BandCascade && a.Band > 0 {
		band = a.Band
	}
	if band < 1 || band > fleet.MaxBand {
		return 0, false
	}
	return band, true
}

// Only the front-door verb surface is converted. Shell builtins and applets
// retain their own help, including when the format environment is set.
func isHelpVerb(name string) bool {
	if tool.Lookup(name) != nil {
		return false
	}
	_, _, verbs := commandsCatalog()
	return slices.Contains(verbs, name) || slices.Contains(hiddenVerbsCatalog(), name)
}

var classicHelpCommand = func(ctx context.Context, args []string) *exec.Cmd {
	// Help is dispatched by the running CLI. Its binary may have any filename
	// (for example, the e2e build), so PATH lookup through bashySelfPath is
	// insufficient here.
	self := strings.TrimSpace(os.Getenv("BASHY_SELF"))
	if self == "" {
		self, _ = os.Executable()
	}
	if self == "" {
		self = bashySelfPath()
	}
	return exec.CommandContext(ctx, self, args...)
}

func classicHelp(args []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := classicHelpCommand(ctx, args)
	cmd.Env = append(os.Environ(), "BASHY_HELP_FORMAT=classic", "BASHY_HINTS=off")
	// Several hand-written help handlers write to stderr and exit 2. Both
	// streams are help input; a nonzero status with no help remains an error.
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil && len(out) == 0 {
		return "", err
	}
	return string(out), nil
}

func dispatchFormattedHelp(args []string) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	explicit, name := "", ""
	request := []string(nil)
	if args[0] == "help" {
		for i := 1; i < len(args); i++ {
			arg := args[i]
			if arg == "--format" {
				i++
				if i == len(args) {
					fmt.Fprintln(os.Stderr, "bashy help: --format needs a value")
					return 2, true
				}
				explicit = args[i]
				if explicit == "" {
					fmt.Fprintln(os.Stderr, "bashy help: --format needs a value")
					return 2, true
				}
			} else if strings.HasPrefix(arg, "--format=") {
				explicit = strings.TrimPrefix(arg, "--format=")
				if explicit == "" {
					fmt.Fprintln(os.Stderr, "bashy help: --format needs a value")
					return 2, true
				}
			} else if name == "" {
				name = arg
			} else {
				fmt.Fprintln(os.Stderr, "bashy help: expected one command")
				return 2, true
			}
		}
		if !isHelpVerb(name) {
			if explicit != "" {
				fmt.Fprintf(os.Stderr, "bashy help: unknown front-door verb %q\n", name)
				return 2, true
			}
			return 0, false
		}
		request = []string{name, "--help"}
	} else {
		// Root command help only: operands and subcommand help remain owned by
		// their dispatcher, so a literal --help after -- is never intercepted.
		if len(args) != 2 || (args[1] != "--help" && args[1] != "-h") || !isHelpVerb(args[0]) {
			return 0, false
		}
		name = args[0]
		request = args
	}
	format, err := helpFormat(explicit, os.Getenv("BASHY_HELP_FORMAT"), os.Getenv("BASHY_AGENTIC") == "1", fleet.DetectTool)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy help:", err)
		return 2, true
	}
	if format == "classic" && args[0] != "help" {
		return 0, false
	}
	entry, ok := lookupEntry(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "bashy help: no atlas entry for %q\n", name)
		return 2, true
	}
	var output string
	if format == "mcp" {
		var definition *mcpsdk.Tool
		definition, err = detailedHelpMCPTool(name, entry)
		if err == nil {
			var data []byte
			data, err = json.MarshalIndent(definition, "", "  ")
			output = string(data) + "\n"
		}
	} else {
		output, err = classicHelp(request)
		if err == nil {
			switch format {
			case "skill":
				output, err = skillHelp(name, output, entry, commandLongDoc(name))
			case "script":
				output, err = scriptHelp(name, output, entry, commandLongDoc(name))
			case "dag":
				output, err = scriptRecipeDAG(name)
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy help:", err)
		return 1, true
	}
	fmt.Fprint(os.Stdout, output)
	return 0, true
}

// commandLongDoc returns the prose body of a command's moved doc
// (skills/bashy/commands/<name>.md), for use as skillHelp's optional long
// document — stripped of its own frontmatter block, since skillHelp
// synthesizes its own. Returns "" when the command has no moved doc, in
// which case skillHelp falls back to the classic preamble as before.
func commandLongDoc(name string) string {
	doc, ok := skills.CommandDoc(name)
	if !ok {
		return ""
	}
	const fence = "---\n"
	if !strings.HasPrefix(doc, fence) {
		return strings.TrimSpace(doc)
	}
	if end := strings.Index(doc[len(fence):], "\n---\n"); end >= 0 {
		return strings.TrimSpace(doc[len(fence)+end+len("\n---\n"):])
	}
	return strings.TrimSpace(doc)
}

// skillHelp converts existing Cobra or hand-written help, never regenerating
// the classic source. Long prose preceding Usage remains prose; an optional
// canonical long document is shown first, ahead of it, never instead of it —
// a command's own Long can carry runtime-assembled content (sprint's
// owner-accountability contract, appended in newSprintCmd) that a static
// moved doc does not repeat, and dropping it would be a silent content loss
// for exactly the agent audience the canonical doc exists to serve.
func skillHelp(name, classic string, entry atlas.Entry, longDoc string) (string, error) {
	definition, err := helpMCPTool(name, entry)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(definition, "", "  ")
	if err != nil {
		return "", err
	}
	// JSON strings are YAML-compatible scalars, including colons and newlines.
	description, _ := json.Marshal(synopsisOf(name))
	var b strings.Builder
	fmt.Fprintf(&b, "---\nname: bashy-%s\ndescription: %s\n---\n# bashy %s\n\n", name, description, name)
	if longDoc != "" {
		b.WriteString(strings.TrimSpace(longDoc))
		b.WriteString("\n\n")
	}
	for _, line := range strings.Split(strings.TrimRight(classic, "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		heading, tail, hasColon := strings.Cut(trimmed, ":")
		section := ""
		if hasColon {
			switch strings.ToLower(heading) {
			case "usage":
				section = "Usage"
			case "available commands", "commands", "subcommands":
				section = "Commands"
			case "flags", "options":
				section = "Flags"
			case "global flags":
				section = "Global Flags"
			}
		}
		if section != "" {
			fmt.Fprintf(&b, "## %s\n", section)
			if strings.TrimSpace(tail) != "" {
				fmt.Fprintf(&b, "  %s\n", strings.TrimSpace(tail))
			}
		} else {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	fmt.Fprintf(&b, "\n```json mcp\n%s\n```\n", data)
	fmt.Fprintf(&b, "\n## Common options\n%s\n", HelpCommonOptionsFooter(name, declaredLongFlags(name)))
	return b.String(), nil
}

// Explicit MCP help is the existing on-demand channel for full flag metadata.
// Skill help and tools/list retain the bounded discovery description.
func detailedHelpMCPTool(name string, entry atlas.Entry) (*mcpsdk.Tool, error) {
	definition, err := helpMCPTool(name, entry)
	if err != nil {
		return nil, err
	}
	if schemas := cobraSchemas(name); schemas != nil {
		data, err := json.Marshal(schemas)
		if err != nil {
			return nil, err
		}
		definition.Description += "\nCLI flag schemas by command path (pflag types/defaults; invoke via args): " + string(data)
	}
	return definition, nil
}

func helpMCPTool(name string, entry atlas.Entry) (*mcpsdk.Tool, error) {
	command := mcpVerbCommand(name, entry)
	// Registered commands carry richer schemas; retain the server's adapter.
	for _, registered := range mcpRegisteredCommands() {
		if registered.Name == name {
			command = registered
			break
		}
	}
	definitions, err := mcpToolDefinitions(yokemcp.Options{Registered: func() []yokemcp.RegisteredCommand { return []yokemcp.RegisteredCommand{command} }})
	if err != nil {
		return nil, err
	}
	for _, definition := range definitions {
		if definition.Name == name {
			return definition, nil
		}
	}
	return nil, fmt.Errorf("no MCP definition for %s", name)
}

// Ask the actual server builder for its wire objects. This preserves SDK
// schema normalization, annotations and future yoke changes without copying
// the MCP schema generator. The in-memory session runs no command or listener.
func mcpToolDefinitions(opts yokemcp.Options) ([]*mcpsdk.Tool, error) {
	srv, err := yokemcp.BuildServer("bashy", "help", opts)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st, ct := mcpsdk.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		return nil, err
	}
	defer ss.Close()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "bashy-help", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		return nil, err
	}
	defer cs.Close()
	var result []*mcpsdk.Tool
	params := &mcpsdk.ListToolsParams{}
	for {
		page, err := cs.ListTools(ctx, params)
		if err != nil {
			return nil, err
		}
		result = append(result, page.Tools...)
		if page.NextCursor == "" {
			break
		}
		params.Cursor = page.NextCursor
	}
	return result, nil
}

// scriptHelp renders the Bash recipe. A task selector executes exactly one
// command from the S9 trigger rows. A declaration change alters this output
// and the dag.md projection together.
func scriptHelp(name, classic string, _ atlas.Entry, _ string) (string, error) {
	tasks, err := scriptRecipeTasks()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\n")
	fmt.Fprintf(&b, "# bashy %s recipes; save as a .sh file and run with one task name.\n", name)
	b.WriteString("# Set the RECIPE_* variables named by that task before running.\n")
	b.WriteString("case \"${1:-}\" in\n")
	for _, task := range tasks {
		fmt.Fprintf(&b, "  %s) # %s\n    %s\n    ;;\n", task.name, task.intent, task.command)
	}
	b.WriteString("  *) printf 'Available tasks:\\n' >&2\n")
	for _, task := range tasks {
		fmt.Fprintf(&b, "     printf '  %s\\n' >&2\n", task.name)
	}
	b.WriteString("     exit 2\n     ;;\nesac\n")
	// Ensure the short usage follows the examples (example-first) but keep it
	// as a comment so the file remains an executable script with no ambiguity.
	b.WriteString("\n# Usage\n")
	foundUsage := false
	for _, line := range strings.Split(classic, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "usage:") || strings.HasPrefix(trimmed, "bashy "+name) || strings.HasPrefix(trimmed, name+" ") {
			fmt.Fprintf(&b, "# %s\n", trimmed)
			foundUsage = true
			break
		}
	}
	if !foundUsage {
		// Fallback: first non-empty line of classic help.
		for _, line := range strings.Split(strings.TrimSpace(classic), "\n") {
			if t := strings.TrimSpace(line); t != "" {
				fmt.Fprintf(&b, "# %s\n", t)
				break
			}
		}
	}
	b.WriteString("# See `bashy help common-options` for vocabulary.\n")
	return b.String(), nil
}

// declaredLongFlags is the set of long flag names the command itself declares
// (derived Cobra schema); empty for commands without one.
func declaredLongFlags(name string) map[string]bool {
	return declaredFlagSet(cobraSchemas(name)[name])
}
