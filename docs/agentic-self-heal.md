# Agentic self-heal: resolve the recoverable case, then report

**One principle.** When an agent's command hits a *recoverable* condition —
a wrong-dialect flag, a transient system error, an empty search — the shell
should **do the obvious next thing itself and tell the agent what it did**,
instead of returning an error or an empty result the agent must spend a *round
trip* to diagnose and retry. Every avoided round trip is wall-time, tool calls,
and tokens saved. This is the act-half of the same idea as the hint engine
(`pkg/nudge`): the hint *observes and suggests*; self-heal *acts and reports*.

This is a differentiator a traditional shell (zsh, bash-on-BSD) structurally
cannot offer: it needs the pure-Go userland as the portable truth, the
Command-Atlas effect classification to know what is safe to auto-act on, and the
agent-mode wire contract to report the adaptation back in the tool result.

## Hard invariants (every member obeys these)

1. **Read-only to auto-act.** Anything that *modifies* the command's effect
   (rewriting args, retrying, escalating) is allowed only on commands the atlas
   classifies read-only. A write is never silently re-run or rewritten — a
   partial mutation plus a retry is a double-apply. (`sed -i` is excluded from
   autofix; a failed `rm` is never auto-retried.)
2. **Transparent.** Every auto-action emits a note on the command's **captured
   stderr** (so it rides back to the agent in the *same* tool result), in the
   agent-mode JSON contract (`schema_version` + `kind` + what-was-done + `off`).
   Nothing is silently changed.
3. **Certain-intent only.** Act only when the fix is unambiguous. `autofix`
   rewrites **true aliases** (identical semantics, different spelling) only —
   never a change that could alter a result (`grep -P` PCRE ≠ `-E` ERE, so it is
   NOT a candidate). When in doubt, do nothing and let the original run.
4. **Tell the agent what was tried, so it doesn't redo it.** For escalating
   actions (retry, augmented search) the note enumerates every strategy already
   attempted — the agent must not re-issue a search the shell already exhausted.
5. **Silenceable + gated.** All members ride the `pkg/nudge.Enabled()` gate
   (agent-mode / `BASHY_HINTS`); a human interactive session and `--posix` see
   nothing. Never linked into the pure `cmd/bash` drop-in.

## Members

Each is an `interp.ExecHandler` (or audit) middleware in **coreutils**, so bashy
AND ycode (and any consumer of the in-process userland) share it. Wire order:
`telemetry → validate → autofix → autoretry → coreutils → fork`, with
augmented-search hooking the search verbs specifically.

| member | trigger | action | status |
|---|---|---|---|
| **nudge** (`pkg/nudge`) | legacy tool w/ better counterpart | ONE hint, no change | shipped |
| **advisor** (bashy) | command failed | explain the space-determined cause | shipped |
| **autofix** (`pkg/autofix`) | read-only wrong-dialect/platform flag | rewrite to local equivalent + note | **P0 shipped** |
| **autoretry** | read-only command fails on a transient error | retry w/ backoff, then report attempts | P1 |
| **augmented-search** | a search verb returns empty | escalate literal→stem→fuzzy→graph, report what was tried | P2 |

### autofix — P0 (shipped)

`coreutils/pkg/autofix`. A read-only command carrying a flag from another
shell/version/platform is rewritten to the local equivalent before it runs, and
the adaptation is noted on the command's stderr. First rule: GNU `sed -r`
(extended-regexp) → portable `sed -E` on non-Linux, guarded to never touch a
writing `sed -i`. The table (`rules` keyed by argv[0]) is the extension point;
every entry must be a true-alias, read-only rewrite. Wired into ycode's
interpreter chain after validation, before exec.

Demonstrated: an agent running `sed -r 's/o+/O/g' f` on macOS gets
`hellO wOrld` **plus** `{"kind":"autofix","note":"adapted GNU sed -r to the
portable sed -E …","ran":["sed","-E",…]}` — a result-with-note, not an error.

Candidate future rules (all must clear the true-alias bar): `egrep`→`grep -E`,
`fgrep`→`grep -F`, `tac`↔`tail -r`. NOT candidates: `grep -P`, `stat -c`↔`-f`
(format strings differ), `readlink -f` (no BSD equivalent) — these change
results and belong to a higher-confidence escalation, not a silent rewrite.

Bigger adjacent win (separate slice): **register the coreutils userland in
ycode's shell** so agent-emitted GNU flags work in-process on every platform
*without any rewrite* — bashy's coreutils is itself the portable truth. autofix
then only handles the residual (forked non-coreutils tools, genuine typos).

### autoretry — P1 (design)

For a **read-only** command that fails on a **transient** error (network blip,
`EAGAIN`, a lock, a briefly-unavailable resource — a bounded, recognizable set,
NOT a logic error), retry with capped exponential backoff, then either return
the eventual success or a note: `retried N times over Ns; still failing with
<class>` — so the agent doesn't burn a round trip re-issuing the same doomed
call, and doesn't retry something the shell already retried. Reuses the
advisor's error-classification (it already distinguishes transient vs terminal).
Idempotency guard: read-only only, and a per-command retry budget surfaced in
the note. Never linked into `cmd/bash`.

### augmented-search — P2 (design, owner: bashy)

When a search verb (`grep`, `ast search`, `kb search`, `graph query`) returns
**empty**, don't hand the agent nothing — escalate along a fixed ladder and
report the whole trail:

```
literal → case-insensitive → stemmed/tokenized → fuzzy (edit-distance)
        → structural (ast symbols/refs) → graph (neighbors/callers)
```

Stop at the first rung that yields results; emit a note listing **every rung
tried and its yield** (`literal:0 stem:0 fuzzy:2 → showing fuzzy`). The
load-bearing part is the *trail*: the agent is told exactly what has already
been exhausted so it does not re-run a literal grep the shell already ran and
widened. Ties into `pkg/nudge`'s existing routing toward `ast`/`graph` — the
hint says "try the structural verb", augmented-search *does* it and reports.
Read-only by nature. Bounds: each rung capped, fuzzy edit-distance small,
results deduped across rungs.

## Why this is bashy's to own

Compat is the floor, the superset is the ceiling, **the hint is the elevator**
(`docs/philosophy.md`). Self-heal is the elevator carrying the agent up
*automatically*: it turns "your flag is wrong / your search found nothing" from
a dead end into a completed step. It requires exactly the three things bashy has
and a raw shell does not — the pure-Go portable userland, the atlas effect
classification (what is safe to auto-act on), and the agent-mode wire contract
(how to report it) — so it is structural, not cosmetic.
