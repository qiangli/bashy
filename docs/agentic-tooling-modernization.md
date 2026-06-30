# Agentic tooling modernization

A survey of GNU coreutils (and the classic-Unix toolset around it) through the
lens of an **agentic shell**: which past-century tools should be replaced,
shadowed, augmented, or left alone — and the default-behavior policy that decides
*when* bashy quietly does the better thing vs. merely nudges toward it.

Companion to `docs/space-time-advisor.md` (the advisor is the *reactive* half of
the same "nudge" subsystem proposed here).

## Prime invariant — help, don't obstruct

> **bashy features must help other agentic tools succeed, never stand in their
> way.**

bashy is **additive and optional**. It never silently changes the *results* or
*output format* an agent would get from a standard tool; it offers better paths
(new verbs, hints, opt-in flags) and otherwise gets out of the way. The agent's
harness — not bashy — owns the contract. Every recommendation below is checked
against this invariant:

- **Allowed to be default/automatic:** changes that are *invisible and
  result-preserving* — e.g. running a search in parallel (same matches, faster),
  or emitting a hint on stderr (data stream untouched).
- **Must be opt-in + hinted (never a silent default):** anything that changes the
  *result set* (gitignore filtering hides files) or the *output format* (`--json`
  instead of text). Offer it via a flag/env and point at it with a hint; let the
  agent choose.
- **Never changed:** the behavior of state-mutating tools.

This is the lens *behind* the lens: the four failure modes below say what's worth
improving; this invariant says improvement must never come at the cost of
surprising the agent.

## The lens — what earns a revamp

A classic tool earns a revamp only if it costs an agent one of four things.
Tools that hit none of these are **fine as-is**; modernizing them is wasted work.

1. **Wasted tokens** — dumps noise (`grep -r` through `node_modules`, `ls -R` of
   a repo) or unstructured walls of text the model must re-read.
2. **Wrong state** — mutates the shell so the *next* command runs in the wrong
   place (`cd`/`pushd`). This is exactly the advisor's `cwd` failure dimension.
3. **Brittle parsing** — emits columnar human text an agent must regex-scrape
   (`ls -l`, `df`, `stat`, `du`).
4. **Textual when structural is correct** — `grep`/`sed` over *code*, where the
   right unit is a symbol/AST node, not a line.

Precedent: `make → bashy dag` is already this pattern, shipped.

## Default-behavior policy (the rule of thumb)

Two regimes, by whether the tool mutates system state. The unifying mechanism is
one **nudge subsystem** (see below), not per-tool bespoke code.

### A. Read-only / view tools → split by visibility (per the prime invariant)

`find`, `ls`, `grep`, `cat`, `du`, `df`, `stat`, `wc`, `tree`, …

The earlier "agentic-by-default" idea is **deliberately narrowed** by the prime
invariant: a default may flip *only* when it doesn't change what the agent gets.

- **Invisible, result-preserving wins → safe to default on** (even silently):
  parallel traversal, faster engines, smart-case that's still a superset. Same
  results, same format, just better. These never stand in the way.
- **Result-changing behavior → opt-in + hinted, never silent default:**
  gitignore/noise filtering *hides* paths, so it ships as `--agentic` / `rg`-style
  verbs / `BASHY_AGENTIC=1`, and a one-time hint points at it. An agent searching
  for something in a generated/ignored file must not be silently starved of it.
- **Format-changing behavior → opt-in, never silent default:** `--json` etc.
  change the byte stream a downstream consumer parses; offer via flag and hint,
  or auto-enable only when the agent has explicitly signalled it wants JSON.
- **Outside agent mode** (interactive humans, scripts): classic GNU defaults,
  hint only — protects scripts and the bash-5.3 conformance contract.
- **Transparency rule (when an opt-in *is* active):** if filtering changed the
  result, announce the delta on stderr — "3 paths hidden by .gitignore;
  `--agentic=false` to include." Even opted-in filtering must not be silent.
- **Overrides:** `--agentic=false` / `--classic` per call; `BASHY_AGENTIC=0`
  global.

### B. Mutating tools → behavior-preserving, hint-only

