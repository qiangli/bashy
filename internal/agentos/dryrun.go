// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/coreutils/pkg/weavecli"
	"github.com/qiangli/coreutils/tool"
)

// dryRunFlag is registered here (in agentos, imported only by cmd/bashy), so the
// pure `bash` drop-in never sees `--dry-run` — it is a bashy-only extension,
// also inert under --posix (see WireExec).
var dryRunFlag = flag.Bool("dry-run", false,
	"bashy: print external commands without running them (xtrace without side effects). "+
		"With DHNT_AGENT, emit a JSON manifest of the distinct commands + present/missing — a preflight/security check.")

var (
	dryRunMu   sync.Mutex
	dryRunSeen = map[string]bool{}
)

// dryRunEntry is one line of the agent-mode JSON manifest.
type dryRunEntry struct {
	Command   string   `json:"command"`
	Available bool     `json:"available"`
	Resolved  string   `json:"resolved,omitempty"`
	Args      []string `json:"args,omitempty"`
}

// dryRunHandler is the print-and-skip ExecHandler middleware. It intercepts
// every external command (builtins/assignments/expansions still run), reports
// it, and returns success WITHOUT executing — "xtrace without side effects".
//
//   - human: every occurrence as `+ argv` on stderr (a MISSING marker if the
//     command would not resolve on this system).
//   - agent (DHNT_AGENT): each DISTINCT command once, as a JSON line on
//     manifestOut, with availability — the agent's dependency/security preflight.
//
// Limitation (documented): skipped commands return 0, so control flow that
// branches on command output takes the success path — accurate for linear
// scripts, approximate for branchy ones. A static all-branches audit is future.
func dryRunHandler(manifestOut io.Writer) func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	agent := weavecli.IsAgent()
	return func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			if len(args) == 0 {
				return next(ctx, args)
			}
			hc := interp.HandlerCtx(ctx)
			name := args[0]
			resolved, ok := resolveCmd(name, hc.Env)
			if agent {
				dryRunMu.Lock()
				first := !dryRunSeen[name]
				dryRunSeen[name] = true
				dryRunMu.Unlock()
				if first {
					b, _ := json.Marshal(dryRunEntry{Command: name, Available: ok, Resolved: resolved, Args: args})
					fmt.Fprintf(manifestOut, "%s\n", b)
				}
			} else {
				tag := ""
				if !ok {
					tag = "   # MISSING on this system"
				}
				fmt.Fprintf(hc.Stderr, "+ %s%s\n", quoteArgs(args), tag)
			}
			return nil // skip execution, report success
		}
	}
}

// resolveCmd reports how name would resolve on this system: an in-process
// coreutils tool, or a binary on the RUNNER's PATH (honoring a script-set PATH).
// Returns ("", false) when it would not be found — a missing dependency.
func resolveCmd(name string, env expand.Environ) (string, bool) {
	if strings.ContainsRune(name, '/') {
		if fi, err := os.Stat(name); err == nil && !fi.IsDir() {
			return name, true
		}
		return "", false
	}
	if tool.Lookup(name) != nil {
		return "coreutils:" + name, true
	}
	path := ""
	if env != nil {
		if v := env.Get("PATH"); v.IsSet() {
			path = v.String()
		}
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			continue
		}
		cand := filepath.Join(dir, name)
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			return cand, true
		}
	}
	return "", false
}

// quoteArgs renders argv with minimal shell quoting so it is copy-pasteable.
func quoteArgs(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = shQuote(a)
	}
	return strings.Join(parts, " ")
}

func shQuote(s string) string {
	if s == "" {
		return "''"
	}
	for _, r := range s {
		switch {
		case r == '_' || r == '-' || r == '.' || r == '/' || r == ':' || r == '=' || r == '@' || r == ',':
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		default:
			return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
		}
	}
	return s
}
