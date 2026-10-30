package kit

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

// UsageError prints the same error object the HTTP API uses for bad input.
func UsageError(w io.Writer, tool string, asJSON bool, message string) int {
	if asJSON {
		fmt.Fprintln(w, CanonicalJSON(Envelope(tool, map[string]any{"error": map[string]any{"code": "invalid_arg", "message": message}})))
	} else {
		fmt.Fprintln(w, "error:", message)
	}
	return 2
}

// CommonOptions is the desk subset of bashy's long-only common options.
type CommonOptions struct {
	JSON, NoInput, Explain, DryRun, HelpMCP bool
	Fields                                  string
}

// ParseCommon removes common options before a tool's domain parser runs.
func ParseCommon(args []string) ([]string, CommonOptions, error) {
	var opts CommonOptions
	var rest []string
	for i := 0; i < len(args); i++ {
		s := args[i]
		switch {
		case s == "--json":
			opts.JSON = true
		case s == "--no-input":
			opts.NoInput = true
		case s == "--explain":
			opts.Explain = true
		case s == "--dry-run":
			opts.DryRun = true
		case s == "--help=mcp":
			opts.HelpMCP = true
		case s == "--fields":
			if i+1 == len(args) {
				return nil, opts, errors.New("--fields requires a value")
			}
			i++
			opts.Fields = args[i]
		case strings.HasPrefix(s, "--fields="):
			opts.Fields = strings.TrimPrefix(s, "--fields=")
			if opts.Fields == "" {
				return nil, opts, errors.New("--fields requires a value")
			}
		default:
			rest = append(rest, s)
		}
	}
	if opts.Fields != "" && !opts.JSON {
		return nil, opts, errors.New("--fields requires --json")
	}
	return rest, opts, nil
}

// ToolVerbs is the common schema used by help, explain and dry-run.
func ToolVerbs(tool string) []string {
	verbs := map[string][]string{
		"notepad":    {"serve", "add", "show", "set", "list", "open", "share", "close", "export", "join"},
		"form":       {"serve", "add", "show", "set", "list", "open", "share", "close", "reply", "ask", "append", "wait"},
		"chart":      {"serve", "add", "show", "set", "list", "open", "share", "close", "revoke"},
		"whiteboard": {"add", "show", "set", "list", "open", "share", "close"},
		"brainstorm": {"add", "show", "set", "list", "open", "share", "close"},
		"memo":       {"add", "show", "set", "list", "open", "share", "close"},
		"viewer":     {"add", "show", "list", "open", "share", "close", "append"},
	}
	return verbs[tool]
}

// FindVerb finds a declared action even when connection options precede it.
func FindVerb(tool string, args []string) string {
	for _, arg := range args {
		for _, verb := range ToolVerbs(tool) {
			if arg == verb {
				return verb
			}
		}
	}
	return ""
}

// MCPHelp is generated from the same fixed desk action schema as the CLI.
func MCPHelp(tool string) map[string]any {
	return Envelope(tool, map[string]any{
		"name":           tool,
		"verbs":          ToolVerbs(tool),
		"common_options": []string{"--json", "--fields", "--no-input", "--explain", "--dry-run", "--help=mcp"},
	})
}
