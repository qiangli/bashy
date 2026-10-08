package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/qiangli/yoke/pkg/llmbudget"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/chat"
	"github.com/qiangli/yoke/pkg/fleet"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/lower"
	"mvdan.cc/sh/v3/polyglot"
	"mvdan.cc/sh/v3/syntax"
)

func agentFenceCatalog(t *testing.T) *fleet.Catalog {
	t.Helper()
	t.Setenv("BASHY_FLEET_DIR", t.TempDir())
	for _, noun := range []string{"AGENTS", "MODELS", "TOOLS"} {
		t.Setenv("BASHY_"+noun+"_DIR", "")
		t.Setenv("BASHY_"+noun+"_PATH", "")
	}
	t.Setenv("BASHY_FLEET_SEEDS", "off")
	cat := fleet.New(fleet.WithoutCloudOverlay())
	if err := cat.SaveAgent(fleet.Agent{Name: "searcher", Tool: "fixture", Model: "fixture-model"}); err != nil {
		t.Fatal(err)
	}
	return cat
}

func TestAgentFenceLifecycle(t *testing.T) {
	for _, outcome := range []string{"success", "failure", "status", "cancel"} {
		t.Run(outcome, func(t *testing.T) {
			cat := agentFenceCatalog(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var names []string
			runtime := newAgentFenceRuntime(polyglot.RuntimeConfig{Dir: t.TempDir()}, cat, func(ctx context.Context, opt chat.Options, _ chat.Runner) (chat.Result, error) {
				names = append(names, opt.Agent)
				if opt.AllowUnsafe || opt.AllowPremium || opt.DryRun || opt.ExecArgv != nil {
					t.Fatal("row bypasses governed defaults")
				}
				clone, ok := cat.Agent(opt.Agent)
				if !ok || !clone.Ephemeral || clone.ClonedFrom != "searcher" || opt.Agent == "searcher" {
					t.Fatalf("not a clone: %+v", clone)
				}
				if opt.Instruction != "find evidence" {
					t.Fatalf("instruction=%q", opt.Instruction)
				}
				switch outcome {
				case "failure":
					return chat.Result{Output: "raw failure"}, errors.New("fixture failed")
				case "status":
					return chat.Result{Output: "raw failure", ExitCode: 7}, nil
				case "cancel":
					cancel()
					return chat.Result{}, ctx.Err()
				}
				return chat.Result{Output: "recorded answer"}, nil
			})
			exports, err := runtime.Analyze(ctx, "searcher")
			if err != nil {
				t.Fatal(err)
			}
			if !exports[0].Agentic || strings.Join(exports[0].Effects, ",") != "exec,net,spend" {
				t.Fatalf("contract=%+v", exports)
			}
			mod := polyglot.Start(polyglot.Plan{ID: "script", Source: "searcher", Exports: exports}, runtime)
			for i := 0; i < 2; i++ {
				result, err := mod.Call(ctx, "run", "find evidence")
				if outcome == "success" {
					if err != nil || result.Value != "recorded answer" {
						t.Fatalf("result=%+v err=%v", result, err)
					}
				} else if err == nil {
					t.Fatal("failure lost")
				}
			}
			if len(names) > 1 && names[0] != names[1] {
				t.Fatal("clone was not reused")
			}
			if err := mod.Close(); err != nil {
				t.Fatal(err)
			}
			if err := mod.Close(); err != nil {
				t.Fatal("close is not idempotent", err)
			}
			for _, name := range names {
				if _, ok := cat.Agent(name); ok {
					t.Fatalf("leaked clone %s", name)
				}
			}
			if _, ok := cat.Agent("searcher"); !ok {
				t.Fatal("parent removed")
			}
		})
	}
}

func TestAgentFenceInlineEmbedGovernance(t *testing.T) {
	savedGate := interp.ForeignEffectGate
	interp.ForeignEffectGate = fenceEffectGate
	defer func() { interp.ForeignEffectGate = savedGate }()
	cat := agentFenceCatalog(t)
	saved, _ := polyglot.LookupLanguage("agent")
	defer polyglot.RegisterLanguage(saved)
	calls := 0
	row := agentFenceRow()
	row.NewRuntime = func(cfg polyglot.RuntimeConfig) polyglot.LanguageRuntime {
		return newAgentFenceRuntime(cfg, cat, func(_ context.Context, opt chat.Options, _ chat.Runner) (chat.Result, error) {
			calls++
			return chat.Result{Output: "recorded answer"}, nil
		})
	}
	polyglot.RegisterLanguage(row)
	dir := newFenceEmbedFixtureDir(t)
	def := filepath.Join(dir, "binding.yaml")
	if err := os.WriteFile(def, []byte("searcher"), 0600); err != nil {
		t.Fatal(err)
	}
	// The parser resolves embed paths relative to its source file.
	rel := fenceEmbedRelPath(t, def)
	for _, declaration := range []string{"~~~agent as searcher\nsearcher\n~~~\n", "embed agent \"./" + rel + "\" as searcher\n"} {
		for _, tc := range []struct {
			name, body, want string
			called           bool
		}{
			{"plain denied", "answer, err := searcher.run(\"find evidence\")", "agentic action requires", false},
			{"read denied", "@guard(effects: \"read\")\nagentic func query() { answer, err := searcher.run(\"find evidence\"); echo \"$answer\"; }\nagentic { query(); }", "exceed the guard", false},
			{"ensured", "@guard(effects: \"exec,net,spend\")\n@ensure('test \"$RESULT\" = \"recorded answer\"')\nagentic func query() string { answer, err := searcher.run(\"find evidence\"); return answer; }\nagentic { answer := query(); echo \"$answer\"; }", "recorded answer", true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				before := calls
				runErr, out, diag := runDecorated(t, context.Background(), syntax.LangBashPP, declaration+tc.body+"\n", map[string]string{"BASHY_AUDIT": "0"})
				if (calls > before) != tc.called {
					t.Fatalf("calls=%d out=%q diag=%q err=%v", calls-before, out, diag, runErr)
				}
				if !strings.Contains(out.String()+diag.String(), tc.want) {
					t.Fatalf("want %q: out=%q diag=%q err=%v", tc.want, out, diag, runErr)
				}
				entries, _ := cat.Agents()
				for _, a := range entries {
					if a.ClonedFrom == "searcher" {
						t.Fatalf("leaked clone %s", a.Name)
					}
				}
			})
		}
	}
}

