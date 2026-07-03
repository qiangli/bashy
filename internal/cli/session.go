package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// SessionIO carries the per-request stdio/env/dir for a command run inside a
// warm `bashy serve` session. One serve process builds a fresh runner per
// request from this, so a session-routed `bashy -c "…"` behaves identically to
// a cold `bashy -c "…"` while skipping the process- and package-init tax.
type SessionIO struct {
	Command string    // the -c command string
	Dir     string    // working directory ("" = server cwd)
	Env     []string  // caller environment (replaces os.Environ())
	Stdin   io.Reader // caller stdin
	Stdout  io.Writer // caller stdout
	Stderr  io.Writer // caller stderr
}

// NewSessionRunner builds a runner for one warm-session request. It mirrors
// newRunner's `-c` command path faithfully — same coreutils userland wiring
// (AgentOSWireExec), same bash version vars, PATH default, exported-function
// import and AgentOS preamble — but takes its stdio/env/dir explicitly instead
// of reading process globals, so a single process can serve many callers.
func NewSessionRunner(io SessionIO) (*interp.Runner, error) {
	// SHLVL from the CALLER's environment (not the serve process).
	shlvl := 0
	for _, kv := range io.Env {
		if strings.HasPrefix(kv, "SHLVL=") {
			fmt.Sscanf(kv[len("SHLVL="):], "%d", &shlvl)
		}
	}
	shlvl++

	envVars := make([]string, 0, len(io.Env)+len(bashVersionVars())+1)
	envVars = append(envVars, shellStartupEnv(io.Env)...)
	envVars = append(envVars, bashVersionVars()...)
	envVars = append(envVars, fmt.Sprintf("SHLVL=%d", shlvl))
	if !hasEnvKey(io.Env, "PATH") {
		envVars = append(envVars, "PATH="+defaultPathValue)
	}
	env := expand.ListEnviron(envVars...)

	var r *interp.Runner
	opts := []interp.RunnerOption{
		interp.Interactive(false),
		interp.MirrorUmask(true),
		interp.CommandString(true),
		interp.StandardInput(false),
		interp.StdIO(io.Stdin, io.Stdout, io.Stderr),
		interp.Env(env),
		interp.WithBashCompatErrors(true),
		interp.PromptExpand(func(s string) string {
			return expandPrompt(s, func(name string) string { return r.Env.Get(name).String() }, 0, 0, *posix)
		}),
	}
	if io.Dir != "" {
		opts = append(opts, interp.Dir(io.Dir))
	}
	if *posix {
		opts = append(opts, interp.Params("-o", "posix"))
	}
	// Same in-process coreutils + code-intel userland the cold path gets.
	opts = AgentOSWireExec(opts, *posix)
	if len(SuppressedForkBuiltins) > 0 {
		opts = append(opts, interp.WithDisabledBuiltins(SuppressedForkBuiltins...))
	}
	var err error
	r, err = interp.New(opts...)
	if err != nil {
		return nil, err
	}
	importBashFuncs(r)
	registerDefaultFuncs(r, AgentOSPreamble())
	return r, nil
}

// RunSessionCommand parses and runs the session request's command string in a
// freshly built runner, returning the shell exit status. It is the warm-session
// analogue of the cold `-c` path.
func RunSessionCommand(ctx context.Context, io SessionIO) int {
	r, err := NewSessionRunner(io)
	if err != nil {
		fmt.Fprintln(io.Stderr, "bashy: session:", err)
		return 1
	}
	prog, perr := syntax.NewParser().Parse(strings.NewReader(io.Command), "")
	if perr != nil {
		fmt.Fprintln(io.Stderr, "bashy:", perr)
		return 2
	}
	if err := interp.WithBashSource([]byte(io.Command))(r); err != nil {
		fmt.Fprintln(io.Stderr, "bashy:", err)
		return 1
	}
	runErr := r.Run(ctx, prog)
	var es interp.ExitStatus
	if errors.As(runErr, &es) {
		return int(es)
	}
	if runErr != nil {
		return 1
	}
	return 0
}

// hasEnvKey reports whether env contains a KEY=… entry for the given key.
func hasEnvKey(env []string, key string) bool {
	pfx := key + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, pfx) {
			return true
		}
	}
	return false
}
