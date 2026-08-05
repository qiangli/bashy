// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// The agentic history recorder: every command bashy dispatches, and what each
// one taught about this machine's environment.
//
// bashy's `history` builtin is bash-exact and therefore interactive-only — it
// records nothing on the script path, nothing on `-c`, and nothing on the
// ExecHandler path an agent drives. So the one process that sees every command
// an agent runs kept no usable record of it. This file is that record.
//
// It writes two planes from one observation:
//
//	TIME   pkg/execlog     what ran, in what order, with what outcome
//	SPACE  pkg/spacegraph  what the environment IS — hosts, endpoints, accounts,
//	                       and the relations between them, accumulated over time
//
// Three properties it must have, because it sits on the hot path of every
// command:
//
//   - It never changes the outcome. The underlying error is returned unchanged
//     on every path, and a store failure is swallowed. A shell that failed a
//     command because its note-taking broke would be indefensible.
//   - It costs one write(2) on an already-open fd. The measured median
//     dispatched command completes in under a millisecond — these are the
//     in-process coreutils calls that are bashy's whole structural advantage —
//     so the audit log's flock+fsync model is not affordable here at any price.
//   - It is resolved ONCE. When the recorder is off, no middleware is appended
//     to the chain at all, and nothing is opened until the first record.
package agentos

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/coreutils/pkg/craft"
	"github.com/qiangli/coreutils/pkg/execlog"
	"github.com/qiangli/coreutils/pkg/policy/coord"
	"github.com/qiangli/coreutils/pkg/redact"
	"github.com/qiangli/coreutils/pkg/spacegraph"
	"github.com/qiangli/coreutils/pkg/weavecli"
)

// execHistEnabled reports whether the recorder should run.
//
// It follows the advisor's gating rather than inventing a posture of its own:
// on by default for agents, off for interactive humans, with BASHY_EXECHIST as
// the explicit control and BASHY_AGENTIC as the master kill.
func execHistEnabled() bool {
	if agenticDisabled() {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BASHY_EXECHIST"))) {
	case "0", "false", "off", "no":
		return false
	case "1", "true", "on", "yes":
		return true
	}
	return weavecli.IsAgentDriven()
}

// execHistDir is the store root: BASHY_EXECHIST when it names a path, else a
// per-user default under the bashy home. Mirrors auditPath's ladder.
func execHistDir() string {
	v := strings.TrimSpace(os.Getenv("BASHY_EXECHIST"))
	switch strings.ToLower(v) {
	case "", "1", "true", "on", "yes", "0", "false", "off", "no":
	default:
		return v
	}
	if home := strings.TrimSpace(os.Getenv("BASHY_HOME")); home != "" {
		return filepath.Join(home, "exec")
	}
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return filepath.Join(h, ".bashy", "exec")
	}
	return filepath.Join(os.TempDir(), "bashy-exec")
}

// The per-process constants. Every one of these is resolved lazily and exactly
// once, and that is not a micro-optimisation:
//
// spacetime's place probe is marked Volatile, so it bypasses the persistent
// cache and re-evaluates on every read — on darwin that means fetching the
// whole routing table through sysctl. Calling it per command would be the
// single worst decision available in this file.
var (
	onceEpisode = sync.OnceValue(func() string { return coord.Self().Episode })
	onceSelf    = sync.OnceValue(func() string {
		h, _ := os.Hostname()
		return h
	})
	onceScrubber = sync.OnceValue(func() *redact.Scrubber { return redact.FromHost() })
	oncePID      = sync.OnceValue(os.Getpid)
	oncePPID     = sync.OnceValue(os.Getppid)
	onceHome     = sync.OnceValue(func() string {
		h, _ := os.UserHomeDir()
		return h
	})
)

// recorder holds the two stores and the process-scoped context.
type recorder struct {
	log   *execlog.Writer
	space *spacegraph.Store
}

func newRecorder() *recorder {
	return &recorder{
		log:   execlog.Open(execHistDir()),
		space: spacegraph.Open(craftStoreDir()),
	}
}

// execHistHandler is the post-exec middleware.
func execHistHandler(rec *recorder) func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			// A dry run did not run. Recording it as a success would poison
			// both planes on day one with commands that never executed — and
			// the graph would assert reachability for a host nothing dialled.
			if safeDryRun(ctx) {
				return next(ctx, args)
			}

			// The slot goes down; the advisor fills it on the way back up.
			// See exechist_diag.go for why the diagnosis is handed over rather
			// than recomputed.
			ctx, diag := withDiagSlot(ctx)

			start := time.Now()
			err := next(ctx, args)
			elapsed := time.Since(start)

			argv := commandArgv(args)
			if len(argv) == 0 {
				return err
			}

			// observedExitOf, never exitStatusOf. The narrow one recognises only
			// the shell's own interp.ExitStatus, so it silently skipped every
			// dispatched CLI — the managed externals and the cobra front-door
			// verbs — and an inert recorder reads as coverage while providing
			// none.
			status, observed := observedExitOf(err)

			rec.writeEpisode(ctx, argv, status, observed, elapsed, diag)
			rec.observeSpace(ctx, argv, status, observed)

			return err
		}
	}
}

