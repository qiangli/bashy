package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/chat"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/llmbudget"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/lower"
	"mvdan.cc/sh/v3/polyglot"
	"mvdan.cc/sh/v3/syntax"
)

func modelFenceFixture(t *testing.T) *fleet.Catalog {
	t.Helper()
	cat := agentFenceCatalog(t)
	if err := cat.SaveTool(fleet.Tool{Name: "fixture", Kind: "cli", CLI: fleet.ToolCLI{Binary: "/bin/echo", Launch: fleet.ToolLaunch{Exec: "/bin/echo --model {model} {prompt}"}}}); err != nil {
		t.Fatal(err)
	}
	if err := cat.SaveModel(fleet.Model{Name: "fixture-model", Kind: "api", Provider: "fixture", CostMicro: 1000}); err != nil {
		t.Fatal(err)
	}
	return cat
}

func TestModelFenceInlineEmbedAndGuards(t *testing.T) {
	savedGate := interp.ForeignEffectGate
	interp.ForeignEffectGate = fenceEffectGate
	defer func() { interp.ForeignEffectGate = savedGate }()
	cat := modelFenceFixture(t)
	saved, _ := polyglot.LookupLanguage("model")
	defer polyglot.RegisterLanguage(saved)
	calls := 0
	row := modelFenceRow()
	row.NewRuntime = func(cfg polyglot.RuntimeConfig) polyglot.LanguageRuntime {
		return newModelFenceRuntime(cfg, cat, func(_ context.Context, opt chat.Options, _ chat.Runner) (chat.Result, error) {
			calls++
			if opt.Agent != "searcher" || opt.Catalog != cat || opt.AllowPremium || opt.AllowUnsafe || opt.DryRun {
				t.Fatalf("ungoverned options: %+v", opt)
			}
			return chat.Result{Output: "recorded model answer"}, nil
		})
	}
	polyglot.RegisterLanguage(row)
	dir := t.TempDir()
	definition := filepath.Join(dir, "model.yaml")
	if err := os.WriteFile(definition, []byte("name: fixture-model\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	rel, _ := filepath.Rel(cwd, definition)
	for _, declaration := range []string{"~~~model as oracle\nname: fixture-model\n~~~\n", "embed model \"./" + rel + "\" as oracle\n"} {
		for _, tc := range []struct {
			body, want string
			called     bool
		}{
			{"answer, err := oracle.run(\"question\")", "agentic action requires", false},
			{"@guard(effects: \"read\")\nagentic func query() { answer, err := oracle.run(\"question\"); }\nagentic { query(); }", "exceed the guard", false},
			{"@guard(effects: \"exec,net,spend\")\n@ensure('test \"$RESULT\" = \"recorded model answer\"')\nagentic func query() string { answer, err := oracle.run(\"question\"); return answer; }\nagentic { answer := query(); echo \"$answer\"; }", "recorded model answer", true},
		} {
			before := calls
			err, out, diag := runDecorated(t, context.Background(), syntax.LangBashPP, declaration+tc.body+"\n", map[string]string{"BASHY_AUDIT": "0"})
			if (calls > before) != tc.called || !strings.Contains(out.String()+diag.String(), tc.want) {
				t.Fatalf("declaration=%q called=%v err=%v out=%q diag=%q", declaration, calls > before, err, out, diag)
			}
		}
	}
}

func TestModelFenceBindingErrorsAndLowering(t *testing.T) {
	cat := modelFenceFixture(t)
	for _, source := range []string{"", "missing", "name: missing", "name: fixture-model\nprovider: other\n", "name: fixture-model\ninvalid_field: true\n", "name: fixture-model\n---\nname: fixture-model"} {
		if _, err := modelFenceAgent(cat, source); err == nil {
			t.Fatalf("accepted %q", source)
		}
	}
	if err := cat.SaveAgent(fleet.Agent{Name: "second", Tool: "fixture", Model: "fixture-model"}); err != nil {
		t.Fatal(err)
	}
	if _, err := modelFenceAgent(cat, "fixture-model"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguity: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "model.yaml"), []byte("fixture-model"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{"~~~model as oracle\nfixture-model\n~~~\n", "embed model \"./model.yaml\" as oracle\n"} {
		file, err := syntax.NewParser(syntax.Variant(syntax.LangBashPP)).Parse(strings.NewReader(src), filepath.Join(dir, "script.bsh"))
		if err != nil {
			t.Fatal(err)
		}
		_, err = lower.Compile(file, lower.Options{})
		if err == nil || !strings.Contains(err.Error(), "run it interpreted") {
			t.Fatalf("lowering=%v", err)
		}
	}
}

func TestModelFenceHarnessMetersSpendAndBlocksBudget(t *testing.T) {
	cat := modelFenceFixture(t)
	t.Setenv("BASHY_ROOM_DIR", t.TempDir())
	t.Setenv("BASHY_CHAT_DIR", t.TempDir())
	t.Setenv("BASHY_KNOWLEDGE", "off")
	t.Setenv("BASHY_OUTPUT_REDUCE", "off")
	path := filepath.Join(t.TempDir(), "meter.json")
	gate := llmbudget.New(llmbudget.Config{StatePath: path, Models: map[string]llmbudget.Model{"fixture-model": {Name: "fixture-model", Kind: "api", Provider: "fixture", CostMicro: 1000}}})
	defer llmbudget.SetDefault(gate)()
	runner := &agentFenceFixtureRunner{}
	runtime := newModelFenceRuntime(polyglot.RuntimeConfig{Dir: t.TempDir()}, cat, func(ctx context.Context, opt chat.Options, _ chat.Runner) (chat.Result, error) {
		return chat.Invoke(ctx, opt, runner)
	})
	exports, err := runtime.Analyze(context.Background(), "fixture-model")
	if err != nil {
		t.Fatal(err)
	}
	if !exports[0].Agentic || strings.Join(exports[0].Effects, ",") != "exec,net,spend" {
		t.Fatalf("contract=%+v", exports)
	}
	mod := polyglot.Start(polyglot.Plan{ID: "metered", Source: "fixture-model", Exports: exports}, runtime)
	defer mod.Close()
	result, err := mod.Call(context.Background(), "run", "question")
	if err != nil || result.Value != "recorded answer" || runner.calls != 1 {
		t.Fatalf("result=%+v err=%v calls=%d", result, err, runner.calls)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state llmbudget.State
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if state.Models["fixture-model"].DayTokens <= 0 || state.Models["fixture-model"].DayCostUSD <= 0 {
		t.Fatalf("unmetered: %+v", state.Models["fixture-model"])
	}
	limit := int64(1)
	strict := llmbudget.New(llmbudget.Config{StatePath: filepath.Join(t.TempDir(), "strict.json"), Policy: &llmbudget.Policy{Version: 1,
		Bindings:    []llmbudget.Binding{{Model: "fixture-model", Provider: "fixture", Account: "fixture", Pool: "fixture", Lane: llmbudget.LaneAPIKey}},
		Constraints: []llmbudget.Constraint{{Provider: "fixture", Account: "fixture", Pool: "fixture", DailySpendMicroUSD: &limit}},
	}})
	defer llmbudget.SetDefault(strict)()
	_, err = mod.Call(context.Background(), "run", "question")
	if err == nil || !strings.Contains(err.Error(), "budget") || runner.calls != 1 {
		t.Fatalf("hard cap bypass: %v calls=%d", err, runner.calls)
	}
}

func TestModelFencePreservesStatusAndErrorOutput(t *testing.T) {
	cat := modelFenceFixture(t)
	for _, fixture := range []struct {
		result chat.Result
		err    error
	}{
		{chat.Result{Output: "partial", ExitCode: 7}, nil},
		{chat.Result{Output: "partial"}, errors.New("transport failed")},
	} {
		runtime := newModelFenceRuntime(polyglot.RuntimeConfig{Dir: t.TempDir()}, cat, func(context.Context, chat.Options, chat.Runner) (chat.Result, error) {
			return fixture.result, fixture.err
		})
		exports, err := runtime.Analyze(context.Background(), "fixture-model")
		if err != nil {
			t.Fatal(err)
		}
		module := polyglot.Start(polyglot.Plan{ID: "status", Source: "fixture-model", Exports: exports}, runtime)
		result, err := module.Call(context.Background(), "run", "question")
		if err == nil || result.Value != "partial" {
			t.Fatalf("result=%+v err=%v", result, err)
		}
		module.Close()
	}
}
