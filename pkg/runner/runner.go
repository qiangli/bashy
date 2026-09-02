// Package runner exposes Bashy's in-process AgentOS shell for embedding.
package runner

import (
	"bytes"
	"context"
	"io"
	"os"

	"github.com/qiangli/bashy/internal/agentos"
	"github.com/qiangli/bashy/internal/cli"
)

// Request is one isolated, non-interactive Bashy script execution.
type Request struct {
	Script         string
	Dir            string
	Env            []string
	Stdin          io.Reader
	MaxOutputChars int
}

// Result is the complete bounded shell result.
type Result struct {
	ExitCode        int    `json:"exit_code"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
}

func init() {
	// Match cmd/bashy's compile-time wiring. Embedders get the same shell and
	// pure-Go AgentOS command handler without launching a bashy process.
	cli.AgentOSWireExec = agentos.WireExec
	cli.AgentOSPreamble = agentos.Preamble
	cli.VersionProduct = "bashy"
	cli.VersionCompatibility = "GNU Bash 5.3 compatible"
	cli.SuppressedForkBuiltins = nil
}

// Run executes one script in a fresh runner. Cancellation is propagated into
// the interpreter and its command handlers.
func Run(ctx context.Context, req Request) Result {
	var stdout, stderr bytes.Buffer
	env := req.Env
	if env == nil {
		env = os.Environ()
	}
	result := Result{ExitCode: cli.RunSessionCommand(ctx, cli.SessionIO{
		Command: req.Script,
		Dir:     req.Dir,
		Env:     env,
		Stdin:   req.Stdin,
		Stdout:  &stdout,
		Stderr:  &stderr,
	})}
	result.Stdout, result.StdoutTruncated = bounded(stdout.String(), req.MaxOutputChars)
	result.Stderr, result.StderrTruncated = bounded(stderr.String(), req.MaxOutputChars)
	return result
}

func bounded(value string, limit int) (string, bool) {
	if limit <= 0 || len(value) <= limit {
		return value, false
	}
	return value[:limit], true
}