// writeEpisode appends to the TIME plane.
func (r *recorder) writeEpisode(ctx context.Context, argv []string, status int, observed bool, d time.Duration, diag *diagSlot) {
	cwd := safeHandlerDir(ctx)

	body := execlog.Scrub(onceScrubber(), argv, cwd, execlog.TemplateOpts{
		RepoRoot: repoRootOf(cwd),
		HomeDir:  onceHome(),
		TmpDir:   os.TempDir(),
	})

	rec := execlog.Record{
		At: time.Now().UTC(),
		// Episode is NOT set here — Append owns it, from the argument below, so
		// the field and the filename cannot disagree.
		PID:        oncePID(),
		PPID:       oncePPID(),
		Cmd:        baseName(argv[0]),
		Observed:   observed,
		DurationMs: d.Milliseconds(),
		Effects:    effectsFor(argv[0]),
		Opaque:     isOpaque(argv[0]),
	}
	// Exit stays nil when nothing was observed. "The command ran and returned
	// 0" and "we could not observe an exit" are different claims, and
	// defaulting the second to 0 is how an abstain becomes a pass.
	if observed {
		rec.Exit = &status

		// A "negative answer" is not a failure. grep finding nothing, test
		// being false, diff differing — counting those as failures would
		// promote a pitfall saying grep is unreliable here, and in a real
		// corpus they are 40% of every exit-1 there is.
		//
		// Computed directly rather than taken from the advisor, so it holds
		// even when the advisor is switched off.
		rec.Benign = status != 0 && isBenignExit(argv, status)
	}
	if diag != nil {
		rec.Dimension, rec.Retryable = diag.dimension, diag.retryable
	}

	// Best effort by design: a record that cannot be written is a missed
	// lesson, never a failed command.
	_ = r.log.Append(rec, body, onceEpisode())
}

// observeSpace folds what the command taught into the SPACE plane.
func (r *recorder) observeSpace(ctx context.Context, argv []string, status int, observed bool) {
	if !observed {
		return // nothing ran, or nothing we can interpret
	}

	// FAILURE TEACHES NOTHING, and this mirrors learn.go deliberately.
	//
	// A transport failure is unattributed: `ssh -p 2222 -l xuser host` can fail
	// because the port is wrong, the login is wrong, the host is rebooting, the
	// VPN dropped, or DNS is down. In three of those five the edge was correct,
	// and recording a failure against it would erode the graph every time the
	// network hiccuped. A PAYLOAD failure is different — the session provably
	// worked and only what it carried returned non-zero — so it counts as
	// evidence exactly like success.
	verdict := craft.Classify(argv[0], status)
	_, record := routeVerdict(verdict, status)
	if !record {
		return
	}

	cwd := safeHandlerDir(ctx)
	root := repoRootOf(cwd)

	obs := spacegraph.Observe(argv, true, spacegraph.Context{
		Self:    onceSelf(),
		Repo:    filepath.Base(root),
		Subpath: subpathOf(root, cwd),
		At:      time.Now().UTC(),
		Source:  "exec:" + baseName(argv[0]),
	})
	_ = r.space.RecordAll(obs)
}

// safeHandlerDir reads the working directory without trusting the context.
//
// interp.HandlerCtx panics when the context carries no HandlerContext. That is
// fine in the normal chain and not fine in a middleware that must never be the
// reason a shell dies, so the read is guarded — the same shape the telemetry
// middleware already uses.
func safeHandlerDir(ctx context.Context) (dir string) {
	defer func() {
		if recover() != nil {
			dir = ""
		}
	}()
	return interp.HandlerCtx(ctx).Dir
}

func safeDryRun(ctx context.Context) (dry bool) {
	defer func() {
		if recover() != nil {
			dry = false
		}
	}()
	return interp.HandlerCtx(ctx).DryRun()
}

// repoRootOf walks up for a .git, so paths can stay repo-relative.
//
// A repo-relative path is the cross-link into the code graph and carries no
// identity; the absolute one is somebody's home directory.
func repoRootOf(dir string) string {
	if dir == "" {
		return ""
	}
	for d := dir; ; {
		if fi, err := os.Stat(filepath.Join(d, ".git")); err == nil && (fi.IsDir() || fi.Mode().IsRegular()) {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

// subpathOf reduces a working directory to a coarse repo-relative location.
//
// Truncated to two segments on purpose. Without it every scratch directory is
// its own node and the corpus fragments to singletons — the failure that
// reports no error at all.
func subpathOf(root, dir string) string {
	if root == "" || dir == "" {
		return ""
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	if rel == "." {
		return "."
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) > 2 {
		parts = parts[:2]
	}
	return strings.Join(parts, "/")
}

// isOpaque marks a command whose real work happens in children this process
// never sees.
//
// Nothing crosses an execve: `make test` spawns a compiler, a linker and a test
// binary, and bashy observes none of them. Rendering it as a leaf would report
// twelve seconds of unobserved work as one observed command, which is a
// confident claim about something that was never measured.
func isOpaque(cmd string) bool {
	switch baseName(cmd) {
	case "make", "gmake", "ninja", "cmake", "bazel",
		"npm", "pnpm", "yarn", "cargo", "mvn", "gradle",
		"python", "python3", "ruby", "perl", "node",
		"bash", "sh", "zsh", "docker", "podman":
		return true
	}
	return false
}
