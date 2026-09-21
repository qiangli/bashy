package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"mvdan.cc/sh/v3/polyglot"
)

// polyglot_rows.go — the text fence rows whose processor is one of bashy's
// own verbs (B30, bashsharp/docs/fenced-text-blocks-plan.md). The engine
// ships `dockerfile` and `tf`, whose tools exist on their own; these rows
// are bashy's because their processors are: `bashy kubectl` and `bashy helm`
// (provisioned CLIs), `bashy dag` (the task language) and `bashy skills`
// (the skill runner). They register through the same polyglot.RegisterLanguage
// an embedder uses, which is the point: sh never imports the dag package.
//
//	~~~k8s as app     app.apply() app.delete() app.get() app.diff()  — kubectl
//	                  app.play()  app.down()                         — podman kube, local
//	~~~helm as chart  chart.template(rel, chart) chart.install(…) chart.upgrade(…) chart.uninstall(rel)
//	~~~dag as ci      ci.<target>()  — the fenced file's own targets, from `bashy dag --list --json`
//	~~~skill as s     s.run() s.run(target) s.verify() s.probe()
//	~~~compose        reserved: refuses by name until the engine has a native path

// K8s: a manifest applied through kubectl, or played locally by podman.
var k8sRow = polyglot.Text{Type: "k8s", FileName: "manifest.yaml", Tool: "kubectl", Verbs: []polyglot.Verb{
	{Name: "apply", Args: []string{"apply", "-f", "{file}"}, Effects: []string{"remote", "write"}},
	{Name: "delete", Args: []string{"delete", "-f", "{file}"}, Effects: []string{"remote", "destroy"}},
	{Name: "get", Args: []string{"get", "-f", "{file}", "-o", "wide"}, Effects: []string{"remote", "read"}},
	{Name: "diff", Args: []string{"diff", "-f", "{file}"}, Effects: []string{"remote", "read"}},
	{Name: "play", Tool: "podman", Args: []string{"kube", "play", "{file}"}, Effects: []string{"exec", "write"}},
	{Name: "down", Tool: "podman", Args: []string{"kube", "down", "{file}"}, Effects: []string{"exec", "destroy"}},
}}

// Helm: a values file; the release and the chart are the call's arguments
// (`chart.install("web", "./charts/web")`), resolved from the caller's
// directory.
var helmRow = polyglot.Text{Type: "helm", FileName: "values.yaml", Tool: "helm", WorkDir: "{cwd}", Verbs: []polyglot.Verb{
	{Name: "template", Args: []string{"template", "-f", "{file}"}, Effects: []string{"net", "read"}},
	{Name: "install", Args: []string{"install", "-f", "{file}"}, Effects: []string{"remote", "write", "net"}},
	{Name: "upgrade", Args: []string{"upgrade", "--install", "-f", "{file}"}, Effects: []string{"remote", "write", "net"}},
	{Name: "uninstall", Args: []string{"uninstall"}, Effects: []string{"remote", "destroy"}},
}}

// Skill: the fence's SKILL.md is the one skill of a private local ring
// rooted at the fence, named `skill`. `run` takes an optional dag target.
var skillRow = polyglot.Text{Type: "skill", FileName: "skill/SKILL.md", Tool: "skills", WorkDir: "{cwd}", Verbs: []polyglot.Verb{
	{Name: "run", Args: []string{"run", "skill"}, Env: []string{"BASHY_SKILLS_DIR={root}"}, Effects: []string{"exec"}},
	{Name: "verify", Args: []string{"verify", "skill"}, Env: []string{"BASHY_SKILLS_DIR={root}"}, Effects: []string{"read"}},
	{Name: "probe", Args: []string{"probe"}, Env: []string{"BASHY_SKILLS_DIR={root}"}, Effects: []string{"read"}},
}}

