package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qiangli/bashy/internal/agentos"
	"github.com/qiangli/coreutils/tool"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

type fixtureRunner struct {
	calls  int
	args   []string
	cwd    string
	scope  bool
	output string
	status int
	err    error
	wait   bool
}

func (f *fixtureRunner) Run(ctx context.Context, _ string, args []string, cwd string) (string, int, error) {
	f.calls++
	f.args, f.cwd, f.scope = args, cwd, interp.HandlerCtx(ctx).Agentic
	if f.wait {
		<-ctx.Done()
		return "", 1, ctx.Err()
	}
	return f.output, f.status, f.err
}

var fixtureNumber atomic.Uint64

func isolate(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, key := range []string{"HOME", "BASHY_HOME", "BASHY_ROOM_DIR", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "BASHY_TOOLS_DIR", "BASHY_AGENTS_DIR", "BASHY_MODELS_DIR"} {
		t.Setenv(key, filepath.Join(root, key))
	}
	for _, key := range []string{"BASHY_EXECHIST", "BASHY_ADVISOR", "BASHY_HINTS", "BASHY_LEARN", "BASHY_KNOWLEDGE", "BASHY_FORCE_AGENT_SHELL", "BASHY_OUTPUT_REDUCE"} {
		t.Setenv(key, "off")
	}
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("BASHY_AUDIT", "")
	t.Setenv("BASHY_LLM_BUDGET_STATE", filepath.Join(root, "budget.json"))
	t.Setenv("SUMMARY_AGENT", "codex")
	t.Setenv("BASHY_FORCE_AGENT_SHELL", "0")
	return root
}

// Run the shipped embedding with the real CLI's file, stdin and -c entry paths.
// Subprocess isolation is necessary because cli.Main owns process exit.
func TestAgenticExampleCLIHelper(t *testing.T) {
	if os.Getenv("AGENTIC_EXAMPLE_CLI_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"agentic-example"}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}
	t.Fatal("missing helper argument separator")
}

func TestAgenticExampleProductEntries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable shell fixture uses /bin/sh")
	}
	root := isolate(t)
	t.Setenv("AGENTIC_EXAMPLE_CLI_HELPER", "1")
	t.Setenv("BASHY_AGENTIC", "0")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	// The existing baseline codex binding launches this deterministic local
	// executable. No API, model or paid inference is involved. It receives the
	// real launcher's argv, cwd and governed child environment.
	provider := "#!/bin/sh\nfor arg do last=$arg; done\ncase $last in *'input with spaces'*) ;; *) printf '%s\\n' 'unexpected fixture input' >&2; exit 9;; esac\nprintf '%s' fixture-summary\nexit ${AGENTIC_FIXTURE_STATUS:-0}\n"
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte(provider), 0700); err != nil {
		t.Fatal(err)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	launcher := "#!/bin/sh\nexec " + quote(exe) + " -test.run=TestAgenticExampleCLIHelper -- \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "bashy"), []byte(launcher), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	examples, err := filepath.Abs("../../examples/agentic")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		args  []string
		input string
	}{
		{"native command", []string{"--bashpp", "-c", `agentic { example-summary "input with spaces"; }`}, ""},
		{"native stdin", []string{"--bashpp"}, `agentic { example-summary "input with spaces"; }`},
		{"typed method file", []string{"--bashpp", filepath.Join(examples, "typed.bpp"), "input with spaces"}, ""},
		{"shell script file", []string{"--bashpp", filepath.Join(examples, "summarize.bpp"), "input with spaces"}, ""},
		{"shell script stdin", []string{"--bashpp", filepath.Join(examples, "summarize.bpp")}, "input with spaces"},
		{"external script tool", []string{"--bashpp", "-c", quote(filepath.Join(examples, "summarize.bpp")) + ` "input with spaces"`}, ""},
		{"classic explicit chat", []string{"--no-bashpp", "-c", `bashy chat --plain --read-only --agent codex -m "input with spaces"`}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, exe, append([]string{"-test.run=TestAgenticExampleCLIHelper", "--"}, tc.args...)...)
			cmd.Dir, cmd.Stdin = root, strings.NewReader(tc.input)
			var out, errOut bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errOut
			if err := cmd.Run(); err != nil || out.String() != "fixture-summary" || errOut.Len() != 0 {
				t.Fatalf("out=%q stderr=%q err=%v", out.String(), errOut.String(), err)
			}
		})
	}
	t.Setenv("AGENTIC_FIXTURE_STATUS", "7")
	cmd := exec.Command(exe, "-test.run=TestAgenticExampleCLIHelper", "--", "--bashpp", filepath.Join(examples, "typed.bpp"), "input with spaces")
	cmd.Dir = root
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err == nil || out.Len() != 0 || !strings.Contains(errOut.String(), "exit status") {
		t.Fatalf("typed failure: out=%q stderr=%q err=%v", out.String(), errOut.String(), err)
	}
	for _, binding := range []string{"", "sprint134-unconfigured-agent"} {
		t.Setenv("SUMMARY_AGENT", binding)
		cmd := exec.Command(exe, "-test.run=TestAgenticExampleCLIHelper", "--", "--bashpp", filepath.Join(examples, "typed.bpp"), "input with spaces")
		cmd.Dir = root
		var out, errOut bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errOut
		if err := cmd.Run(); err == nil || out.Len() != 0 || errOut.Len() == 0 {
			t.Fatalf("binding=%q: out=%q stderr=%q err=%v", binding, out.String(), errOut.String(), err)
		}
	}
}