func TestAgentFenceInvalid(t *testing.T) {
	cat := agentFenceCatalog(t)
	for _, source := range []string{"", "missing", "name: missing", "agents: []", "name: searcher\ntool: different", "name: searcher\nunknown_policy: true", "name: searcher\n---\nname: searcher"} {
		if _, _, err := agentFenceBinding(cat, source); err == nil {
			t.Fatalf("accepted %q", source)
		}
	}
}

// Only the external model process is replaced. Catalog resolution, launch
// governance, budget reservation/settlement, and clone lifecycle remain real.
type agentFenceFixtureRunner struct{ calls int }

func (r *agentFenceFixtureRunner) Run(ctx context.Context, _ string, _ []string, _ string) (string, int, error) {
	r.calls++
	return "recorded answer", 0, ctx.Err()
}

func TestAgentFenceHarnessMetersSpend(t *testing.T) {
	cat := agentFenceCatalog(t)
	t.Setenv("BASHY_ROOM_DIR", t.TempDir())
	t.Setenv("BASHY_CHAT_DIR", t.TempDir())
	t.Setenv("BASHY_KNOWLEDGE", "off")
	t.Setenv("BASHY_OUTPUT_REDUCE", "off")
	if err := cat.SaveTool(fleet.Tool{Name: "fixture", Kind: "cli", CLI: fleet.ToolCLI{Binary: "/bin/echo", Launch: fleet.ToolLaunch{Exec: "/bin/echo --model {model} {prompt}"}}}); err != nil {
		t.Fatal(err)
	}
	if err := cat.SaveModel(fleet.Model{Name: "fixture-model", Kind: "api", Provider: "fixture", CostMicro: 1000}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "meter.json")
	gate := llmbudget.New(llmbudget.Config{StatePath: path, Models: map[string]llmbudget.Model{"fixture-model": {Name: "fixture-model", Kind: "api", Provider: "fixture", CostMicro: 1000}}})
	defer llmbudget.SetDefault(gate)()
	runner := &agentFenceFixtureRunner{}
	runtime := newAgentFenceRuntime(polyglot.RuntimeConfig{Dir: t.TempDir()}, cat, func(ctx context.Context, opt chat.Options, _ chat.Runner) (chat.Result, error) {
		return chat.Invoke(ctx, opt, runner)
	})
	exports, err := runtime.Analyze(context.Background(), "searcher")
	if err != nil {
		t.Fatal(err)
	}
	mod := polyglot.Start(polyglot.Plan{ID: "metered", Source: "searcher", Exports: exports}, runtime)
	defer mod.Close()
	result, err := mod.Call(context.Background(), "run", "find evidence")
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
	meter := state.Models["fixture-model"]
	if meter.DayTokens <= 0 || meter.DayCostUSD <= 0 {
		t.Fatalf("not metered: %+v", meter)
	}

	// A hard spend policy refuses this opaque harness turn before transport:
	// no invented estimate is allowed to satisfy a hard pre-call ceiling.
	limit := int64(1)
	strict := llmbudget.New(llmbudget.Config{StatePath: filepath.Join(t.TempDir(), "meter.json"), Policy: &llmbudget.Policy{Version: 1,
		Bindings:    []llmbudget.Binding{{Model: "fixture-model", Provider: "fixture", Account: "fixture", Pool: "fixture", Lane: llmbudget.LaneAPIKey}},
		Constraints: []llmbudget.Constraint{{Provider: "fixture", Account: "fixture", Pool: "fixture", DailySpendMicroUSD: &limit}},
	}})
	restore := llmbudget.SetDefault(strict)
	defer restore()
	_, err = mod.Call(context.Background(), "run", "find evidence")
	if err == nil || !strings.Contains(err.Error(), "budget") || runner.calls != 1 {
		t.Fatalf("hard spend bypass: err=%v calls=%d", err, runner.calls)
	}
}

