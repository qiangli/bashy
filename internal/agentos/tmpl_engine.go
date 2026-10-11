package agentos

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/bashy/internal/tmpl"
)

// tmpl_engine.go — `bashy tmpl render --engine ENGINE`. The CLI resolves
// ENGINE as the ~~~tmpl fence's `!ENGINE` runner does for a command: a
// `bashy commands add` record or a command bashy dispatches itself, never a
// program on PATH. A Bash# function engine lives in a unit, so only the
// fence reaches it.

func init() {
	tmpl.ResolveEngine = tmplEngine
}

func tmplEngine(name string) (tmpl.Engine, bool) {
	if rec, ok := registeredLookup(name); ok {
		return func(ctx context.Context, argv []string, stderr io.Writer) (string, error) {
			bound, err := interp.BindCommandSchema(rec.Name, registeredSchema(rec.Args), append([]string{rec.Name}, argv...))
			if err != nil {
				return "", err
			}
			full, err := registeredArgv(ctx, rec, bound[1:])
			if err != nil {
				return "", err
			}
			cmd := exec.CommandContext(ctx, full[0], full[1:]...)
			cmd.Env = append(os.Environ(), rec.Env...)
			cmd.Dir = rec.Cwd
			return tmplEngineRun(cmd, stderr)
		}, true
	}
	if fenceRunnerRegistered(name) {
		return func(ctx context.Context, argv []string, stderr io.Writer) (string, error) {
			return tmplEngineRun(exec.CommandContext(ctx, bashySelfPath(), append([]string{name}, argv...)...), stderr)
		}, true
	}
	return nil, false
}

func tmplEngineRun(cmd *exec.Cmd, stderr io.Writer) (string, error) {
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return out.String(), fmt.Errorf("exited %d", ee.ExitCode())
		}
		return out.String(), err
	}
	return out.String(), nil
}