func runExample(t *testing.T, ctx context.Context, root, source, input string, f *fixtureRunner, lang syntax.LangVariant) (string, string, error) {
	t.Helper()
	name := fmt.Sprintf("example-summary-test-%d", fixtureNumber.Add(1))
	tool.Register(summaryTool(name, f))
	source = strings.ReplaceAll(source, "SUMMARY", name)
	var out, errOut bytes.Buffer
	opts := []interp.RunnerOption{interp.Lang(lang), interp.Dir(root), interp.Env(expand.ListEnviron(os.Environ()...))}
	opts = agentos.WireSessionExec(false)(opts, lang == syntax.LangPOSIX, os.Environ(), strings.NewReader(input), &out, &errOut)
	r, err := interp.New(opts...)
	if err != nil {
		t.Fatal(err)
	}
	node, err := syntax.NewParser(syntax.Variant(lang)).Parse(strings.NewReader(source), "agentic-example.bpp")
	if err != nil {
		t.Fatal(err)
	}
	err = r.Run(ctx, node)
	return out.String(), errOut.String(), err
}

func TestAgenticExampleDispatch(t *testing.T) {
	root := isolate(t)
	for _, ambient := range []string{"", "1", "0"} {
		t.Run("ambient="+ambient, func(t *testing.T) {
			t.Setenv("BASHY_AGENTIC", ambient)
			f := &fixtureRunner{output: "summary\n"}
			out, errOut, err := runExample(t, context.Background(), root, `agentic { SUMMARY "input with spaces"; }; printf '%s' ordinary`, "", f, syntax.LangBashPP)
			if err != nil || errOut != "" || out != "summary\nordinary" {
				t.Fatalf("out=%q stderr=%q err=%v", out, errOut, err)
			}
			if f.calls != 1 || !f.scope || f.cwd != root || !strings.HasSuffix(f.args[len(f.args)-1], "\n\ninput with spaces") {
				t.Fatalf("provider request: %+v", f)
			}
		})
	}
}

func TestAgenticExampleScopeAndClassic(t *testing.T) {
	root := isolate(t)
	for _, tc := range []struct {
		name, source string
		lang         syntax.LangVariant
	}{
		{"unmarked", `SUMMARY hello`, syntax.LangBashPP},
		{"ordinary helper", `function helper() { SUMMARY hello; }; agentic { helper; }`, syntax.LangBashPP},
		{"restored", `agentic { printf '%s' before; }; SUMMARY hello`, syntax.LangBashPP},
		{"classic", `SUMMARY hello`, syntax.LangBash},
		{"posix", `SUMMARY hello`, syntax.LangPOSIX},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fixtureRunner{}
			_, errOut, err := runExample(t, context.Background(), root, tc.source, "", f, tc.lang)
			if status, ok := interp.IsExitStatus(err); !ok || status != 2 || f.calls != 0 || !strings.Contains(errOut, "requires an explicit agentic block") {
				t.Fatalf("calls=%d stderr=%q err=%v", f.calls, errOut, err)
			}
		})
	}
	f := &fixtureRunner{}
	out, errOut, err := runExample(t, context.Background(), root, `printf '%s' ordinary; cat`, " input", f, syntax.LangBash)
	if err != nil || out != "ordinary input" || errOut != "" || f.calls != 0 {
		t.Fatalf("out=%q stderr=%q err=%v calls=%d", out, errOut, err, f.calls)
	}
}

func TestAgenticExampleStdinAndFailures(t *testing.T) {
	root := isolate(t)
	f := &fixtureRunner{output: "stdin summary"}
	out, errOut, err := runExample(t, context.Background(), root, `agentic { SUMMARY; }`, "first\nsecond\n", f, syntax.LangBashPP)
	if err != nil || out != "stdin summary" || errOut != "" || !strings.HasSuffix(f.args[len(f.args)-1], "first\nsecond") {
		t.Fatalf("out=%q stderr=%q err=%v request=%+v", out, errOut, err, f)
	}
	for _, providerErr := range []error{errors.New("provider unavailable"), nil} {
		f := &fixtureRunner{output: "partial", status: 7, err: providerErr}
		out, errOut, err := runExample(t, context.Background(), root, `agentic { SUMMARY hello; }`, "", f, syntax.LangBashPP)
		if status, ok := interp.IsExitStatus(err); !ok || status != 7 || out != "partial" || (providerErr != nil && !strings.Contains(errOut, "provider unavailable")) {
			t.Fatalf("out=%q stderr=%q err=%v", out, errOut, err)
		}
	}
}

func TestAgenticExampleCancellation(t *testing.T) {
	root := isolate(t)
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	f := &fixtureRunner{wait: true}
	_, errOut, err := runExample(t, ctx, root, `agentic { SUMMARY hello; }`, "", f, syntax.LangBashPP)
	if err == nil || f.calls != 1 || !strings.Contains(errOut, "context deadline exceeded") {
		t.Fatalf("calls=%d stderr=%q err=%v", f.calls, errOut, err)
	}
}

func TestAgenticExampleTypedPresentations(t *testing.T) {
	root := isolate(t)
	f := &fixtureRunner{output: "typed summary"}
	source := `agentic func summarize(text string) (string, error) {
    output, err := capture("SUMMARY", "$text")
    return output, err
}
type Report string
agentic func (r Report) Summarize() (string, error) {
    text := string(r)
    output, err := summarize(text)
    return output, err
}
agentic {
    var report Report = "input"
    output, err := report.Summarize()
    printf '%s|%s' "$output" "$err"
}`
	out, errOut, err := runExample(t, context.Background(), root, source, "", f, syntax.LangBashPP)
	if err != nil || errOut != "" || out != "typed summary|" || f.calls != 1 {
		t.Fatalf("out=%q stderr=%q err=%v calls=%d", out, errOut, err, f.calls)
	}
}
