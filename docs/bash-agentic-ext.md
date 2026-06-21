# Bash Agentic Extensions — Consolidated Feature TODO

Date: 2026-06-21

This is a single, prioritized feature backlog for extending bash to be a
first-class substrate for AI agents. It is compiled from seven inputs:

- `docs/agentic-extensions.md` — bashy's own design catalogue (the 15 numbered
  extensions; referenced below as **CAT-N**). This is the only source written
  against bashy's Go interpreter (`interp/`) rather than the C bash 5.3 tree, so
  it carries the concrete implementation seams.
- Six independent "if bash were redesigned for agents" wishlists, each written
  from a different agentic tool's perspective:
  - `aider-wishlist.md` (**AID**) — language-modernization lens.
  - `antigravity-wishlist.md` (**ANT**) — hermetic scoping, telemetry, sandboxing.
  - `claude-wishlist.md` (**CL1**) — structured/typed/auditable/scoped.
  - `claude-wishlist2.md` (**CL2**) — reproducibility-first (memoize, hermetic, secrets).
  - `codex-wishlist.md` (**CX**) — process-orchestration-API lens.
  - `opencode-wishlist.md` (**OC**) — broadest surface (17 categories).

Each TODO item records **who asked for it** (consensus is the strongest
prioritization signal), the **bashy seam** it maps to, and an **effort/risk**
estimate. Effort uses the catalogue's S/M/L scale.

---

## How this list was prioritized

Two axes:

1. **Consensus** — how many of the seven independent sources asked for it.
   Six of the seven were written without seeing each other (CL2 explicitly cites
   the others), so overlap is a real signal, not an echo.
2. **Leverage-per-effort against bashy's existing seams.** `interp/api.go`
   already exposes `ExecHandler`, `OpenHandler`, `ReadDirHandler`, `StatHandler`,
   `CallHandler` middleware. Anything that slots into an existing handler is
   cheap; anything needing new grammar (`parse.y`-equivalent) or a threaded type
   system is expensive.

### Consensus heatmap

Counts are out of 7 sources (CAT = bashy catalogue counts as a source).

| Theme | CAT | AID | ANT | CL1 | CL2 | CX | OC | Σ |
|-------|:--:|:--:|:--:|:--:|:--:|:--:|:--:|:--:|
| Structured output channel / events-fd | ● | | ● | ● | ● | ● | ● | **6** |
| `--json` for builtins | ● | | | ● | ● | | ● | **4** |
| Native structured data (JSON/maps/lists) | | ● | ● | ● | ● | ● | ● | **6** |
| Structured errors / result objects | ● | ● | | ● | | ● | ● | **5** |
| try/catch/finally | | ● | ● | ● | (●) | (●) | ● | **6** |
| Sandbox / capability `restrict` | ● | ● | ● | ● | ● | ● | ● | **7** |
| Capability/requirements declaration | ● | | | ● | | ● | ● | **4** |
| Deterministic / hermetic mode | ● | | | ● | ● | | (●) | **4** |
| Content-addressed memoization / cache | | | | ● | ● | | ● | **3** |
| Audit / provenance / journal | ● | | ● | ● | | ● | ● | **5** |
| Record / replay sessions | ● | | | ● | ● | ● | | **4** |
| Out-of-band state inspection | ● | | ● | | (●) | ● | | **4** |
| Dry-run / plan / `--explain` | ● | | | ● | ● | ● | ● | **5** |
| Resource limits (cpu/mem/time/output) | ● | | | ● | | | ● | **3** |
| Structured cancellation / cleanup | ● | | ● | ● | | ● | ● | **5** |
| Bounded parallelism / spawn-await | | ● | | | ● | | ● | **3** |
| Named persistent tasks + readiness | | | | ● | | | (●) | **2** |
| Filesystem transactions / rollback | | | | ● | | ● | ● | **3** |
| Bounded / streaming output | | | | ● | | | ● | **2** |
| Module system / namespacing | | ● | ● | | | | ● | **3** |
| Lexical scope / `local` by default | | | ● | ● | | | ● | **3** |
| Stack traces with source spans | | ● | | ● | ● | ● | ● | **5** |
| Structured diagnostics (editor schema) | ● | | | | ● | ● | ● | **4** |
| Self-describing commands / `--describe` | | | | ● | ● | | (●) | **3** |
| Inline docs / `explain` builtin | ● | ● | | | | | | **2** |
| Embedder-supplied builtins registry | ● | | | | | | (●) | **2** |
| Secrets as tainted type | | | | | ● | | ● | **2** |
| Test / assert primitives | | ● | | | | ● | ● | **3** |
| Telemetry / metrics handler | ● | | ● | | | | ● | **3** |
| Checkpoint / resume | | | | | ● | | ● | **2** |
| Redirection interceptor hooks | ● | | ● | | | | | **2** |
| Strict mode (`euo pipefail`) default | | ● | | ● | | | ● | **3** |
| Networking stack (http/tls/listen) | | | | | | | ● | **1** |
| String builtins (regex/split/join/…) | | ● | | | | | ● | **2** |
| UI primitives (prompt/confirm/table) | | | | | | | ● | **1** |
| Package manager | | ● | | | | | ● | **2** |

