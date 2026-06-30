// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// `bashy run` wraps a single command and emits ONE structured record bundling
// the result with bashy's agentic meta — the generic alternative to per-tool
// --json output. The unique value is the META (non-lossy exit/signal, duration,
// cwd, and the space-time advisor's hints), not reformatting the command's
// output: agents read plain output fine, so by default the streams pass through
// live (tee) and only a compact meta line is added.
//
//	bashy run -- go test ./...        # output live; trailing {…meta…} on stderr
//	bashy run --capture -- ssh host   # no live output; one JSON record on stdout
//	                                  # (embeds stdout/stderr) — for logging/transport
//
// stdout stays pure in stream mode (the meta goes to stderr), so `bashy run` is
// pipeable and exit-transparent (it returns the command's own status).
package agentos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

const runSchemaVersion = "bashy-run-v1"

type runHint struct {
	Dimension string `json:"dimension"`
	Retryable bool   `json:"retryable"`
	Text      string `json:"text"`
	Suggest   string `json:"suggest,omitempty"`
}

type runEnvelope struct {
	Schema     string    `json:"schema_version"`
	Argv       []string  `json:"argv"`
	Cwd        string    `json:"cwd"`
	Exit       int       `json:"exit"`
	Signaled   bool      `json:"signaled,omitempty"`
	DurationMs int64     `json:"duration_ms"`
	Stdout     string    `json:"stdout,omitempty"` // populated only with --capture
	Stderr     string    `json:"stderr,omitempty"` // populated only with --capture
	Hints      []runHint `json:"hints,omitempty"`
}

// dispatchRun implements `bashy run`. It runs before shell flag parsing, so it
// uses the process stdio/cwd/env directly.
func dispatchRun(args []string) int {
	capture := false
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			i++
			break
		}
		if a == "" || a[0] != '-' || a == "-" {
			break
		}
		switch a {
		case "--capture":
			capture = true
		default:
			fmt.Fprintf(os.Stderr, "bashy run: unknown option %q\n", a)
			return 2
		}
	}
	argv := args[i:]
	if len(argv) == 0 {
		fmt.Fprintln(os.Stderr, "usage: bashy run [--capture] [--] command [args...]")
		return 2
	}

	env, status := runCommand(argv, capture, os.Stdout, os.Stderr)
	b, _ := json.Marshal(env)
	if capture {
		fmt.Fprintln(os.Stdout, string(b)) // the record IS the output
	} else {
		fmt.Fprintln(os.Stderr, string(b)) // streams went live; meta trails on stderr
	}
	return status
}

// runCommand executes argv and returns its result envelope + exit status. In
// stream mode the command's stdout/stderr go to liveOut/liveErr; with capture
// they are buffered into the envelope instead.
func runCommand(argv []string, capture bool, liveOut, liveErr io.Writer) (runEnvelope, int) {
	cwd, _ := os.Getwd()
	c := exec.Command(argv[0], argv[1:]...)
	var ob, eb bytes.Buffer
	if capture {
		c.Stdout, c.Stderr = &ob, &eb
	} else {
		c.Stdout, c.Stderr = liveOut, liveErr
	}
	start := time.Now()
	if err := c.Start(); err != nil {
		// could not start (not found / not executable) — 127, like a shell.
		return runEnvelope{Schema: runSchemaVersion, Argv: argv, Cwd: cwd, Exit: 127}, 127
	}
	_ = c.Wait()
	status, signaled := procStatus(c.ProcessState) // 128+sig on a signal (platform helper)

	env := runEnvelope{
		Schema:     runSchemaVersion,
		Argv:       argv,
		Cwd:        cwd,
		Exit:       status,
		Signaled:   signaled,
		DurationMs: time.Since(start).Milliseconds(),
	}
	// Reuse the space-time advisor's pattern library for the hint, as structured
	// data rather than a stderr prose line.
	if h := newAdvisor().advise(cwd, argv, status); h != nil {
		env.Hints = []runHint{{Dimension: h.dimension, Retryable: h.retryable, Text: h.text, Suggest: h.suggest}}
	}
	if capture {
		env.Stdout, env.Stderr = ob.String(), eb.String()
	}
	return env, status
}
