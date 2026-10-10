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
		_, agent := detect()
		if agentic || agent {
			return "skill", nil
		}
		return "classic", nil
	}
	switch format {
	case "skill", "classic", "mcp":
		return format, nil
	}
	return "", fmt.Errorf("unknown help format %q (want skill, classic, or mcp)", format)
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
	return exec.CommandContext(ctx, bashySelfPath(), args...)
}

func classicHelp(args []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := classicHelpCommand(ctx, args)
	cmd.Env = append(os.Environ(), "BASHY_HELP_FORMAT=classic")
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
	entry, ok := atlas.Lookup(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "bashy help: no atlas entry for %q\n", name)
		return 2, true
	}
	var output string
	if format == "mcp" {
		var definition *mcpsdk.Tool
		definition, err = helpMCPTool(name, entry)
		if err == nil {
			var data []byte
			data, err = json.MarshalIndent(definition, "", "  ")
			output = string(data) + "\n"
		}
	} else {
		output, err = classicHelp(request)
		if err == nil && format == "skill" {
			output, err = skillHelp(name, output, entry, "")
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy help:", err)
		return 1, true
	}
	fmt.Fprint(os.Stdout, output)
	return 0, true
}

// skillHelp converts existing Cobra or hand-written help, never regenerating
// the classic source. Long prose preceding Usage remains prose; an optional
// canonical long document replaces that preamble.
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
	inSections := false
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
			inSections = true
			fmt.Fprintf(&b, "## %s\n", section)
			if strings.TrimSpace(tail) != "" {
				fmt.Fprintf(&b, "  %s\n", strings.TrimSpace(tail))
			}
		} else if longDoc == "" || inSections {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	fmt.Fprintf(&b, "\n```json mcp\n%s\n```\n", data)
	return b.String(), nil
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
