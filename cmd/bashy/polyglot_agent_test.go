//go:build !bashy_core && !bashy_cert_base

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/bashy/internal/agentos"
	"github.com/qiangli/ycode/pkg/ycode"
	"github.com/qiangli/ycode/pkg/ycodecli"
	"github.com/qiangli/yoke/pkg/llmbudget"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

type yamlAgentFixtureProvider struct{ calls int }

func (*yamlAgentFixtureProvider) Kind() ycode.ProviderKind { return ycode.ProviderOpenAI }
func (p *yamlAgentFixtureProvider) Send(context.Context, *ycode.ProviderRequest) (<-chan *ycode.ProviderStreamEvent, <-chan error) {
	p.calls++
	events := make(chan *ycode.ProviderStreamEvent, 3)
	delta, _ := json.Marshal(map[string]string{"type": "text_delta", "text": "recorded answer"})
	events <- &ycode.ProviderStreamEvent{Type: "content_block_delta", Delta: delta}
	events <- &ycode.ProviderStreamEvent{Type: "message_delta", Delta: json.RawMessage(`{"stop_reason":"end_turn"}`)}
	events <- &ycode.ProviderStreamEvent{Type: "message_stop"}
	close(events)
	errs := make(chan error)
	close(errs)
	return events, errs
}

func TestYAMLAgentFenceCompilerRunner(t *testing.T) {
	// Install the same effect gate as the executable's front door before
	// constructing an interpreter directly in this integration test.
	args := os.Args
	os.Args = []string{"bashy"}
	agentos.Dispatch()
	os.Args = args
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BASHY_HOME", t.TempDir())
	t.Setenv("BASHY_AUDIT", "0")
	t.Setenv("BASHY_OUTPUT_REDUCE", "off")
	defer llmbudget.SetDefault(llmbudget.New(llmbudget.Config{StatePath: filepath.Join(t.TempDir(), "meter.json")}))()
	saved := agentos.YAMLAgentPrepare
	defer func() { agentos.YAMLAgentPrepare = saved }()
	provider := &yamlAgentFixtureProvider{}
	agentos.YAMLAgentPrepare = func(source string, data []byte) (func() (agentos.YAMLAgentSession, error), error) {
		open, err := ycodecli.PrepareTextAgent(source, data)
		if err != nil {
			return nil, err
		}
		return func() (agentos.YAMLAgentSession, error) { return open(ycode.WithHarnessProvider("openai", provider)) }, nil
	}
	for _, definition := range []string{"agent.yaml", "genie/agent.yaml"} {
		path, _ := filepath.Abs(filepath.Join("..", "..", "..", "ycode", "examples", definition))
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, embedded := range []bool{false, true} {
			decl := "~~~agent as searcher\n" + string(raw) + "\n~~~\n"
			if embedded {
				decl = "embed agent \"./agent.yaml\" as searcher\n"
			}
			for _, tc := range []struct {
				body, want string
				calls      int
			}{
				{"@guard(effects: \"exec,net,spend\")\n@ensure('test \"$RESULT\" = \"recorded answer\"')\nagentic func query() string { return \"recorded answer\"; }\nagentic { a := query(); echo \"$a\"; }", "recorded answer", 0},
				{"agentic { a, err := searcher.run(\"request\"); echo \"$a\"; }", "recorded answer", 1},
				{"agentic func query() string { a, err := searcher.run(\"request\"); return a; }\nagentic { a := query(); echo \"$a\"; }", "recorded answer", 1},
				{"@guard(effects: \"exec,net,spend\")\nagentic func query() string { a, err := searcher.run(\"request\"); return a; }\nagentic { a := query(); echo \"$a\"; }", "recorded answer", 1},
				{"@ensure('test \"$RESULT\" = \"recorded answer\"')\nagentic func query() string { a, err := searcher.run(\"request\"); return a; }\nagentic { a := query(); echo \"$a\"; }", "recorded answer", 1},
				{"answer, err := searcher.run(\"request\")", "agentic action requires", 0},
				{"@guard(effects: \"read\")\nagentic func query() { a, err := searcher.run(\"request\"); }\nagentic { query(); }", "exceed the guard", 0},
				{"@guard(effects: \"read,write,exec,net,spend\")\n@ensure('test \"$RESULT\" = \"recorded answer\"')\nagentic func query() string { a, err := searcher.run(\"request\"); return a; }\nagentic { a := query(); echo \"$a\"; }", "recorded answer", 1},
			} {
				before := provider.calls
				program, err := syntax.NewParser(syntax.Variant(syntax.LangBashPP)).Parse(strings.NewReader(decl+tc.body+"\n"), filepath.Join(filepath.Dir(path), "fence.bsh"))
				if err != nil {
					t.Fatal(err)
				}
				var out, diagnostic bytes.Buffer
				runner, err := interp.New(interp.Lang(syntax.LangBashPP), interp.Env(nil))
				if err != nil {
					t.Fatal(err)
				}
				for _, option := range agentos.WireExec(nil, false, os.Environ(), nil, &out, &diagnostic) {
					if err := option(runner); err != nil {
						t.Fatal(err)
					}
				}
				// Assert language results directly; terminal output reduction has
				// its own lifecycle tests and is not this fixture's transport.
				if err := interp.StdIO(nil, &out, &diagnostic)(runner); err != nil {
					t.Fatal(err)
				}
				runErr := runner.Run(context.Background(), program)
				if tc.want == "recorded answer" && (out.String() != "recorded answer\n" || runErr != nil || diagnostic.Len() != 0) {
					t.Fatalf("judged result failed: output=%q diagnostic=%q err=%v", out.String(), diagnostic.String(), runErr)
				}
				if provider.calls-before != tc.calls || !strings.Contains(out.String()+diagnostic.String(), tc.want) {
					t.Fatalf("%s embed=%v calls=%d output=%q diagnostic=%q err=%v", definition, embedded, provider.calls-before, out.String(), diagnostic.String(), runErr)
				}
			}
		}
	}
}