`cd`, `pushd`/`popd`, `rm`, `mv`, `cp`, `chmod`, `truncate`, …

- **Never change what they do.** Anything that alters shell or filesystem state
  keeps its exact semantics.
- **Emit a non-intrusive hint** on stderr pointing at the safer agentic
  alternative (`cd` → `awd`), including the off-switch — once.

### Cross-cutting rules (both regimes)

- **Channel = stderr, never stdout.** Appending to stdout (even with a marker)
  corrupts pipelines (`find | xargs`, `ls | wc`) and `--json` consumers. stdout
  stays pure data. (The advisor already uses stderr for the same reason.)
- **Rate-limit: once per (tool, session).** A hint on every `cd` becomes noise the
  model learns to ignore. First use teaches + shows the off-switch; then silent.
  Reuse the advisor's session memory.
- **One marker, two renderings:**
  - agent mode → one JSON line:
    `{"schema_version":"bashy-hint-v1","kind":"hint","tool":"cd","suggest":"…","off":"BASHY_AGENTIC=0"}`
  - human mode → a delimited block: `─── bashy hint ─── … (silence: BASHY_HINTS=off)`
- **Off-switch family** (reuse, don't proliferate):
  - `BASHY_AGENTIC=0` — master: classic defaults + no hints.
  - `BASHY_HINTS=off` — keep agentic defaults, silence the nudges.
  - `--agentic=false` / `--classic` — per-invocation classic behavior.
  - `BASHY_ADVISOR` stays for the reactive advisor; long-term these fold under
    one umbrella.

### The architectural point

The **advisor** (reactive — fires on a *failure*) and **tool-hints** (proactive —
fire on *use of a legacy tool*) are the same mechanism at different moments.
Build them as one nudge subsystem: shared stderr emission + rate-limiter +
session memory + agent/human rendering. Two overlapping systems would drift.

## Example #1 — `cd; cmd` / `pushd…popd` → an `awd` builtin

Highest-value, lowest-cost item; pairs directly with the advisor's `cwd` dimension
(advisor = cure after the fact; `awd` = prevention).

- `coreutils env` today has no `-C` and refuses to run a COMMAND, so `env -C dir
  cmd` isn't available.
- `pushd/popd/cd` mutate `r.Dir` — the exact source of the "wrong directory" loop.
- `(cd dir && cmd)` works but agents routinely drop the subshell parens and leak.

**Recommendation:** `awd DIR -- cmd args…` (alter-working-directory) as a **sh
builtin** — runs the command with `r.Dir=DIR` for its duration, then restores. A
builtin beats `env -C` because it wraps builtins, functions, and pipelines, not
just external execs. Also add `-C` to `coreutils env` for GNU-script compat.
Symmetry: `pwd` reports cwd; `awd` overrides it for one command.

## Example #2 — `find`/`grep` → faster + semantic

Today's `grep`/`find` are GNU-*compat* reimplementations (Go `regexp`, lexical
walk): correct but **not gitignore-aware, not parallel**, so recursive search
drags the agent through `.git/`, `node_modules/`, `vendor/`, `dist/` — a large
token tax.

- **(a) Fast textual** — a ripgrep-class searcher (gitignore-aware, parallel,
  smart-case, `--json` matches) and an `fd`-class finder. Biggest token saver for
  "where is X." New verbs (`rg`/`fd`) and/or agentic-default fast modes on
  `grep -r`/`find` per the policy above.
- **(b) Semantic/structural — already exists, underexposed.** `yc symbols` /
  `yc refs` / `yc repomap` (treesitter, 9 languages, PageRank, token budget) is the
  "resmap hint to agents." The work is **promotion + routing**: when an agent greps
  a symbol name, hint "use `yc refs <name>` — structural, ranked, budgeted." Add the
  missing third leg: an **`ast-grep`-class** structural search/replace (the
  structural answer to `sed`).

## Systematic pass (the 79 + gaps)