(●) = endorsed/implied rather than a primary ask.

---

## Tier 1 — Build first (high consensus × low effort on bashy seams)

These are the catalogue's "recommended first agentic batch" reinforced by where
the six external wishlists actually cluster. All are additive and degrade to
plain POSIX behavior.

### T1.1 — Structured output for builtins (`--json` everywhere)
- **Σ 4** (CAT-2, CL1, CL2-5, OC-1). The single most-requested *cheap* win:
  agents repeatedly fail parsing the shell's own output (`jobs`, `declare -p`,
  `trap -p`, `type`, `set -o`, `shopt -p`, `kill -l`, `times`, `caller`).
- **Surface.** `--json` flag on every informational builtin; global
  `BASHY_OUTPUT_JSON=1`; companion `read --json` / `mapfile --json` for ingestion.
  Stable, versioned field names decoupled from human format.
- **Bashy seam.** `interp/builtin.go` — add a `jsonOut(any)` helper; each flag
  parser learns `--json`; dump existing internal tables (e.g. `bashOptsTable`)
  directly. Document shapes in `docs/json-output.md` (file already exists).
- **Effort/Risk.** S per builtin, none. Do the whole set in one batch for schema
  consistency.

### T1.2 — `runner-state` / `snapshot` introspection builtin
- **Σ 4** (CAT-3, CX-6, CX-9, partially ANT-3/CL2). Lets an agent ask "why isn't
  `$FOO` what I expected" without an out-of-band socket.
- **Surface.** `runner-state {vars,traps,fds,opts,callstack,all}` emitting JSON
  by default, `--text` fallback. Plus a `snapshot`/`restore` pair (CX-9) for
  selected state categories (env, functions, aliases, traps, options, cwd,
  umask, job table), with hashes for large values.
- **Bashy seam.** New `interp/state_dump.go` serializing from `Runner`; dispatch
  case in `builtin.go`; add to `IsBuiltin`. Vet output for secrets.
- **Effort/Risk.** S, none (pure introspection).

### T1.3 — Audit / provenance hook + structured execution journal
- **Σ 5** (CAT-6, ANT-1, CL1-9, CX-12, OC-11). Foundation for replay (T2.3),
  metrics (T3.x), and capability enforcement. "What did the LLM-driven shell
  actually do?"
- **Surface.** `RunnerOption WithAuditHandler(func(AuditEvent))`;
  `BASHY_AUDIT_LOG=/path.jsonl` writes one JSONL line per command with
  **post-expansion argv**, cwd, env delta, pos (file/line/col), exit object,
  wall time, files read/written. This *is* the "machine-readable execution
  events on a dedicated fd" that ANT/CX/CL1 all lead with.
- **Bashy seam.** Single call site in `interp/runner.go` before simple-command
  dispatch and in `builtin.go` before the switch. Log writer on its own
  goroutine to avoid blocking.
- **Effort/Risk.** S, none. Highest design ROI per line.

