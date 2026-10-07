package agentos

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/qiangli/yoke/pkg/chat"
	"github.com/qiangli/yoke/pkg/fleet"
	"gopkg.in/yaml.v3"
	"mvdan.cc/sh/v3/polyglot"
)

// The agent row delegates exclusively to the governed chat one-shot harness.
// The engine sees only the method contract, never a model or its credentials.
func init() { polyglot.RegisterLanguage(agentFenceRow()) }

func agentFenceRow() polyglot.Language {
	return polyglot.Language{
		Canonical: "agent", Text: true, InterpretedOnly: true,
		NewRuntime: func(cfg polyglot.RuntimeConfig) polyglot.LanguageRuntime {
			return newAgentFenceRuntime(cfg, fleet.New(), chat.Invoke)
		},
	}
}

type agentFenceInvoke func(context.Context, chat.Options, chat.Runner) (chat.Result, error)

func newAgentFenceRuntime(cfg polyglot.RuntimeConfig, catalog *fleet.Catalog, invoke agentFenceInvoke) polyglot.Embedded {
	// A module serializes its calls. The mutex also protects Close and keeps a
	// script's singleton clone from being invoked concurrently through aliases.
	var mu sync.Mutex
	clones := map[string]string{}
	return polyglot.Embedded{
		RuntimeName: "agent",
		AnalyzeFunc: func(ctx context.Context, source string) ([]polyglot.Export, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if _, _, err := agentFenceBinding(catalog, source); err != nil {
				return nil, err
			}
			return []polyglot.Export{{Name: "run", Agentic: true, Effects: []string{"exec", "net", "spend"}, Signature: polyglot.Signature{Params: []string{"string"}, Results: []string{"string", "error"}}}}, nil
		},
		CallFunc: func(ctx context.Context, plan polyglot.Plan, method string, args []any, kwargs map[string]any) (polyglot.CallResult, error) {
			mu.Lock()
			defer mu.Unlock()
			if err := ctx.Err(); err != nil {
				return polyglot.CallResult{}, err
			}
			if method != "run" || len(args) != 1 || len(kwargs) != 0 {
				return polyglot.CallResult{}, errors.New("agent.run requires one instruction string")
			}
			instruction, ok := args[0].(string)
			if !ok || strings.TrimSpace(instruction) == "" {
				return polyglot.CallResult{}, errors.New("agent.run requires a nonempty instruction string")
			}
			name := clones[plan.ID]
			if name == "" {
				parent, prefix, err := agentFenceBinding(catalog, plan.Source)
				if err != nil {
					return polyglot.CallResult{}, err
				}
				var id [12]byte
				if _, err := rand.Read(id[:]); err != nil {
					return polyglot.CallResult{}, err
				}
				name = "fence-" + hex.EncodeToString(id[:])
				clone, err := catalog.CloneAgent(parent, name, true, "Bash# agent fence")
				if err != nil {
					return polyglot.CallResult{}, err
				}
				if prefix != "" {
					standing := ""
					if clone.Instruction != nil {
						standing = clone.Instruction.Content
					}
					clone.Instruction = &fleet.AgentInstruction{Content: strings.TrimSpace(standing + "\n" + prefix)}
				}
				if err := catalog.SaveAgent(clone); err != nil {
					return polyglot.CallResult{}, err
				}
				clones[plan.ID] = name
			}
			if clone, ok := catalog.Agent(name); ok && clone.Instruction != nil {
				instruction = strings.TrimSpace(clone.Instruction.Content + "\n" + instruction)
			}
			result, err := invoke(ctx, chat.Options{Agent: name, Instruction: instruction, Cwd: cfg.CallerDir()}, nil)
			if err == nil && result.ExitCode != 0 {
				err = fmt.Errorf("agent.run: chat exited %d", result.ExitCode)
			}
			return polyglot.CallResult{Value: result.Output}, err
		},
		CloseFunc: func(plan polyglot.Plan) error {
			mu.Lock()
			defer mu.Unlock()
			name := clones[plan.ID]
			if name == "" {
				return nil
			}
			if err := catalog.RemoveAgent(name); err != nil {
				return fmt.Errorf("agent fence cleanup %s: %w", name, err)
			}
			delete(clones, plan.ID)
			return nil
		},
	}
}

// Binding names and the existing fleet agent YAML's name/instruction fields
// refer to a registered identity. Definitions requiring a different harness
// spec must not silently degrade to a plain prompt or an ungoverned execution.
func agentFenceBinding(catalog *fleet.Catalog, source string) (string, string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", "", errors.New("agent fence: empty definition")
	}
	if agent, ok := catalog.Agent(source); ok {
		return agent.Name, "", nil
	}
	// Fleet's catalog parser accepts forward-compatible fields. A runnable
	// fence must instead refuse typos or a different harness schema, not
	// silently drop policy supplied in the source artifact.
	var document map[string]any
	if err := yaml.Unmarshal([]byte(source), &document); err != nil {
		return "", "", fmt.Errorf("agent fence: invalid definition: %w", err)
	}
	definition := document
	if agents, ok := document["agents"].([]any); ok && len(agents) == 1 {
		definition, _ = agents[0].(map[string]any)
	}
	for key := range definition {
		switch key {
		case "name", "tool", "model", "instruction":
		default:
			return "", "", fmt.Errorf("agent fence: field %q requires full spec integration; this row currently accepts registered binding references", key)
		}
	}
	decoder := yaml.NewDecoder(strings.NewReader(source))
	decoder.KnownFields(true)
	var target any = &fleet.Agent{}
	if _, envelope := document["agents"]; envelope {
		target = &fleet.AgentFile{}
	}
	if err := decoder.Decode(target); err != nil {
		return "", "", fmt.Errorf("agent fence: invalid fleet binding definition: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return "", "", errors.New("agent fence: expected exactly one YAML document")
	}
	file, err := fleet.ParseAgentFile("", []byte(source), nil)
	if err != nil {
		return "", "", fmt.Errorf("agent fence: expected a registered binding or fleet agent definition: %w", err)
	}
	if len(file.Agents) != 1 {
		return "", "", errors.New("agent fence: definition must reference exactly one agent")
	}
	agent := file.Agents[0]
	parent, ok := catalog.Agent(agent.Name)
	if !ok {
		return "", "", fmt.Errorf("agent fence: binding %q is not registered; use bashy agent add", agent.Name)
	}
	if agent.Tool != "" && agent.Tool != parent.Tool || agent.Model != "" && agent.Model != parent.Model {
		return "", "", errors.New("agent fence: definition changes the binding; register that binding with bashy agent add first")
	}
	prefix := ""
	if agent.Instruction != nil {
		prefix = agent.Instruction.Content
	}
	return parent.Name, prefix, nil
}