| Bucket | Tools | Action |
|---|---|---|
| Build — ephemeral cwd | `cd`/`pushd`/`popd` | `awd` builtin + `env -C` (Ex. #1) |
| Shadow — faster, gitignore-aware | `grep`, `find` | ripgrep/fd-class engine or agentic-default fast mode (Ex. #2a) |
| Promote/route — already exist | `yc symbols/refs/repomap` | default for code search; advisor/hint routing (Ex. #2b) |
| Augment with `--json` | `ls`, `stat`, `df`, `du`, `find`, `wc`, `env`/`printenv` | structured output; reuse the `loom-v2` envelope so agent mode is uniform |
| Revamp for code-reading agents | `cat`, `diff` | `cat`: line numbers (+opt syntax) = bat-class — agents need line refs to edit. `diff`: structural/`difftastic`-class + `--json` hunks |
| Fill gaps (missing today) | `sed`, `xargs`, `tree`, `watch` | `sed`→ prefer `sd`+`ast-grep` for agents; `xargs`→ parallel-capable or `fd -x`; `tree`→ budgeted/`--json`; `watch`→ diff-on-change |
| Keep as-is (no agentic gain) | `true/false`, `echo`, `seq`, `sleep`, `tee`, `sync`, `tr`, `comm`, `join`, `paste`, `tac`, `split`, `shuf`, `cmp`, `tsort`, `sha*sum`, `base32/64`, `basename`, `dirname`, `link/unlink`, `mkdir/rmdir/ln/touch`, `chmod/chown/chgrp`, `readlink/realpath`, `truncate`, `id/whoami/hostname/uname/tty/uptime/which/strings` | leave them |

## Beyond coreutils (same lens, adjacent tools)

- `ps`/`top` → structured process list (`--json`); `tail -f`/`less` → bounded
  agent log-follow (diff-on-change).
- `man` → `tldr`-class (terse, example-first — fewer tokens).
- `du`/`df` → visual/`--json` (dust/duf-class).
- `curl`/`wget` → `httpie`-class with `--json` (ycode already has
  `yc browser fetch` — promote).
- `ifconfig`/`netstat` → `ip`/`ss` (and they'd feed the advisor's network
  dimension).

## Shipped

- **`awd` builtin** (sh, gated) — ephemeral alter-working-directory.
- **Nudge subsystem** (bashy) — reactive advisor + proactive `cd`/`pushd`→`awd`
  hints, shared session memory, rate-limited, `BASHY_AGENTIC`/`BASHY_HINTS` off-switches.
- **Opt-in `--agentic` search** (coreutils `grep`/`find`) — `.gitignore` + noise-dir
  filter via `pkg/ignore`, stderr transparency line, byte-identical without the flag;
  plus a `grep -r`/`find` → `--agentic`/`yc refs`/`yc repomap` routing nudge.
- **`time`** (coreutils `cmds/time`) — pure-Go GNU `/usr/bin/time` drop-in
  (default/`-p`/`-v`/`-f`/`-o`/`-a`), coexists with the bash `time` keyword (reach
  via `command time`/`\time`); agentic `--budget DUR --todo TEXT` surfaces a TODO
  (JSON in agent mode) when a step overruns. Conductor self-dependency.
- **`sed`** (coreutils `cmds/sed`) — GNU sed drop-in. Engine vendored from
  rwtodd/Go.Sed (MIT, `internal/gosed`) — full command set (s/y/d/p/n/N/D/P/
  hold-space/branching/a/i/c/ranges) — adapted for GNU semantics: patterns via
  `pkg/bre` (BRE default, ERE under `-E/-r` — same translator as grep), `s///`
  with GNU `\1`/`&` replacements + `i`/`m` flags, `-n/-e/-f/-i[SUFFIX]/-s`.
  Pattern back-refs / `\<\>` fail loudly (RE2 can't express them). The BRE
  translator was extracted from grep into shared `pkg/bre`.
- **`xargs`** (coreutils `cmds/xargs`) — GNU-subset xargs (structure credited to
  u-root, BSD-3). `-0`, `-n`, `-I` (replace-str), `-P` (parallel), `-r`, `-E`
  (eof-str), `-d` (delimiter), `-t`; GNU default quote/backslash word splitting;
  child stdin = null device; GNU exit codes (123/124/125/126/127). `-p` (needs a
  tty) fails loudly. Parallel output is flushed atomically per-invocation.
- **`bashy schedule`** (coreutils `pkg/schedule`) — modern cron (`--cron` via
  robfig/cron, `--every`, `--at`) with a JSON store + `daemon`/`tick`; agentic
  `--prompt`/`--context` delivered to the fired command as `BASHY_SCHEDULE_*`, so
  a conductor self-wakes a long-running campaign. Host `cron`/`crontab` untouched.

- **`tree`** (coreutils `cmds/tree`) — recursive box-drawing listing + dir/file
  summary. Defaults match classic tree (hides dot-files); `-a` shows all, `-L N`
  limits depth, `-d` dirs-only. Opt-in `--agentic` skips `.gitignore`d + noise
  paths via `pkg/ignore` (+ a transparency line) for repo orientation without
  the dependency-tree noise. (`view` was dropped — `cat -n` already numbers lines.)

- **`yc query`** (coreutils `cmds/yc`) — **structural search** via tree-sitter
  queries (S-expression patterns with `@captures`), the ast-grep-class addition.
  `yc query --lang go '(function_declaration name: (identifier) @fn)' [path]`
  matches the AST (not text) across the 9 treesitter languages, pure-Go, reusing
  `pkg/treesitter` + the binding's Query API. Grammar-specific (`--lang` required
  for a dir, inferred for a single file); invalid queries fail loudly. We expose
  tree-sitter's query language (which ast-grep compiles down to, and which LLMs
  write fluently) rather than reimplementing ast-grep's `foo($A)` pattern-compiler
  (a large project that would risk silent mis-matches).

## Decided against: per-tool `--json`

Per-tool `--json` (ls/stat/df/du/wc) was **dropped**: agents parse plain text
fine in-context (the brittle-parsing case — programmatic `awk`/`cut` pipelines —
is the rarer path), and JSON output is *more* tokens per datum, not fewer. The
real byte-savers are noise-filtering (`--agentic`, shipped) and budgeting
(repomap), not the format. Replaced by one generic mechanism:

- **`bashy run`** (bashy `internal/agentos/run.go`) — wrap any command and emit a
  `bashy-run-v1` envelope bundling the result with bashy's agentic meta
  (non-lossy exit/signal, duration, cwd, and the space-time advisor's hints as
  structured data). Default **streams (tee)** — output goes live, a compact meta
  line trails on **stderr** (stdout stays pure/pipeable); **`--capture`** emits
  one stdout record embedding stdout/stderr (for logging/transport). Returns the
  command's own exit status. Reachable as the front-door `bashy run` or the bare
  `run` shim. The value is the *meta*, not reformatting output.

## Prioritized shortlist

1. **`awd` builtin** — smallest, highest daily value; closes the loop with the
   advisor's cwd dimension.
2. **The nudge subsystem** — shared stderr emission + rate-limit + agent/human
   rendering + off-switches. The substrate every other item plugs into.
3. **gitignore-aware fast `grep -r`/`find`** — parallel speed can default on
   (invisible win); the *filtering* ships opt-in (`--agentic`/`rg`-verb) + hinted,
   with the transparency rule. Biggest token saver, invariant-safe.
4. **`--json` for `ls`/`stat`/`find`/`df`/`du`** — opt-in flag (format change is
   never a silent default); kills brittle text-scraping. Mechanical, parallelizable.
5. **Promote `yc refs`/`repomap` as code-search default** + hint when an agent
   greps a symbol — it already exists.
6. **`cat` line-numbers-by-default in agent mode** + structural `diff --json`.
7. **`ast-grep`-class structural search/replace** — the real `sed` revamp; do last.

Through-line: items 2–6 are structured-output + noise-filtering — the same
"fewer wasted turns" axis the advisor optimizes; they compound.

## Open decisions

- Whether `awd` should also accept a trailing form (`awd DIR cmd…` without `--`)
  and whether to alias it to `@DIR cmd` sugar.
- Whether agentic-default read-only behavior should ever apply outside agent mode
  behind an explicit `BASHY_AGENTIC=1` (opt-in for power users).
- Final env-var consolidation: keep `BASHY_ADVISOR` separate or fold the whole
  nudge subsystem under `BASHY_AGENTIC` with sub-controls.
