// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
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
// pure `bash` drop-in never sees `--dry-run` — bashy-only, also inert under
// --posix (see WireExec).
// dryRunFlag is the bashy-only --dryrun (consistent with `set -o dryrun`),
// registered here (in agentos, imported only by cmd/bashy) so the pure bash
// drop-in never sees it; also inert under --posix (see WireExec).
var dryRunFlag = flag.Bool("dryrun", false,
	"bashy: print external commands without running them (xtrace without side effects). "+
		"With DHNT_AGENT, emit a JSON manifest (commands present/missing + file destructions) — a preflight/security check.")

// dryRunRequested reports whether --dryrun was passed at startup.
func dryRunRequested() bool { return *dryRunFlag }

// reporter is shared by the exec + open handlers for one dry run.
type dryRunReporter struct {
	agent    bool
	out      io.Writer // manifest (agent) — real stdout
	mu       sync.Mutex
	seenCmd  map[string]bool
	seenDest map[string]bool
}

func newReporter(out io.Writer) *dryRunReporter {
	return &dryRunReporter{agent: weavecli.IsAgent(), out: out,
		seenCmd: map[string]bool{}, seenDest: map[string]bool{}}
}

// ---- JSON-lines manifest entries (agent mode) ----

type cmdEntry struct {
	Kind      string   `json:"kind"` // "command"
	Command   string   `json:"command"`
	Available bool     `json:"available"`
	Resolved  string   `json:"resolved,omitempty"`
	Args      []string `json:"args,omitempty"`
}
type destroyEntry struct {
	Kind      string   `json:"kind"` // "destroy"
	Op        string   `json:"op"`
	Recursive bool     `json:"recursive,omitempty"`
	Paths     []string `json:"paths"`
	Files     int      `json:"files"`
	Bytes     int64    `json:"bytes"`
	Sample    []string `json:"sample,omitempty"`
}
type truncEntry struct {
	Kind  string `json:"kind"` // "truncate"
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

func (r *dryRunReporter) jsonLine(v any) {
	b, _ := json.Marshal(v)
	r.mu.Lock()
	fmt.Fprintf(r.out, "%s\n", b)
	r.mu.Unlock()
}

// command reports one external command (the dependency/security manifest) and,
// if it is destructive, what it would remove.
func (r *dryRunReporter) command(ctx context.Context, args []string) {
	hc := interp.HandlerCtx(ctx)
	name := args[0]
	resolved, ok := resolveCmd(name, hc.Env)
	imp := analyzeDestroy(name, args, hc.Dir)

	if r.agent {
		r.mu.Lock()
		first := !r.seenCmd[name]
		r.seenCmd[name] = true
		r.mu.Unlock()
		if first {
			r.jsonLine(cmdEntry{Kind: "command", Command: name, Available: ok, Resolved: resolved, Args: args})
		}
		if imp != nil {
			imp.Kind = "destroy"
			r.jsonLine(imp)
		}
		return
	}
	// human
	tag := ""
	if !ok {
		tag = "   # MISSING on this system"
	} else if imp != nil {
		tag = fmt.Sprintf("   # ⚠ DESTROYS %d file(s), %s", imp.Files, humanBytes(imp.Bytes))
	}
	fmt.Fprintf(hc.Stderr, "+ %s%s\n", quoteArgs(args), tag)
	if imp != nil && len(imp.Sample) > 0 {
		fmt.Fprintf(hc.Stderr, "    e.g. %s%s\n", strings.Join(imp.Sample, ", "),
			ellipsisIf(imp.Files > len(imp.Sample)))
	}
}

// truncate reports a `>`-style clobber of an existing file (via the OpenHandler).
func (r *dryRunReporter) truncate(stderr io.Writer, path string, size int64) {
	if r.agent {
		r.jsonLine(truncEntry{Kind: "truncate", Path: path, Bytes: size})
		return
	}
	fmt.Fprintf(stderr, "+ > %s   # ⚠ TRUNCATES existing %s\n", shQuote(path), humanBytes(size))
}

// dryRunHandler is the print-and-skip ExecHandler: external commands are
// reported and NOT executed (builtins/assignments/expansions still run).
func dryRunHandler(r *dryRunReporter) func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			if len(args) == 0 || !interp.HandlerCtx(ctx).DryRun() {
				return next(ctx, args) // dry-run off (or `set +o dryrun`): execute
			}
			r.command(ctx, args)
			return nil // skip execution, report success
		}
	}
}