func init() {
	k8s := polyglot.TextRow("k8s", []string{"kube"}, k8sRow)
	polyglot.RegisterLanguage(k8s)
	polyglot.RegisterLanguage(polyglot.TextRow("helm", nil, helmRow))
	skill := polyglot.TextRow("skill", nil, skillRow)
	skill.InterpretedOnly = true
	polyglot.RegisterLanguage(skill)
	polyglot.RegisterLanguage(dagRow())
	polyglot.RegisterLanguage(stubRow("compose", "compose: not yet supported — the row is reserved (up, down, ps, logs) until the embedded engine has a native path; podman kube play under the k8s row is the local target"))
}

// dagRow is `~~~dag`: the fenced dag.md's targets are the methods, answered
// from `bashy dag --list --json`, and a call runs `bashy dag <file> <target>`
// in the caller's directory — dag bodies address the checkout, not the
// cache. A target's `Effects:` line is the method's declared effects.
func dagRow() polyglot.Language {
	return polyglot.Language{
		Canonical: "dag", Text: true, InterpretedOnly: true,
		NewRuntime: func(cfg polyglot.RuntimeConfig) polyglot.LanguageRuntime {
			return polyglot.RunnerFence{Type: "dag", FileName: "dag.md", Runner: "dag",
				Invoke: func(ctx context.Context, argv []string) (string, error) { return dagInvoke(ctx, cfg, argv) }}
		},
		LoweredRuntime: func(prefix, _ string) string { return prefix + "polyglot.RunnerFence{Type:\"dag\"}" },
	}
}

func dagInvoke(ctx context.Context, cfg polyglot.RuntimeConfig, argv []string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	verb, file := argv[0], argv[1]
	if verb == polyglot.MethodsVerb {
		out, err := selfOutput(ctx, cfg, exe, "dag", "--list", "--json", file)
		if err != nil {
			return "", err
		}
		return dagMethods(out)
	}
	// --quiet keeps the dag runner's banners and its summary line out of
	// the value: a target's stdout is the method's result.
	args := append([]string{"dag", "--quiet", file, verb}, argv[2:]...)
	return selfOutput(ctx, cfg, exe, args...)
}

// dagMethods turns the dag list envelope into method lines.
func dagMethods(listJSON string) (string, error) {
	var envelope struct {
		Status string `json:"status"`
		Result struct {
			Tasks []struct {
				Name    string   `json:"name"`
				Effects []string `json:"effects"`
			} `json:"tasks"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(listJSON), &envelope); err != nil {
		return "", fmt.Errorf("dag --list --json: %v", err)
	}
	var out strings.Builder
	for _, task := range envelope.Result.Tasks {
		line, _ := json.Marshal(map[string]any{"name": task.Name, "effects": task.Effects})
		out.Write(line)
		out.WriteByte('\n')
	}
	return out.String(), nil
}

// selfOutput runs one of bashy's own verbs from the caller's directory and
// returns its stdout without trailing newlines; a failing status is the
// error, with stderr in it.
func selfOutput(ctx context.Context, cfg polyglot.RuntimeConfig, exe string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = cfg.CallerDir()
	if env := cfg.CallerEnv(); len(env) > 0 {
		cmd.Env = append([]string(nil), env...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return strings.TrimRight(stdout.String(), "\n"), fmt.Errorf("bashy %s exited %d: %s", args[0], exit.ExitCode(), strings.TrimSpace(stderr.String()))
		}
		return "", err
	}
	if stderr.Len() > 0 {
		fmt.Fprint(os.Stderr, stderr.String())
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}

// stubRow reserves a type in the table and refuses it by name at prepare.
func stubRow(canonical, reason string) polyglot.Language {
	refuse := func(context.Context, string) ([]polyglot.Export, error) { return nil, errors.New(reason) }
	return polyglot.Language{
		Canonical: canonical, Text: true,
		NewRuntime: func(polyglot.RuntimeConfig) polyglot.LanguageRuntime {
			return polyglot.Embedded{RuntimeName: canonical, AnalyzeFunc: refuse}
		},
		LoweredRuntime: func(prefix, _ string) string { return prefix + "polyglot.Embedded{}" },
	}
}
