// Command agentic-example embeds Bash++ with one example-only native tool.
// It does not add a command to the standard bashy distribution.
package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/qiangli/bashy/internal/agentos"
	"github.com/qiangli/bashy/internal/cli"
	"github.com/qiangli/coreutils/pkg/chat"
	"github.com/qiangli/coreutils/tool"
	"mvdan.cc/sh/v3/interp"
)

func main() {
	tool.Register(summaryTool("example-summary", nil))
	cli.AgentOSDispatch = agentos.Dispatch
	cli.AgentOSWireExec = agentos.WireExec
	cli.AgentOSBashPPDefault = true
	cli.Main()
}

// summaryTool demonstrates the existing native tool boundary. The shell's
// handler context survives coreutils dispatch inside rc.Ctx; no environment
// variable supplies the source opt-in. Provider configuration remains owned by
// chat and its host. SUMMARY_AGENT selects a binding, not additional authority.
// A nil runner uses chat's normal governed production launcher.
func summaryTool(name string, runner chat.Runner) *tool.Tool {
	return &tool.Tool{Name: name, Synopsis: "summarize text in an agentic scope", Run: func(rc *tool.RunContext, args []string) int {
		fail := func(err error, code int) int {
			fmt.Fprintf(rc.Err, "%s: %v\n", name, err)
			return code
		}
		if !interp.HandlerCtx(rc.Ctx).Agentic {
			return fail(fmt.Errorf("requires an explicit agentic block"), 2)
		}
		if len(args) > 1 {
			return fail(fmt.Errorf("expected one text argument, or text on stdin"), 2)
		}
		var agent string
		for _, entry := range rc.Env {
			if value, ok := strings.CutPrefix(entry, "SUMMARY_AGENT="); ok {
				agent = value
			}
		}
		if agent == "" {
			return fail(fmt.Errorf("set SUMMARY_AGENT to an existing configured agent"), 2)
		}
		var input string
		if len(args) == 1 {
			input = args[0]
		} else {
			data, err := io.ReadAll(rc.In)
			if err != nil {
				return fail(err, 1)
			}
			input = string(data)
		}
		result, err := chat.Invoke(rc.Ctx, chat.Options{
			Agent: agent, Cwd: rc.Dir, ReadOnly: true,
			Instruction: "Summarize the following text in one sentence. Return only the summary.\n\n" + input,
		}, runner)
		if _, writeErr := io.WriteString(rc.Out, result.Output); writeErr != nil {
			return fail(writeErr, 1)
		}
		if err != nil {
			code := result.ExitCode
			if code < 1 || code > 255 {
				code = 1
			}
			return fail(err, code)
		}
		return result.ExitCode
	}}
}
