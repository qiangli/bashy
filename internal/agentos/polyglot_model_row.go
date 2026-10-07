package agentos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/qiangli/yoke/pkg/chat"
	"github.com/qiangli/yoke/pkg/fleet"
	"gopkg.in/yaml.v3"
	"mvdan.cc/sh/v3/polyglot"
)

// A model fence names a fleet model and uses one existing registered agent
// bound to it. Choosing between several identities would silently change
// authorization and spend policy, so that case is refused at preparation.
func init() { polyglot.RegisterLanguage(modelFenceRow()) }

func modelFenceRow() polyglot.Language {
	return polyglot.Language{Canonical: "model", Text: true, InterpretedOnly: true,
		NewRuntime: func(cfg polyglot.RuntimeConfig) polyglot.LanguageRuntime {
			return newModelFenceRuntime(cfg, fleet.New(), chat.Invoke)
		},
	}
}

func newModelFenceRuntime(cfg polyglot.RuntimeConfig, catalog *fleet.Catalog, invoke agentFenceInvoke) polyglot.Embedded {
	return polyglot.Embedded{RuntimeName: "model",
		AnalyzeFunc: func(ctx context.Context, source string) ([]polyglot.Export, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if _, err := modelFenceAgent(catalog, source); err != nil {
				return nil, err
			}
			return []polyglot.Export{{Name: "run", Agentic: true, Effects: []string{"exec", "net", "spend"},
				Signature: polyglot.Signature{Params: []string{"string"}, Results: []string{"string", "error"}}}}, nil
		},
		CallFunc: func(ctx context.Context, plan polyglot.Plan, method string, args []any, kwargs map[string]any) (polyglot.CallResult, error) {
			if method != "run" || len(args) != 1 || len(kwargs) != 0 {
				return polyglot.CallResult{}, errors.New("model.run requires one prompt string")
			}
			prompt, ok := args[0].(string)
			if !ok || strings.TrimSpace(prompt) == "" {
				return polyglot.CallResult{}, errors.New("model.run requires a nonempty prompt string")
			}
			agent, err := modelFenceAgent(catalog, plan.Source)
			if err != nil {
				return polyglot.CallResult{}, err
			}
			result, err := invoke(ctx, chat.Options{Agent: agent, Catalog: catalog, Instruction: prompt, Cwd: cfg.CallerDir()}, nil)
			if err == nil && result.ExitCode != 0 {
				err = fmt.Errorf("model.run: chat exited %d", result.ExitCode)
			}
			return polyglot.CallResult{Value: result.Output}, err
		},
	}
}

func modelFenceAgent(catalog *fleet.Catalog, source string) (string, error) {
	source = strings.TrimSpace(source)
	if model, ok := catalog.Model(source); ok {
		return modelFenceBoundAgent(catalog, model)
	}
	name, err := fleetFenceName(source, "model", &fleet.Model{})
	if err != nil {
		return "", err
	}
	model, ok := catalog.Model(name)
	if !ok {
		return "", fmt.Errorf("model fence: model %q is not registered", name)
	}
	if err := fleetFenceMatches(source, "model", model); err != nil {
		return "", err
	}
	return modelFenceBoundAgent(catalog, model)
}

func modelFenceBoundAgent(catalog *fleet.Catalog, model fleet.Model) (string, error) {
	agents, errs := catalog.Agents()
	if len(errs) > 0 {
		return "", fmt.Errorf("model fence: catalog agents: %w", errors.Join(errs...))
	}
	var matches []string
	for _, agent := range agents {
		_, _, bound, err := catalog.Binding(agent.Name)
		if err == nil && bound.Name == model.Name {
			matches = append(matches, agent.Name)
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("model fence: no registered agent binding for %q", model.Name)
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("model fence: ambiguous agent bindings for %q: %s", model.Name, strings.Join(matches, ", "))
	}
	return matches[0], nil
}

// A fence accepts the existing fleet YAML shape or one registered name.
// Supplied fields must agree with the registered definition. This prevents a
// YAML body from implying a provider, credential, or launch policy that the
// actual call would not use.
func fleetFenceName(source, kind string, target any) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", fmt.Errorf("%s fence: empty definition", kind)
	}
	if !strings.Contains(source, "\n") && !strings.HasPrefix(source, "name:") {
		return source, nil
	}
	decoder := yaml.NewDecoder(strings.NewReader(source))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return "", fmt.Errorf("%s fence: invalid fleet definition: %w", kind, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return "", fmt.Errorf("%s fence: expected exactly one YAML document", kind)
	}
	var fields struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal([]byte(source), &fields); err != nil {
		return "", err
	}
	if fields.Name == "" {
		return "", fmt.Errorf("%s fence: definition requires name", kind)
	}
	return fields.Name, nil
}

func fleetFenceMatches(source, kind string, registered any) error {
	if !strings.Contains(source, "\n") && !strings.HasPrefix(source, "name:") {
		return nil
	}
	var supplied map[string]any
	if err := yaml.Unmarshal([]byte(source), &supplied); err != nil {
		return err
	}
	return fleetFenceFieldsMatch(supplied, kind, registered)
}

func fleetFenceFieldsMatch(supplied map[string]any, kind string, registered any) error {
	var actual map[string]any
	data, err := yaml.Marshal(registered)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, &actual); err != nil {
		return err
	}
	for key, value := range supplied {
		if !reflect.DeepEqual(value, actual[key]) {
			return fmt.Errorf("%s fence: field %q differs from registered binding", kind, key)
		}
	}
	return nil
}