### T1.4 — Deterministic / hermetic mode
- **Σ 4** (CAT-1, CL1-4, CL2-2, OC-11). Reproducible runs; the prerequisite for
  trustworthy replay and memoization.
- **Surface.** `set -o deterministic` + `BASHY_DETERMINISTIC=1` (optional seed
  `=42`). Stabilizes `$RANDOM`, `$SECONDS`, `$EPOCHSECONDS`, `$EPOCHREALTIME`,
  `$$`, `BASHPID`, `$!`, `mktemp` suffixes, `printf %()T -1`. Also pin `LC_ALL`,
  `TZ`, and **stable glob / associative-array iteration order** (CL2's
  `shopt -s stable_assoc`).
- **Bashy seam.** `Runner.deterministic` bool + `WithDeterministic`;
  `interp/vars.go:lookupVar` for the dynamic vars; `moreinterp/coreutils` mktemp.
- **Effort/Risk.** S–M, low (opt-in toggle). Watch tests keyed on dynamic PPID.

### T1.5 — Structured / position-aware errors + diagnostics
- **Σ 5** errors (CAT-9, AID, CL1-2, CX-2, OC-6) + **Σ 4** diagnostics
  (CAT-9, CL2-11, CX-8, OC-6). Merge into one schema.
- **Surface.** `RunnerOption WithStructuredErrors(func(ErrorEvent))` /
  `WithDiagnosticHandler`. `ErrorEvent`: `{kind, severity, message, pos,
  function, command, code, related[], stderr_tail}` in an **editor-compatible**
  shape (file/line/col/severity/code/related/fix-hint) so it interops with
  compiler/linter diagnostics. Carry a `category` (`TRANSIENT`/`PERMISSION`/
  `NOT_FOUND`/`USAGE`) and `retryable` flag (CL1's "honest exit semantics") so
  agents branch on category instead of memorizing exit-code tables.
- **Bashy seam.** Helper `r.report(ErrorEvent)` from `failf` and the parser;
  additive next to existing `errf` stderr path.
- **Effort/Risk.** S, none.

### T1.6 — First-class command result object
- **Σ 5** (CL1-2, CX-2, CL2-shared, OC-7, ANT-1). The non-lossy replacement for
  one-byte `$?`.
- **Surface.** Every command/compound yields `{kind, status, signal,
  core_dumped, duration_ms, rusage, cwd, argv, env_delta, source_pos,
  pipe_status[]}`. Per-pipeline-stage status (`PIPESTATUS` already hints) with
  the failing stage's stderr attached. Native predicates
  (`failed_due_to signal SIGINT`, `failed_due_to missing_executable`).
- **Bashy seam.** Travels through `interp/runner.go` command dispatch; surfaced
  via the T1.3 audit event and a `$BASHY_RESULT` JSON var. Reuses T1.5 schema.
- **Effort/Risk.** M, low. Pairs tightly with T1.3/T1.5; build together.

---

## Tier 2 — High value, moderate effort

### T2.1 — Sandbox / capability `restrict` (the universal ask)
- **Σ 7 — the only feature all seven sources requested** (CAT-5, AID-16, ANT-2,
  CL1-3, CL2-shared/4, CX-4, OC-12). "Lock the shell to this repo tree."
- **Surface.** `restrict --read . --write .,/tmp --net off --exec git,go,make { … }`
  block, plus `RunnerOption WithSandboxRoots(read,write)` and env triggers
  `BASHY_SANDBOX_READ` / `BASHY_SANDBOX_WRITE`. Reads outside read-roots →
  ENOENT; writes outside write-roots → EACCES; execs outside the exec allowlist
  → "command not found". Denials surface as **structured failures** (T1.5), not
  raw EPERM. Per-block escalation hook so the agent can pause and request
  permission with a reason.
- **Bashy seam.** Wrap existing `OpenHandler`/`StatHandler`/`ReadDirHandler`
  middleware (well-covered by `_test.go`); wrap `ExecHandler` to deny
  out-of-allowlist absolute execs. Audit builtins that bypass these handlers.
- **Effort/Risk.** M, low — the middleware pattern is already well-trodden.
  Note: bashy enforces at the interpreter boundary; true *external-process*
  containment (seccomp/landlock/`sandbox_init`) is an OS concern bashy can wrap
  but not fully implement (call this out honestly per OC's feasibility note).

### T2.2 — Capability / requirements declaration
- **Σ 4** (CAT-8, CL1-10-ish, CX-11, OC-12). Static "this script needs net +
  fs-write" review before running; pairs with T2.1.
- **Surface.** Magic preamble comment `# bashy: requires net,fs-write,exec`, a
  `require fs-write` builtin (defensive against truncated preambles), and CX's
  richer block:
  ```bash
  requires { command go >= 1.24; command git; write ./dist; network off }
  ```
  `bashy --check-requirements script.sh` validates without running. Mismatch
  with `WithCapabilities(set)` → fatal before any execution.
- **Bashy seam.** Lex preamble in `cmd/bashy/main.go`; `grantedCaps`/
  `requestedCaps` in `interp/api.go`; `require` builtin in `builtin.go`.
  Vocabulary: `net, fs-read, fs-write, exec, env-write, proc-create`.
- **Effort/Risk.** S–M, low.

### T2.3 — Record / replay
- **Σ 4** (CAT-10, CL1-9, CL2-9, CX-12). Debug "the LLM did something weird"
  without re-running the whole flow.
- **Surface.** `BASHY_RECORD=/path.jsonl` / `--record`; `bashy --replay file
  [--strict|--lax]`. Replay re-feeds recorded stdin, captures outputs, diffs.
  Redaction of secrets (T3.4) before persisting.
- **Bashy seam.** Built on T1.3 (audit hook writes the journal) + T1.4
  (determinism makes diffs tight) + a tee layer on stdin/stdout/stderr.
- **Effort/Risk.** M, medium (strict-vs-lax knob is delicate around
  timestamps/PIDs — which is why it sits on T1.4).

### T2.4 — Dry-run / plan / `--explain` with provenance
- **Σ 5** (CAT-7, CL1-3, CL2-7, CX-3, OC-17). Preview the resolved plan before
  committing; catch quoting bugs before they bite.
- **Surface.** `bashy --dry-run` / `--plan` and a `plan { … }` scope: walk
  execution, perform all expansions, substitute no-op effects for spawns/writes/
  network. Emit a structured plan: ordered resolved commands, final argv,
  inferred reads/writes, unresolved dynamic branches. `command --explain foo`
  reports the resolution chain (alias/function/builtin/hashed/PATH). `--explain`
  annotates each argv element with **provenance** (literal/var/cmd-sub/glob/
  brace/arith).
- **Bashy seam.** Flag in Runner; leaf-command entry prints `[+ would-run] …`
  and skips execution; honestly flag `$(...)` / `${var:=default}` side effects
  that must still evaluate. `--explain` reuses `typeMatches` in `builtin.go`.
  Provenance needs origin metadata threaded through the expansion path.
- **Effort/Risk.** M (`--explain` alone is S), medium — honest dry-run is hard;
  document the `$(...)` semantics explicitly.

### T2.5 — Structured cancellation & scope-tied cleanup
- **Σ 5** (CAT-12, ANT-6, CL1, CX-7, OC-3). "Stop, I changed my mind" must kill
  the whole process tree with no orphans.
- **Surface.** Document that `Runner.Run(ctx)` cancellation cleanly terminates
  everything including bg jobs. `WithCancelHook(func())` for last-rites cleanup.
  Scope-bound process groups per compound command (ANT's `async-group { … }`,
  CX's phased protocol: interrupt → terminate → kill → cleanup → final event).
  Built-in `timeout 30s { … }` block with a structured timeout result.
- **Bashy seam.** Audit every loop in `interp/runner.go` for `ctx.Done()`;
  verify bg-goroutine kill propagation; `CancelHook` from `Run` defer.
- **Effort/Risk.** M, low (mostly auditing).

### T2.6 — Per-runner resource limits
- **Σ 3** (CAT-4, CL1-6-ish, OC-12). Self-cap before the OS has to; graceful
  "give me up to N seconds / M bytes."
- **Surface.** `WithMaxWallTime`, `WithMaxCPUTime`, `WithMaxOutputBytes` (per
  stream), `WithMaxChildProcs`, `WithMaxOpenFiles`; env `BASHY_MAX_WALL=30s`,
  `BASHY_MAX_OUTPUT=1MB`; `limits` builtin to inspect. (Memory limit is best
  effort / OS-dependent.)
- **Bashy seam.** `Runner.limits` struct; `context.WithTimeout` in `Run`;
  child-count check in `ExecHandler`; byte-counting stdout/stderr wrappers
  returning code 137 at the cap; periodic `syscall.Getrusage` sampling.
- **Effort/Risk.** M, low — wrap-stdio needs care; feature-flag it.

### T2.7 — Bounded output / streaming budgets
- **Σ 2** (CL1-6, OC-7) but high pain — a 200k-line dump blows the agent's
  context window.
- **Surface.** `cmd --max-output=4kb --on-overflow=summarize` — shell keeps
  head+tail+counts with **semantic, labeled** truncation (not a silent `head`).
  Structured capture: `result=$(cmd)` yields `{stdout, stderr, code}`. Streaming
  line-buffered consumption with backpressure.
- **Bashy seam.** Builds on the byte-counting stream wrappers from T2.6 plus the
  T1.6 result object.
- **Effort/Risk.** M, low–medium.

---

## Tier 3 — Strong individual asks (build once Tier 1–2 plumbing exists)

### T3.1 — Native structured data types & typed pipelines
- **Σ 6** (AID-1/6, ANT-4, CL1-8, CL2-3, CX-3, OC-1/2/14). Universally wanted,
  but the **biggest core lift** — touches the value model and expansion grammar.
- **Surface.** First-class nested lists/maps/records with value semantics and no
  word-splitting; JSON/YAML/TOML in and out as builtins (`json`, `yaml`, `toml`);
  path accessors (`${data.stats[0]}`); typed scalars (`-i` exists; add float/
  bool/datetime); `null`/`undefined` sentinels; typed pipes (`|:` assert
  content-type, `|>` structured transform) that degrade to bytes for legacy
  tools.
- **Bashy seam.** Extend bashy's variable value model with a `json`/`object`
  attribute; new expansion path operators; new `json` builtin first (cheapest,
  unblocks the 80% case) before full typed pipelines.
- **Effort/Risk.** L, medium. **Recommendation:** ship a `json` builtin +
  `read --json`/`mapfile --json` (already in T1.1) as the pragmatic 80% slice;
  defer nested-value-model + typed-pipe grammar to a dedicated project.

### T3.2 — try/catch/finally + result/option types
- **Σ 6** (AID-2, ANT-8, CL1-2, CL2-endorsed, CX-endorsed, OC-6). Replace the
  `set -e` / `trap ERR` / `|| { … }` patchwork.
- **Surface.** Parser-level `try { … } catch err { … } finally { … }` with
  structured `${err.line}` / `${err.message}` / `${err.stack}` (ties to T1.5/
  T1.6). Optional `Result<T,E>`/`Option<T>` returns; error-propagation operator
  `cmd?`.
- **Bashy seam.** New grammar in bashy's parser + a `cm_try_catch`-equivalent
  command node + execution logic that populates the error object and jumps to
  catch. Reuses T1.6 result object as the catch payload.
- **Effort/Risk.** L, medium — new grammar. High consensus justifies it after
  the result-object plumbing (T1.6) lands.

### T3.3 — Real stack traces with source spans
- **Σ 5** (AID-2, CL1, CL2-8, CX, OC-6). Compilers give traces; the shell should
  too.
- **Surface.** On any error (and on-demand via builtin): backtrace of
  `{function, file, line, col-span, resolved-command}` per frame, surviving
  `source`/function/subshell/command-sub boundaries; same diagnostic schema as
  T1.5 so editors consume it.
- **Bashy seam.** `BASH_SOURCE`/`BASH_LINENO`/`FUNCNAME` + `caller` already exist;
  add column spans by carrying token positions from the parser into command
  nodes and out through the error path.
- **Effort/Risk.** M, low–medium. Natural extension of T1.5.

### T3.4 — Secrets as a first-class tainted type
- **Σ 2** (CL2-4, OC-12) but uniquely agent-relevant (exfiltration risk).
- **Surface.** `declare -s TOKEN=…` marks a value tainted; taint **propagates**
  through expansion/command-sub; tainted values redact to `‹secret:sha256:…›` in
  `set -x`, `declare -p`, errors, telemetry, journals — but pass intact to child
  env. A write-guard **refuses** writing a secret to a non-tty file / network fd
  / terminal without `--reveal`. `with-secret VAR from <vault-cmd> { … }` scope.
- **Bashy seam.** Attribute bit on the shell var; propagate in assignment/
  expansion; redaction hook in the xtrace printer and `declare -p`; write-guard
  in the redirection open path. Folds into the T1.3 audit redaction.
- **Effort/Risk.** M, low. Strong safety story; small surface.

### T3.5 — Content-addressed memoization / cache
- **Σ 3** (CL1-4, CL2-1, OC-11). Eliminates the largest category of wasted
  agent wall-clock (re-running unchanged `build`/`test`/`lint`).
- **Surface.** `memoize --in 'src/**/*.go go.mod' --out 'bin/app' --env 'CGO_ENABLED'
  { go build … }` fingerprints argv + input-file contents + env-subset + tool
  version; on a hit, replay cached stdout/stderr/exit/declared outputs.
  `--cache=off|read|write|verify` (verify = flake detector). Also OC's lighter
  `cache --ttl 5m --key "url" { … }`.
- **Bashy seam.** Wrap command dispatch to compute fingerprint before exec;
  resolve input globs via existing pathexp; tee outputs via the redirection
  layer. Depends on T1.4 (determinism) to be sound.
- **Effort/Risk.** M–L, medium (soundness of the input/output declaration).

### T3.6 — Named persistent tasks + readiness predicates
- **Σ 2** (CL1-5, OC-3) but addresses the single biggest wasted-wall-clock
  source after memoization: `sleep 5 && curl` polling.
- **Surface.**
  ```bash
  task start web -- npm run dev          # detached, survives the session
  task wait-for web --until='Listening on' --timeout=30s
  task logs web --since=last-check --grep=ERROR
  task status web                        # {running, pid, uptime, last_exit}
  ```
  Readiness predicates ("until port open / log line appears / healthcheck
  passes") as a primitive. Ring-buffered, cursor-queryable per-task logs.
- **Bashy seam.** A supervisor subsystem decoupled from the Runner lifetime;
  reuses the T1.3 event stream for log capture. Larger than job-control tweaks.
- **Effort/Risk.** L, medium.

### T3.7 — Bounded parallelism / spawn-await
- **Σ 3** (AID-4, CL2-6, OC-3). Agents fan out lint/test/fetch; the safe bash
  pattern (bg jobs + semaphore + `wait -n`) is fiddly enough that agents
  serialize work they could parallelize.
- **Surface.** `parallel --jobs N { … }` / `for x in … parallel(N)`; deterministic
  **ordered** result collection tagged with item + exit; fail policy
  (`--fail-fast` vs `--keep-going` with aggregated structured summary);
  global concurrency budget so nested blocks don't fork-bomb. `spawn`/`await`
  keywords as the explicit form.
- **Bashy seam.** Builds on job control + per-iteration output buffering keyed by
  index. Pairs with T2.5 cancellation for `--fail-fast`.
- **Effort/Risk.** M–L, medium.

### T3.8 — Filesystem transactions / rollback
- **Σ 3** (CL1-7, CX-5, OC-11). Step 3 of 5 fails → no half-mutated tree.
- **Surface.** `transaction { patch a.c; patch b.c; ./build || abort }` —
  stage writes (overlay/CoW snapshot), commit on success, auto-rollback on
  failure/signal/`abort`. Structured write log mapping fs changes back to the
  responsible command ID. Undo journaling for destructive builtins.
- **Bashy seam.** Shell-redirection writes observable in the redirection layer;
  **external-process writes need OS support** (snapshots/interposition) — scope
  honestly to shell-mediated writes first, flag external as best-effort.
- **Effort/Risk.** L, medium–high (external writes are the hard part).

### T3.9 — Self-describing commands + invocation validation
- **Σ 3** (CL1-10, CL2-10, OC-17-ish). "What are your exact flags and types?"
  so the agent constructs a correct invocation on the first try.
- **Surface.** `cmd --describe` convention returning a typed schema (subcommands,
  flags, value types, required/optional, mutually-exclusive groups, side-effect
  declarations). Shell-side **pre-fork validation** of constructed argv against
  the schema → structured diagnostic before spending a process. Typed completion
  data as a byproduct.
- **Bashy seam.** Extend the completion subsystem toward typed descriptors;
  pre-fork validation in the command-resolution path before exec.
- **Effort/Risk.** M–L, low–medium. Self-describe bashy's own builtins first.

### T3.10 — Out-of-band state inspection socket
- **Σ 4** (ANT-3, CX-6, CL2-shared, CAT-3-adjacent). Query shell state while a
  command blocks stdin, without polluting history.
- **Surface.** A local control socket / fd for read-only queries: pwd, vars,
  functions, aliases, traps, jobs, options, current AST node. Snapshots clearly
  marked mutable/stale.
- **Bashy seam.** A read-only server over the same serializers as T1.2
  (`runner-state`). For many embedders the in-process T1.2 builtin already
  covers this; the socket matters mainly for *external* supervisors.
- **Effort/Risk.** M, low–medium. **Recommendation:** ship T1.2 first; add the
  socket only if an external-supervisor use case is real.

---

## Tier 4 — Ecosystem / language modernization (large, lower agent-specific ROI)

These appear mostly in AID and OC (the modernization-lens sources). They're real,
but they're closer to "redesign bash the language" than "make bash observable to
an agent," so they rank below the Tier 1–3 substrate work.

- **T4.1 Module / namespace system** (Σ3: AID-8, ANT-5, OC-5) — `import "./x.sh"
  as utils`; per-module hash table; `BASH_MODULE_PATH`; versioned imports.
  Pairs with lexical scoping. Effort L.
- **T4.2 Lexical scope / `local` by default** (Σ3: ANT-5, CL1-8, OC-15) — change
  variable-context lookup to honor lexical definition chain; high blast radius on
  compat, so gate behind a mode. Effort M–L, **risk high** (compat).
- **T4.3 Test / assert primitives** (Σ3: AID-9, CX-10, OC-6/10) — `assert`,
  `assert_eq`, `assert_file_exists`, `assert_json_eq`, `assert_status`; TAP/JUnit/
  JSON output; `test_*` discovery. Effort M, low risk (new builtins).
- **T4.4 Inline docs / `explain` builtin** (Σ2: CAT-11, AID-14) — `bashy explain
  mapfile` with bashy's actual honored-flag truth table via `//go:embed`. Effort
  S; value is in the content.
- **T4.5 Embedder-supplied builtins registry** (Σ2: CAT-13, OC-17) —
  `WithExtraBuiltins(map[string]BuiltinFunc)`; per-Runner map checked before the
  unsupported-hint fallthrough. Unblocks exposing MCP/tool calls as builtins.
  Effort S, low. (Once T1.3 lands these get audited for free.)
- **T4.6 Telemetry / metrics handler** (Σ3: CAT-14, ANT-1, OC-10) —
  `WithMetricsHandler(func(Metric))`; one metric per builtin/exec (name,
  duration, byte counts, exit). Effort S; rides the T1.3 dispatch wrap.
- **T4.7 Checkpoint / resume** (Σ2: CL2-9, OC-11) — `checkpoint` snapshots
  loop/iterator progress + declared state; `bash --resume <journal>` skips done
  iterations. Pairs with T3.5 memoize + T3.7 parallel. Effort L.
- **T4.8 Redirection interceptor hooks** (Σ2: CAT-7-adjacent/ANT-7) —
  `trap '…' REDIRECT_WRITE`; hook the redirection open path to inspect/reroute/
  mock targets. Effort M.
- **T4.9 Strict mode as default** (Σ3: AID-10, CL1-2, OC-15) — a `--agent`
  invocation mode that turns on `errexit`/`nounset`/`pipefail` plus
  **refuse-empty-expansion** (`rm -rf $X` with empty `$X` errors — CL1's single
  biggest blast-radius reduction). Effort S–M, low (opt-in mode).
- **T4.10 String/text builtins** (Σ2: AID-5, OC-9) — `regex`, `split`, `join`,
  `replace`, `trim`, grapheme-aware `len`/`substr`/`upper`/`lower`. Reduces
  subprocess count. Effort M (incremental).
- **T4.11 Networking stack** (Σ1: OC-4) — `http`, `https`, `listen`/`accept`,
  websockets, unix sockets, retry/backoff. Large; lowest agent-substrate ROI of
  the set. Effort L.
- **T4.12 UI primitives** (Σ1: OC-13) — `prompt`, `confirm`, `select`, `table`,
  `notify`. Useful for human-in-the-loop; not core to autonomous execution.
- **T4.13 Package manager** (Σ2: AID-13, OC-5) — registry + `bash install`.
  Ecosystem play, out of scope for the interpreter. Effort L.
- **T4.14 Agent-specific builtins** (Σ1: OC-17) — `llm`, `tool`, `schema`,
  `embed`, `retry`, `human_in_the_loop`. Best delivered via T4.5 (embedder
  registry) rather than baked into the core.

---

## Recommended implementation batches

**Batch A — "make the shell observable" (1–2 sessions, zero compat risk):**
T1.3 audit hook → T1.1 `--json` builtins → T1.2 `runner-state` → T1.5 structured
errors/diagnostics → T1.6 result object. Everything additive; produces a
meaningfully agent-friendly shell without touching bash compatibility. This is
the catalogue's "recommended first agentic batch," confirmed by consensus.

**Batch B — "make it reproducible & safe":** T1.4 deterministic mode → T2.1
sandbox/`restrict` (the unanimous ask) → T2.2 capability declarations → T2.3
record/replay (rides A+T1.4) → T3.4 secrets. Sandbox is the one feature all seven
sources requested; do it as soon as the handler-wrapping pattern from Batch A is
warm.

**Batch C — "make it safe to preview & bounded":** T2.4 dry-run/plan/`--explain`
→ T2.5 structured cancellation → T2.6 resource limits → T2.7 bounded output.

**Batch D — "the expensive, high-consensus core changes":** T3.1 structured data
(ship the `json` builtin slice first) → T3.2 try/catch → T3.3 stack traces →
T3.5 memoization → T3.7 parallelism. These need new grammar or value-model work;
sequence them after the cheap observability layer proves the schemas.

**Defer / opportunistic:** Tier 4 ecosystem items, picked up as specific user
demand appears. T4.5 (embedder builtins) is the cheapest high-leverage one and
can jump ahead whenever an embedder needs to expose a tool as a builtin.

---

## Design invariants (shared across every source)

1. **Backward compatible or dead on arrival.** Every feature degrades to plain
   POSIX/bash behavior; new powers are opt-in lanes, modes, builtins, and flags.
2. **Dual-audience output.** Human prose and machine-structured data must never
   compete for the same byte stream — hence a *separate* data lane / fd, not
   reformatted stdout.
3. **Errors loud and typed; success quiet.** The agent's worst outcome is
   confident wrongness from a failure that didn't surface.
4. **Safe by construction, not by vigilance.** Capability scoping, dry-run,
   refuse-empty-expansion, and secret taint move safety from the agent's
   fallible judgment into the substrate.
5. **One consistent schema.** Audit events, errors, diagnostics, result objects,
   and `--json` output should share field names and versioning so an agent learns
   one shape, not ten.