// dryRunOpenHandler intercepts file opens so a redirection like `> f` records a
// truncation instead of performing it. Read-only opens pass through to the real
// file (safe); any write/create/truncate open returns a discard handle so the
// dry run never mutates the filesystem.
func dryRunOpenHandler(r *dryRunReporter) interp.OpenHandlerFunc {
	def := interp.DefaultOpenHandler()
	return func(ctx context.Context, path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
		if !interp.HandlerCtx(ctx).DryRun() {
			return def(ctx, path, flag, perm) // dry-run off: real open
		}
		if flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC|os.O_APPEND) == 0 {
			return def(ctx, path, flag, perm) // read-only: open the real file
		}
		if flag&os.O_TRUNC != 0 {
			abs := path
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(handlerDir(ctx), abs)
			}
			if fi, err := os.Stat(abs); err == nil && !fi.IsDir() && fi.Size() > 0 {
				r.truncate(handlerStderr(ctx), path, fi.Size())
			}
		}
		return nopRWC{}, nil // discard the write — no side effect
	}
}

// analyzeDestroy returns what a destructive command (v1: rm) would remove, by
// walking the real filesystem read-only. Args are already glob-expanded by the
// shell. Returns nil for non-destructive commands or no existing targets.
func analyzeDestroy(name string, args []string, dir string) *destroyEntry {
	if name != "rm" {
		return nil // v1: rm only; mv/dd/truncate are easy follow-ups
	}
	d := &destroyEntry{Op: "rm"}
	for _, a := range args[1:] {
		if len(a) > 1 && a[0] == '-' {
			if a == "--recursive" || strings.ContainsAny(a, "rR") {
				d.Recursive = true
			}
			continue
		}
		d.Paths = append(d.Paths, a)
	}
	for _, p := range d.Paths {
		abs := p
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(dir, abs)
		}
		fi, err := os.Lstat(abs)
		if err != nil {
			continue // nothing there
		}
		if fi.IsDir() {
			if !d.Recursive {
				continue // rm without -r on a dir fails — nothing destroyed
			}
			_ = filepath.WalkDir(abs, func(path string, de fs.DirEntry, err error) error {
				if err != nil || de.IsDir() {
					return nil
				}
				if info, e := de.Info(); e == nil {
					d.Files++
					d.Bytes += info.Size()
				}
				if len(d.Sample) < 8 {
					d.Sample = append(d.Sample, path)
				}
				return nil
			})
		} else {
			d.Files++
			d.Bytes += fi.Size()
			if len(d.Sample) < 8 {
				d.Sample = append(d.Sample, abs)
			}
		}
	}
	if d.Files == 0 {
		return nil
	}
	return d
}

// resolveCmd reports how name resolves on this system: an in-process coreutils
// tool, or a binary on the runner's PATH. ("", false) means it would not be
// found — a missing dependency.
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
	for _, d := range filepath.SplitList(path) {
		if d == "" {
			continue
		}
		cand := filepath.Join(d, name)
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			return cand, true
		}
	}
	return "", false
}

func handlerDir(ctx context.Context) string       { return interp.HandlerCtx(ctx).Dir }
func handlerStderr(ctx context.Context) io.Writer { return interp.HandlerCtx(ctx).Stderr }

type nopRWC struct{}

func (nopRWC) Read([]byte) (int, error)    { return 0, io.EOF }
func (nopRWC) Write(p []byte) (int, error) { return len(p), nil }
func (nopRWC) Close() error                { return nil }

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

func humanBytes(n int64) string {
	const u = 1024
	if n < u {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(u), 0
	for x := n / u; x >= u; x /= u {
		div *= u
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func ellipsisIf(b bool) string {
	if b {
		return ", …"
	}
	return ""
}