func TestAgentFenceBindingInstruction(t *testing.T) {
	cat := agentFenceCatalog(t)
	name, prefix, err := agentFenceBinding(cat, "name: searcher\ninstruction:\n  content: be precise\n")
	if err != nil || name != "searcher" || prefix != "be precise" {
		t.Fatalf("name=%q prefix=%q err=%v", name, prefix, err)
	}
	parent, _ := cat.Agent(name)
	if parent.Instruction != nil {
		t.Fatal("definition mutated parent")
	}
}

func TestAgentFenceInterpreterCancellationCleanup(t *testing.T) {
	cat := agentFenceCatalog(t)
	saved, _ := polyglot.LookupLanguage("agent")
	defer polyglot.RegisterLanguage(saved)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var clone string
	row := agentFenceRow()
	row.NewRuntime = func(cfg polyglot.RuntimeConfig) polyglot.LanguageRuntime {
		return newAgentFenceRuntime(cfg, cat, func(ctx context.Context, opt chat.Options, _ chat.Runner) (chat.Result, error) {
			clone = opt.Agent
			cancel()
			return chat.Result{}, ctx.Err()
		})
	}
	polyglot.RegisterLanguage(row)
	err, _, _ := runDecorated(t, ctx, syntax.LangBashPP, "~~~agent as searcher\nsearcher\n~~~\nagentic { answer, err := searcher.run(\"find evidence\"); }\n", map[string]string{"BASHY_AUDIT": "0"})
	if clone == "" || err == nil {
		t.Fatalf("did not cancel a started call: clone=%q err=%v", clone, err)
	}
	if _, ok := cat.Agent(clone); ok {
		t.Fatalf("interpreter leaked clone %q", clone)
	}
}

