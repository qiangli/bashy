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
	"strings"
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
	Schema     string       `json:"schema_version"`
	Argv       []string     `json:"argv"`
	Cwd        string       `json:"cwd"`
	Exit       int          `json:"exit"`
	Signaled   bool         `json:"signaled,omitempty"`
	DurationMs int64        `json:"duration_ms"`
	Stdout     string       `json:"stdout,omitempty"` // populated only with --capture
	Stderr     string       `json:"stderr,omitempty"` // populated only with --capture
	Hints      []runHint    `json:"hints,omitempty"`
	Check      *checkReport `json:"check,omitempty"`
}

// dispatchRun implements `bashy run`. It runs before shell flag parsing, so it
// uses the process stdio/cwd/env directly.
func dispatchRun(args []string) int {
	capture := false
	check := false
	target := ""
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--help" || a == "-h" {
			fmt.Fprintln(os.Stdout, "usage: bashy run [--capture] [--check] [--target NAME] -- command [args...]")
			fmt.Fprintln(os.Stdout, "       bashy run [--capture] [--target NAME] <bundle.bar|archive|dag.md|DAG-directory> [args...]")
			fmt.Fprintln(os.Stdout, "A runnable archive has a root dag.md with a main target. Direct DAGs use --target or their configured default (main when present).")
			return 0
		}
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
		case "--check":
			check = true
		case "--target":
			i++
			if i >= len(args) || args[i] == "" {
				fmt.Fprintln(os.Stderr, "bashy run: --target requires a DAG target name")
				return 2
			}
			target = args[i]
		default:
			if strings.HasPrefix(a, "--target=") {
				target = strings.TrimPrefix(a, "--target=")
				if target == "" {
					fmt.Fprintln(os.Stderr, "bashy run: --target requires a DAG target name")
					return 2
				}
			} else {
				fmt.Fprintf(os.Stderr, "bashy run: unknown option %q\n", a)
				return 2
			}
		}
	}
	argv := args[i:]
	if len(argv) == 0 {
		fmt.Fprintln(os.Stderr, "usage: bashy run [--capture] [--check] [--target NAME] [--] command [args...] | bundle.bar | dag.md | DAG-directory")
		return 2
	}
	if isBarBundle(argv[0]) || isDagEntry(argv[0]) {
		if check {
			fmt.Fprintln(os.Stderr, "bashy run: --check applies to command scripts, not DAGs")
			return 2
		}
		tail, selected, err := extractRunTarget(argv[1:], target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bashy run: %v\n", err)
			return 2
		}
		return dispatchDagEntry(argv[0], selected, tail, capture)
	}

	env, status := runCommand(argv, capture, check, os.Stdout, os.Stderr)
	b, _ := json.Marshal(env)
	if capture {
		fmt.Fprintln(os.Stdout, string(b)) // the record IS the output
	} else {
		fmt.Fprintln(os.Stderr, string(b)) // streams went live; meta trails on stderr
	}
	return status
}

func extractRunTarget(args []string, target string) ([]string, string, error) {
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--target" {
			if target != "" || i+1 >= len(args) || args[i+1] == "" {
				return nil, "", fmt.Errorf("--target requires one DAG target name")
			}
			target = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(a, "--target=") {
			if target != "" || strings.TrimPrefix(a, "--target=") == "" {
				return nil, "", fmt.Errorf("--target requires one DAG target name")
			}
			target = strings.TrimPrefix(a, "--target=")
			continue
		}
		remaining = append(remaining, a)
	}
	return remaining, target, nil
}

// runCommand executes argv and returns its result envelope + exit status. In
// stream mode the command's stdout/stderr go to liveOut/liveErr; with capture
// they are buffered into the envelope instead.
func runCommand(argv []string, capture bool, check bool, liveOut, liveErr io.Writer) (runEnvelope, int) {
	return runCommandAt(argv, capture, check, "", liveOut, liveErr)
}

func runCommandAt(argv []string, capture bool, check bool, dir string, liveOut, liveErr io.Writer) (runEnvelope, int) {
	return runCommandAtEnv(argv, capture, check, dir, nil, liveOut, liveErr)
}

func runCommandAtEnv(argv []string, capture bool, check bool, dir string, extraEnv []string, liveOut, liveErr io.Writer) (runEnvelope, int) {
	cwd, _ := os.Getwd()
	if dir != "" {
		cwd = dir
	}
	var report *checkReport
	if check {
		if script := scriptArgForCheck(argv); script != "" {
			r := newCheckAnalyzer(checkOptions{mode: "bashy", agent: true, maxDepth: 8}).run([]string{script})
			report = &r
			if r.Summary.Errors > 0 {
				env := runEnvelope{Schema: runSchemaVersion, Argv: argv, Cwd: cwd, Exit: 1, Check: report}
				return env, 1
			}
		}
	}
	c := exec.Command(argv[0], argv[1:]...)
	c.Dir = dir
	// The command reads bashy's stdin, as it would under any shell: a piped
	// message or an interactive terminal reaches it (a nil Stdin is /dev/null).
	c.Stdin = os.Stdin
	c.Env = runCommandEnv(os.Environ())
	for _, kv := range extraEnv {
		if key, value, ok := strings.Cut(kv, "="); ok {
			c.Env = setEnv(c.Env, key, value)
		}
	}
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
		Check:      report,
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

func runCommandEnv(env []string) []string {
	env = setEnv(env, "BASHY_AGENTIC", "1")
	env = setEnvDefault(env, "GIT_TERMINAL_PROMPT", "0")
	env = setEnvDefault(env, "GCM_INTERACTIVE", "never")
	return env
}

func setEnv(env []string, name, value string) []string {
	prefix := name + "="
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			if !replaced {
				out = append(out, prefix+value)
				replaced = true
			}
			continue
		}
		out = append(out, kv)
	}
	if !replaced {
		out = append(out, prefix+value)
	}
	return out
}

func setEnvDefault(env []string, name, value string) []string {
	prefix := name + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return env
		}
	}
	return append(env, prefix+value)
}

func scriptArgForCheck(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	switch argv[0] {
	case "bash", "bashy", "sh":
		for _, arg := range argv[1:] {
			if arg == "-c" || arg == "--command" {
				return ""
			}
			if strings.HasPrefix(arg, "-") {
				continue
			}
			return arg
		}
		return ""
	default:
		return argv[0]
	}
}
