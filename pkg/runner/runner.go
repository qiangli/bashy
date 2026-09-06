// Package runner exposes Bashy's in-process AgentOS shell for embedding.
package runner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/qiangli/bashy/internal/agentos"
	"github.com/qiangli/bashy/internal/cli"
)

// SchemaVersion identifies the stable result envelope returned by this
// package. It intentionally matches the command-line `bashy run` envelope.
const SchemaVersion = "bashy-run-v1"

// Outcome describes how an embedded Bashy invocation finished. A non-zero
// shell status is an observed result (OutcomeExit), not an API error.
type Outcome string

const (
	OutcomeOK        Outcome = "ok"
	OutcomeExit      Outcome = "exit"
	OutcomeSignal    Outcome = "signal"
	OutcomeCancelled Outcome = "cancelled"
	OutcomeTimeout   Outcome = "timeout"
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
	SchemaVersion   string  `json:"schema_version"`
	Outcome         Outcome `json:"outcome"`
	ExitCode        int     `json:"exit"`
	Signaled        bool    `json:"signaled,omitempty"`
	Signal          string  `json:"signal,omitempty"`
	DurationMs      int64   `json:"duration_ms"`
	Stdout          string  `json:"stdout"`
	Stderr          string  `json:"stderr"`
	StdoutTruncated bool    `json:"stdout_truncated"`
	StderrTruncated bool    `json:"stderr_truncated"`
}

// Preflight evaluates one script with Bashy's dry-run option enabled. It uses
// the same parser, interpreter, working directory, environment, stdio, and
// AgentOS handlers as Run, while external commands and filesystem writes are
// reported rather than performed.
func Preflight(ctx context.Context, req Request) Result {
	return run(ctx, req, true)
}

// Run executes one script in a fresh runner. Cancellation is propagated into
// the interpreter and its command handlers.
func Run(ctx context.Context, req Request) Result {
	return run(ctx, req, false)
}

func run(ctx context.Context, req Request, preflight bool) Result {
	var stdout, stderr bytes.Buffer
	env := req.Env
	if env == nil {
		env = os.Environ()
	}
	start := time.Now()
	termination := cli.RunSessionCommandResultWithConfig(ctx, cli.SessionIO{
		Command: req.Script,
		Dir:     req.Dir,
		Env:     env,
		Stdin:   req.Stdin,
		Stdout:  &stdout,
		Stderr:  &stderr,
	}, cli.SessionConfig{
		WireExec: agentos.WireSessionExec(preflight),
		Preamble: agentos.Preamble,
		// Bashy keeps the fork's useful in-process builtins enabled.
		SuppressedForkBuiltins: nil,
	})
	result := Result{
		SchemaVersion: SchemaVersion,
		ExitCode:      termination.ExitCode,
		Signaled:      termination.Signaled,
		Signal:        termination.Signal,
		DurationMs:    time.Since(start).Milliseconds(),
		Outcome:       outcome(ctx, termination.ExitCode, termination.Signaled),
	}
	result.Stdout, result.StdoutTruncated = bounded(stdout.String(), req.MaxOutputChars)
	result.Stderr, result.StderrTruncated = bounded(stderr.String(), req.MaxOutputChars)
	return result
}

func outcome(ctx context.Context, exitCode int, signaled bool) Outcome {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return OutcomeTimeout
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return OutcomeCancelled
	}
	if signaled {
		return OutcomeSignal
	}
	if exitCode != 0 {
		return OutcomeExit
	}
	return OutcomeOK
}

func bounded(value string, limit int) (string, bool) {
	if limit <= 0 || len(value) <= limit {
		return value, false
	}
	return value[:limit], true
}