func TestAgentFenceLoweringAndMissingEmbed(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "script.bsh")
	definition := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(definition, []byte("searcher"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{"~~~agent as searcher\nsearcher\n~~~\n", "embed agent \"./agent.yaml\" as searcher\n"} {
		file, err := syntax.NewParser(syntax.Variant(syntax.LangBashPP)).Parse(strings.NewReader(src), source)
		if err != nil {
			t.Fatal(err)
		}
		_, err = lower.Compile(file, lower.Options{})
		if err == nil || !strings.Contains(err.Error(), "~~~agent") || !strings.Contains(err.Error(), "run it interpreted") {
			t.Fatalf("missing lowering route: %v", err)
		}
	}
	_, err := syntax.NewParser(syntax.Variant(syntax.LangBashPP)).Parse(strings.NewReader("embed agent \"./missing.yaml\" as searcher\n"), source)
	if err == nil || !strings.Contains(err.Error(), "embed:") || !strings.Contains(err.Error(), "missing.yaml") {
		t.Fatalf("missing definition: %v", err)
	}
}

type yamlFenceFixture struct {
	calls, closes int
	failure       bool
}

func TestAgentFenceRejectsBindingDrift(t *testing.T) {
	cat := agentFenceCatalog(t)
	runtime := newAgentFenceRuntime(polyglot.RuntimeConfig{Dir: t.TempDir()}, cat, func(context.Context, chat.Options, chat.Runner) (chat.Result, error) {
		t.Fatal("drift reached transport")
		return chat.Result{}, nil
	})
	exports, err := runtime.Analyze(context.Background(), "searcher")
	if err != nil {
		t.Fatal(err)
	}
	parent, _ := cat.Agent("searcher")
	parent.Model = "changed"
	if err := cat.SaveAgent(parent); err != nil {
		t.Fatal(err)
	}
	mod := polyglot.Start(polyglot.Plan{ID: "drift", Source: "searcher", Exports: exports}, runtime)
	defer mod.Close()
	if _, err := mod.Call(context.Background(), "run", "request"); err == nil || !strings.Contains(err.Error(), "changed after preparation") {
		t.Fatalf("drift err=%v", err)
	}
}

func (f *yamlFenceFixture) Run(context.Context, string) (string, error) {
	f.calls++
	if f.failure {
		return "partial", errors.New("fixture failure")
	}
	return "recorded answer", nil
}
func (f *yamlFenceFixture) Close() error { f.closes++; return nil }

func TestAgentFenceYAMLInlineEmbedLifecycle(t *testing.T) {
	oldPrepare := YAMLAgentPrepare
	defer func() { YAMLAgentPrepare = oldPrepare }()
	cat := agentFenceCatalog(t)
	oldRow, _ := polyglot.LookupLanguage("agent")
	defer polyglot.RegisterLanguage(oldRow)
	oldGate := interp.ForeignEffectGate
	interp.ForeignEffectGate = fenceEffectGate
	defer func() { interp.ForeignEffectGate = oldGate }()
	dir := newFenceEmbedFixtureDir(t)
	path := filepath.Join(dir, "agent.yaml")
	const source = "apiVersion: ycode.dev/v1alpha1\nkind: Harness\nspec: {}\n"
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	rel := fenceEmbedRelPath(t, path)
	for _, embed := range []bool{false, true} {
		for _, failure := range []bool{false, true} {
			fixture := &yamlFenceFixture{failure: failure}
			opened := 0
			YAMLAgentPrepare = func(location string, raw []byte) (func() (YAMLAgentSession, error), error) {
				if string(raw) != source {
					t.Fatalf("definition changed: %q", raw)
				}
				if embed && location != path {
					t.Fatalf("embed origin=%q want %q", location, path)
				}
				return func() (YAMLAgentSession, error) { opened++; return fixture, nil }, nil
			}
			row := agentFenceRow()
			row.NewRuntime = func(cfg polyglot.RuntimeConfig) polyglot.LanguageRuntime { return newAgentFenceRuntime(cfg, cat, nil) }
			polyglot.RegisterLanguage(row)
			decl := "~~~agent as oracle\n" + source + "~~~\n"
			if embed {
				decl = "embed agent \"./" + rel + "\" as oracle\n"
			}
			body := "@ensure('test \"$RESULT\" = \"recorded answer\"')\nagentic func query() string { a, err := oracle.run(\"request\"); return a; }\nagentic { a := query(); echo \"$a\"; }\n"
			_, out, diag := runDecorated(t, context.Background(), syntax.LangBashPP, decl+body, map[string]string{"BASHY_AUDIT": "0"})
			if opened != 1 || fixture.calls != 1 || fixture.closes != 1 {
				t.Fatalf("opened=%d fixture=%+v output=%q %q", opened, fixture, out, diag)
			}
			if !failure && !strings.Contains(out.String(), "recorded answer") {
				t.Fatalf("output=%q %q", out, diag)
			}
		}
	}
}

func TestAgentFenceTwoEmbedsKeepTheirOwnOrigins(t *testing.T) {
	oldPrepare := YAMLAgentPrepare
	defer func() { YAMLAgentPrepare = oldPrepare }()
	oldRow, _ := polyglot.LookupLanguage("agent")
	defer polyglot.RegisterLanguage(oldRow)
	oldGate := interp.ForeignEffectGate
	interp.ForeignEffectGate = fenceEffectGate
	defer func() { interp.ForeignEffectGate = oldGate }()
	cat := agentFenceCatalog(t)
	row := agentFenceRow()
	row.NewRuntime = func(cfg polyglot.RuntimeConfig) polyglot.LanguageRuntime { return newAgentFenceRuntime(cfg, cat, nil) }
	polyglot.RegisterLanguage(row)
	const source = "apiVersion: ycode.dev/v1alpha1\nkind: Harness\nspec: {}\n"
	root := newFenceEmbedFixtureDir(t)
	var paths []string
	for _, name := range []string{"east", "west"} {
		path := filepath.Join(root, name, "agent.yaml")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	seen := map[string]int{}
	YAMLAgentPrepare = func(path string, raw []byte) (func() (YAMLAgentSession, error), error) {
		if string(raw) != source {
			t.Fatalf("source=%q", raw)
		}
		seen[path]++
		return func() (YAMLAgentSession, error) { return &yamlFenceFixture{}, nil }, nil
	}
	east := fenceEmbedRelPath(t, paths[0])
	west := fenceEmbedRelPath(t, paths[1])
	decl := "embed agent \"./" + east + "\" as east\nembed agent \"./" + west + "\" as west\n"
	_, out, diag := runDecorated(t, context.Background(), syntax.LangBashPP,
		decl+"agentic { a, ea := east.run(\"one\"); b, eb := west.run(\"two\"); echo \"$a $b\"; }\n",
		map[string]string{"BASHY_AUDIT": "0"})
	if seen[paths[0]] != 1 || seen[paths[1]] != 1 || !strings.Contains(out.String(), "recorded answer recorded answer") {
		t.Fatalf("origins=%v stdout=%q stderr=%q", seen, out, diag)
	}
}
