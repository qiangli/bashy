package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/llmbudget"
	"github.com/qiangli/yoke/pkg/toolcmd"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/lower"
	"mvdan.cc/sh/v3/polyglot"
	"mvdan.cc/sh/v3/syntax"
)

func toolFenceFixture(t *testing.T) *fleet.Catalog {
	t.Helper()
	cat := agentFenceCatalog(t)
	tool := fleet.Tool{Name: "fixture", Kind: "cli", CLI: fleet.ToolCLI{Binary: "/bin/echo", Launch: fleet.ToolLaunch{Exec: "/bin/echo --model {model} {prompt}"}},
		Commands: []fleet.ToolCommand{{Name: "review", Slash: "review {args}", Mode: fleet.ToolCommandPrint, Effects: []string{"read"}}}}
	if err := cat.SaveTool(tool); err != nil {
		t.Fatal(err)
	}
	if err := cat.SaveModel(fleet.Model{Name: "fixture-model", Kind: "api", Provider: "fixture", CostMicro: 1000}); err != nil {
		t.Fatal(err)
	}
	return cat
}

func TestToolFenceInlineEmbedMethodsAndGuards(t *testing.T) {
	savedGate := interp.ForeignEffectGate
	interp.ForeignEffectGate = fenceEffectGate
	defer func() { interp.ForeignEffectGate = savedGate }()
	cat := toolFenceFixture(t)
	saved, _ := polyglot.LookupLanguage("tool")
	defer polyglot.RegisterLanguage(saved)
	calls := 0
	row := toolFenceRow()
	row.NewRuntime = func(cfg polyglot.RuntimeConfig) polyglot.LanguageRuntime {
		return newToolFenceRuntime(cfg, cat, func(_ context.Context, tool fleet.Tool, command fleet.ToolCommand, args string, opt toolcmd.Options) (toolcmd.Result, error) {
			calls++
			if tool.Name != "fixture" || command.Name != "review" || args != "change" || opt.Catalog != cat || opt.Agent != "searcher" || opt.DryRun {
				t.Fatalf("runner parameters: %q %+v %+v", args, command, opt)
			}
			return toolcmd.Result{Outcome: toolcmd.OutcomeSuccess, Text: "recorded review"}, nil
		})
	}
	polyglot.RegisterLanguage(row)
	dir := newFenceEmbedFixtureDir(t)
	definition := filepath.Join(dir, "tool.yaml")
	if err := os.WriteFile(definition, []byte("name: fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	rel := fenceEmbedRelPath(t, definition)
	for _, declaration := range []string{"~~~tool as kit\nname: fixture\n~~~\n", "embed tool \"./" + rel + "\" as kit\n"} {
		for _, tc := range []struct {
			body, want string
			called     bool
		}{
			{"answer, err := kit.review(\"change\")", "agentic action requires", false},
			{"@guard(effects: \"read\")\nagentic func query() { answer, err := kit.review(\"change\"); }\nagentic { query(); }", "exceed the guard", false},
			{"@guard(effects: \"read,exec,net,spend\")\nagentic func query() string { answer, err := kit.review(\"change\"); return answer; }\nagentic { answer := query(); echo \"$answer\"; }", "recorded review", true},
		} {
			before := calls
			err, out, diag := runDecorated(t, context.Background(), syntax.LangBashPP, declaration+tc.body+"\n", map[string]string{"BASHY_AUDIT": "0"})
			if (calls > before) != tc.called || !strings.Contains(out.String()+diag.String(), tc.want) {
				t.Fatalf("declaration=%q called=%v err=%v out=%q diag=%q", declaration, calls > before, err, out, diag)
			}
		}
	}
}

func TestToolFenceStatusErrorsInvalidAndLowering(t *testing.T) {
	cat := toolFenceFixture(t)
	for _, source := range []string{"", "missing", "name: missing", "name: fixture\nkind: web\n", "name: fixture\nunknown_policy: true\n"} {
		if _, err := toolFenceBinding(cat, source); err == nil {
			t.Fatalf("accepted %q", source)
		}
	}
	if _, err := toolFenceBinding(cat, "kit: fixture\ntype: cli\n"); err != nil {
		t.Fatalf("legacy fleet definition rejected: %v", err)
	}
	for _, failure := range []struct {
		result toolcmd.Result
		err    error
	}{
		{toolcmd.Result{Outcome: toolcmd.OutcomeError, ExitCode: 7, Error: "bad"}, nil},
		{toolcmd.Result{Outcome: toolcmd.OutcomeError, Text: "partial"}, errors.New("runner failed")},
	} {
		runtime := newToolFenceRuntime(polyglot.RuntimeConfig{Dir: t.TempDir()}, cat, func(context.Context, fleet.Tool, fleet.ToolCommand, string, toolcmd.Options) (toolcmd.Result, error) {
			return failure.result, failure.err
		})
		exports, err := runtime.Analyze(context.Background(), "fixture")
		if err != nil {
			t.Fatal(err)
		}
		if len(exports) != 1 || !exports[0].Agentic || strings.Join(exports[0].Effects, ",") != "read,exec,net,spend" {
			t.Fatalf("contract=%+v", exports)
		}
		mod := polyglot.Start(polyglot.Plan{ID: "status", Source: "fixture", Exports: exports}, runtime)
		result, err := mod.Call(context.Background(), "review", "change")
		if err == nil || result.Value != failure.result.Text {
			t.Fatalf("result=%+v err=%v", result, err)
		}
		mod.Close()
	}
	tool, _ := cat.Tool("fixture")
	tool.Commands[0].Effects = nil
	if err := cat.SaveTool(tool); err != nil {
		t.Fatal(err)
	}
	if _, err := newToolFenceRuntime(polyglot.RuntimeConfig{}, cat, toolcmd.Run).Analyze(context.Background(), "fixture"); err == nil || !strings.Contains(err.Error(), "declare effects") {
		t.Fatalf("undeclared effects: %v", err)
	}
	tool.Commands[0].Effects = []string{"read", "read"}
	if err := cat.SaveTool(tool); err != nil {
		t.Fatal(err)
	}
	if _, err := newToolFenceRuntime(polyglot.RuntimeConfig{}, cat, toolcmd.Run).Analyze(context.Background(), "fixture"); err == nil || !strings.Contains(err.Error(), "duplicate effect") {
		t.Fatalf("duplicate effects: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tool.yaml"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{"~~~tool as kit\nfixture\n~~~\n", "embed tool \"./tool.yaml\" as kit\n"} {
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

func TestToolFenceRejectsCatalogDriftAfterAnalysis(t *testing.T) {
	for _, change := range []struct {
		name string
		edit func(*fleet.Tool)
	}{
		{"effects", func(tool *fleet.Tool) { tool.Commands[0].Effects = []string{"read", "write"} }},
		{"slash", func(tool *fleet.Tool) { tool.Commands[0].Slash = "write {args}" }},
	} {
		t.Run(change.name, func(t *testing.T) {
			cat := toolFenceFixture(t)
			calls := 0
			runtime := newToolFenceRuntime(polyglot.RuntimeConfig{}, cat, func(context.Context, fleet.Tool, fleet.ToolCommand, string, toolcmd.Options) (toolcmd.Result, error) {
				calls++
				return toolcmd.Result{Outcome: toolcmd.OutcomeSuccess}, nil
			})
			exports, err := runtime.Analyze(context.Background(), "fixture")
			if err != nil {
				t.Fatal(err)
			}
			unadmitted := polyglot.Start(polyglot.Plan{ID: "unadmitted", Source: "fixture"}, runtime)
			if _, err := unadmitted.Call(context.Background(), "review", "change"); err == nil || calls != 0 {
				t.Fatalf("missing export dispatched: err=%v calls=%d", err, calls)
			}
			unadmitted.Close()
			tool, _ := cat.Tool("fixture")
			change.edit(&tool)
			if err := cat.SaveTool(tool); err != nil {
				t.Fatal(err)
			}
			mod := polyglot.Start(polyglot.Plan{ID: "drift", Source: "fixture", Exports: exports}, runtime)
			defer mod.Close()
			if _, err := mod.Call(context.Background(), "review", "change"); err == nil || !strings.Contains(err.Error(), "changed since analysis") || calls != 0 {
				t.Fatalf("drift dispatched: err=%v calls=%d", err, calls)
			}
		})
	}
}

func TestToolFenceRegisteredNestedDefinitionDefaultsAndAliases(t *testing.T) {
	cat := toolFenceFixture(t)
	definition := "kit: nested\ntype: cli\ncli:\n  launch:\n    exec: /bin/echo {prompt}\ncommands:\n  - name: review\n    slash: review {args}\n    mode: print\n    effects: [read]\n"
	tool, err := fleet.ParseTool("", []byte(definition), nil)
	if err != nil {
		t.Fatal(err)
	}
	if tool.CLI.Binary != "nested" {
		t.Fatalf("missing binary default: %+v", tool.CLI)
	}
	if err := cat.SaveTool(tool); err != nil {
		t.Fatal(err)
	}
	if _, err := toolFenceBinding(cat, definition); err != nil {
		t.Fatalf("registered definition rejected: %v", err)
	}
	if _, err := toolFenceBinding(cat, strings.Replace(definition, "review {args}", "write {args}", 1)); err == nil {
		t.Fatal("changed nested command accepted")
	}
}

func TestToolFenceRunnerMetersAndHonorsHardSpendCap(t *testing.T) {
	cat := toolFenceFixture(t)
	t.Setenv("BASHY_ROOM_DIR", t.TempDir())
	t.Setenv("BASHY_CHAT_DIR", t.TempDir())
	t.Setenv("BASHY_KNOWLEDGE", "off")
	t.Setenv("BASHY_OUTPUT_REDUCE", "off")
	path := filepath.Join(t.TempDir(), "meter.json")
	gate := llmbudget.New(llmbudget.Config{StatePath: path, Models: map[string]llmbudget.Model{"fixture-model": {Name: "fixture-model", Kind: "api", Provider: "fixture", CostMicro: 1000}}})
	defer llmbudget.SetDefault(gate)()
	runtime := newToolFenceRuntime(polyglot.RuntimeConfig{Dir: t.TempDir()}, cat, toolcmd.Run)
	exports, err := runtime.Analyze(context.Background(), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	mod := polyglot.Start(polyglot.Plan{ID: "metered-tool", Source: "fixture", Exports: exports}, runtime)
	defer mod.Close()
	result, err := mod.Call(context.Background(), "review", "change")
	if err != nil || !strings.Contains(fmt.Sprint(result.Value), "review change") {
		t.Fatalf("result=%+v err=%v", result, err)
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
	_, err = mod.Call(context.Background(), "review", "change")
	if err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("hard cap bypass: %v", err)
	}
}
