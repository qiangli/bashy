// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/qiangli/bashy/internal/cli"
	yokemcp "github.com/qiangli/yoke/mcp"
	"github.com/qiangli/yoke/pkg/atlas"
	"mvdan.cc/sh/v3/interp"
)

// M8 prerequisite evidence: the script tool's session wiring reaches external
// execution for front-door verbs. Intercept at the END of that wiring so this
// test never launches bashy (or recursively launches the Go test executable).
// Replace this evidence test with successful native calls once the session
// dispatcher supports request-local argv, streams, cwd, context and status.
func TestMCPVerbInterpreterRequiresSelfExec(t *testing.T) {
	ringDir(t)
	for _, verb := range []string{"sprint", "weave", "dag", "commands", "kb"} {
		t.Run(verb, func(t *testing.T) {
			var got []string
			var stdout, stderr bytes.Buffer
			exit := cli.RunSessionCommandWithConfig(context.Background(), cli.SessionIO{
				Command: `set -- --help; ` + verb + ` "$@"`, Env: os.Environ(),
				Stdin: bytes.NewReader(nil), Stdout: &stdout, Stderr: &stderr,
			}, cli.SessionConfig{
				Preamble: func() string { return PreambleFor(false) },
				WireExec: func(opts []interp.RunnerOption, posix bool, env []string, stdin io.Reader, stdout, stderr io.Writer) []interp.RunnerOption {
					opts = WireSessionExec(false)(opts, posix, env, stdin, stdout, stderr)
					return append(opts, interp.ExecHandlers(func(_ interp.ExecHandlerFunc) interp.ExecHandlerFunc {
						return func(_ context.Context, args []string) error {
							got = append([]string(nil), args...)
							return interp.ExitStatus(126)
						}
					}))
				},
			})
			want := []string{bashySelfPath(), verb, "--help"}
			if exit != 126 || !reflect.DeepEqual(got, want) {
				t.Fatalf("session dispatch changed; revisit M8 blocker: exit=%d argv=%q want=%q stderr=%q", exit, got, want, stderr.String())
			}
		})
	}
}

// The current yoke API treats every Registered entry as refreshable. A static
// verb cannot be removed from subsequent snapshots either: it disappears.
func TestMCPVerbRegisteredSnapshotIsNotStatic(t *testing.T) {
	ringDir(t)
	entry, ok := atlas.Lookup("sprint")
	if !ok {
		t.Fatal("sprint absent from atlas")
	}
	command := yokemcp.RegisteredCommand{
		Name: "sprint", Synopsis: "static candidate", Effects: entry.Effects, OS: entry.OS,
		Run: func(context.Context, []string, string, string) (string, string, int) { return "initial", "", 0 },
	}
	commands := []yokemcp.RegisteredCommand{command}
	opts := mcpTestOptions(t)
	opts.Registered = func() []yokemcp.RegisteredCommand { return commands }
	ctx, cs, srv := mcpFrontClient(t, opts)
	commands[0].Run = func(context.Context, []string, string, string) (string, string, int) { return "refreshed", "", 0 }
	if err := yokemcp.NotifyToolsChanged(srv, opts); err != nil {
		t.Fatal(err)
	}
	out := mcpSessionDecode[yokemcp.RunToolOutput](t, mcpSessionCall(t, ctx, cs, "sprint", map[string]any{}))
	if out.Stdout != "refreshed" {
		t.Fatalf("unexpected refresh result: %+v", out)
	}
	commands = nil
	if err := yokemcp.NotifyToolsChanged(srv, opts); err != nil {
		t.Fatal(err)
	}
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range list.Tools {
		if item.Name == "sprint" {
			t.Fatal("omitted registered entry unexpectedly survived refresh")
		}
	}
}
