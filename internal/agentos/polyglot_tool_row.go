package agentos

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/toolcmd"
	"gopkg.in/yaml.v3"
	"mvdan.cc/sh/v3/polyglot"
)

// A tool fence exposes the vendor commands already declared on a fleet tool.
// The run path is pkg/toolcmd, including its chat admission, budget and usage
// accounting. No fence-defined executable or direct provider API is accepted.
func init() { polyglot.RegisterLanguage(toolFenceRow()) }

func toolFenceRow() polyglot.Language {
	return polyglot.Language{Canonical: "tool", Text: true, InterpretedOnly: true,
		NewRuntime: func(cfg polyglot.RuntimeConfig) polyglot.LanguageRuntime {
			return newToolFenceRuntime(cfg, fleet.New(), toolcmd.Run)
		},
	}
}

type toolFenceRun func(context.Context, fleet.Tool, fleet.ToolCommand, string, toolcmd.Options) (toolcmd.Result, error)

func newToolFenceRuntime(cfg polyglot.RuntimeConfig, catalog *fleet.Catalog, run toolFenceRun) polyglot.Embedded {
	return polyglot.Embedded{RuntimeName: "tool",
		AnalyzeFunc: func(ctx context.Context, source string) ([]polyglot.Export, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			tool, err := toolFenceBinding(catalog, source)
			if err != nil {
				return nil, err
			}
			if _, err := toolFenceAgent(catalog, tool); err != nil {
				return nil, err
			}
			if len(tool.Commands) == 0 {
				return nil, fmt.Errorf("tool fence: %q declares no commands", tool.Name)
			}
			if errs, _ := tool.ValidateCommands(); len(errs) > 0 {
				return nil, fmt.Errorf("tool fence: %w", errors.Join(errs...))
			}
			out := make([]polyglot.Export, 0, len(tool.Commands))
			for _, command := range tool.Commands {
				if len(command.Effects) == 0 {
					return nil, fmt.Errorf("tool fence: %s:%s must declare effects", tool.Name, command.Name)
				}
				out = append(out, polyglot.Export{Name: command.Name, Agentic: true,
					Effects:   toolFenceEffects(command.Effects),
					Signature: polyglot.Signature{Params: []string{"string"}, Results: []string{"string", "error"}}})
			}
			return out, nil
		},
		CallFunc: func(ctx context.Context, plan polyglot.Plan, method string, args []any, kwargs map[string]any) (polyglot.CallResult, error) {
			if len(args) != 1 || len(kwargs) != 0 {
				return polyglot.CallResult{}, fmt.Errorf("tool.%s requires one argument string", method)
			}
			arg, ok := args[0].(string)
			if !ok {
				return polyglot.CallResult{}, fmt.Errorf("tool.%s requires a string", method)
			}
			tool, err := toolFenceBinding(catalog, plan.Source)
			if err != nil {
				return polyglot.CallResult{}, err
			}
			command, ok := tool.Command(method)
			if !ok {
				return polyglot.CallResult{}, fmt.Errorf("tool fence: %s has no command %q", tool.Name, method)
			}
			if len(command.Effects) == 0 {
				return polyglot.CallResult{}, fmt.Errorf("tool fence: %s:%s must declare effects", tool.Name, method)
			}
			agent, err := toolFenceAgent(catalog, tool)
			if err != nil {
				return polyglot.CallResult{}, err
			}
			result, err := run(ctx, tool, command, arg, toolcmd.Options{Agent: agent, Catalog: catalog, Dir: cfg.CallerDir()})
			if err == nil && result.Outcome != toolcmd.OutcomeSuccess {
				err = fmt.Errorf("tool.%s: outcome %s (exit %d): %s", method, result.Outcome, result.ExitCode, result.Error)
			}
			return polyglot.CallResult{Value: result.Text}, err
		},
	}
}

func toolFenceAgent(catalog *fleet.Catalog, tool fleet.Tool) (string, error) {
	agents, errs := catalog.Agents()
	if len(errs) > 0 {
		return "", fmt.Errorf("tool fence: catalog agents: %w", errors.Join(errs...))
	}
	var matches []string
	for _, agent := range agents {
		_, boundTool, _, err := catalog.Binding(agent.Name)
		if err == nil && boundTool.Name == tool.Name {
			matches = append(matches, agent.Name)
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("tool fence: no registered agent binding for %q", tool.Name)
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("tool fence: ambiguous agent bindings for %q: %s", tool.Name, strings.Join(matches, ", "))
	}
	return matches[0], nil
}

func toolFenceEffects(declared []string) []string {
	out := append([]string(nil), declared...)
	seen := map[string]bool{}
	for _, effect := range out {
		seen[effect] = true
	}
	// toolcmd launches an agentic CLI, even when its vendor command only reads
	// project state. These are effects of that launch and must reach @guard.
	for _, effect := range []string{"exec", "net", "spend"} {
		if !seen[effect] {
			out = append(out, effect)
		}
	}
	return out
}

func toolFenceBinding(catalog *fleet.Catalog, source string) (fleet.Tool, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return fleet.Tool{}, errors.New("tool fence: empty definition")
	}
	if tool, ok := catalog.Tool(source); ok {
		return tool, nil
	}
	name := source
	if strings.Contains(source, "\n") || strings.HasPrefix(source, "name:") || strings.HasPrefix(source, "kit:") {
		// ParseTool is the fleet parser, including the established kit/type
		// spellings. The row never invents a second tool document format.
		parsed, err := fleet.ParseTool("", []byte(source), nil)
		if err != nil {
			return fleet.Tool{}, fmt.Errorf("tool fence: invalid fleet definition: %w", err)
		}
		name = parsed.Name
		if name == "" {
			return fleet.Tool{}, errors.New("tool fence: definition requires name or kit")
		}
		var supplied map[string]any
		if err := yaml.Unmarshal([]byte(source), &supplied); err != nil {
			return fleet.Tool{}, err
		}
		for key := range supplied {
			switch key {
			case "name", "kit", "kind", "type", "aliases", "display", "hidden", "cli", "quirks", "harness", "commands":
			default:
				return fleet.Tool{}, fmt.Errorf("tool fence: unknown fleet field %q", key)
			}
		}
	}
	tool, ok := catalog.Tool(name)
	if !ok {
		return fleet.Tool{}, fmt.Errorf("tool fence: tool %q is not registered", name)
	}
	if strings.Contains(source, "\n") || strings.HasPrefix(source, "name:") || strings.HasPrefix(source, "kit:") {
		var supplied map[string]any
		if err := yaml.Unmarshal([]byte(source), &supplied); err != nil {
			return fleet.Tool{}, err
		}
		if legacy, ok := supplied["kit"]; ok {
			if _, canonical := supplied["name"]; !canonical {
				supplied["name"] = legacy
			}
			delete(supplied, "kit")
		}
		if legacy, ok := supplied["type"]; ok {
			if _, canonical := supplied["kind"]; !canonical {
				supplied["kind"] = legacy
			}
			delete(supplied, "type")
		}
		if err := fleetFenceFieldsMatch(supplied, "tool", tool); err != nil {
			return fleet.Tool{}, err
		}
	}
	return tool, nil
}
