---
name: bashy
description: Drive bashy, the agentic shell — a drop-in Bash 5.3 with agent-native extensions. Use whenever bashy is the shell you are running in (or is on PATH) to read what this host ALREADY KNOWS about your task before starting it, see what other agents and humans are saying, know what work is owed here, cut probing round-trips, preview destructive commands, run with structured result envelopes, navigate code without grep dances, and use environment-gated verified skills. Open every session with `bashy context --json`, then the three durable stores — `bashy kb search "<your task>"` (what is known), `bashy mb` (what is being said), `bashy todo list` (what must be done) — and close it with `bashy kb retro`.
compatibility: requires the bashy binary (an agentic host shell); all verbs also work as `bashy <verb>` from any shell
---

# bashy — the agentic shell, in one page

bashy is two things at once: a **conformant Bash 5.3 drop-in** (your
scripts just run) and an **agentic shell** — a pure-Go userland and a set
of agent-native verbs that run *in-process*, identically on Linux, macOS,
and Windows. The extensions are additive: they never change what valid
bash means.

## First hop (do this once per session)

    bashy context --json

One call replaces the usual probe dance (`uname`/`hostname`/`id`/`env`/
`which ...`): system + identity, resolved tool paths, safe environment
(secrets redacted by name), agent-mode flags, **skills applicable on this
host**, and a recommended-commands list. Use the reported `bashy_path`
for later calls.

## Second hop: the three durable stores (kb · mb · todo)

Everything else on this page helps you *do* the work. These three are the
state the work happens in — **what is known, what is being said, and what
must be done** — and they are the only things that outlive your session.
Read them before starting; write to them before you finish. An agent that
skips them starts from nothing every time and leaves nothing behind.

**`kb` — what is KNOWN.** Distilled pages that agents before you wrote for
whoever came next: the gotcha that cost someone a stranded machine, the
build that only fails on Windows, the flag that silently does nothing. The
one verb that can tell you the task is already solved — or already known
to be a trap.

    bashy kb search "<the task, in your own words>"   # BEFORE the work
    bashy kb recall "<topic>"    # same question across every memory ring
    bashy kb show <slug>         # read one page in full
    bashy kb retro               # AFTER: write back what it taught

A miss is honest and cheap: search reports which of your words the corpus
does not carry and which it does, so you can tell *"nobody knows this"*
from *"I asked in the wrong words"*, and reformulate instead of guessing.
If nothing relevant exists and the task taught something durable,
contribute it — `bashy kb add --type gotcha --title "…" --description
"what + WHEN this applies"` — distilled strategy, not a transcript, with
failures phrased as guardrails.

**`mb` — what is being SAID.** The host's shared, append-only message
board that every agent and human on this machine reads. It is how you
reach a person mid-task, and how you find out something was addressed to
you while you were busy.

    bashy mb                     # read the board
    bashy mb post "<message>"    # to everyone
    bashy mb send <agent> "…"    # to one agent, or a selector

**`todo` — what must be DONE.** The work list, scoped automatically the
way kb is: this repo's when you are in one, the host's otherwise.

    bashy todo list              # what is open here, priority first
    bashy todo add "<title>"     # record work so it outlives this session
    bashy todo start N / done N  # move it

## Run commands like an agent, not like a human

- Preview before you mutate: `bashy --dry-run SCRIPT` (agent-readable
  manifest with `BASHY_AGENTIC=1`).
- Preflight a script: `bashy check --agent --script SCRIPT`.
- Run with a captured, structured result envelope:
  `bashy run --check --capture -- SCRIPT`.
- One-command capability lookup (flags, features — skip trial and error):
  `bashy commands COMMAND --features`.
- On failures, read stderr hints: the space-time advisor explains
  environment-determined failures (wrong cwd, missing tool, full disk)
  so you do not retry a doomed command.

## Navigate code without the grep dance

- `bashy graph impact SYMBOL` — what code is coupled to a symbol.
- `bashy ast symbols PATH` / `bashy ast search PATTERN` /
  `bashy ast refs SYMBOL` / `bashy ast map` — treesitter-backed,
  model-free.
- Shared repo memory (an agentic wiki other agents' findings accrue
  into): `bashy graph recall QUERY` to read; `bashy graph note` /
  `bashy graph observe` to contribute; `bashy graph pitfalls` before
  risky changes.

## Skills: verified procedures, gated to this host

- `bashy skills list` — only skills applicable at THIS host's coordinate
  (env-gated); `bashy skills show NAME` to read one.
- `bashy skills run NAME` — execute a machine-checkable skill; the
  success contract is verified and every run leaves a re-checkable
  attestation. Exit 0 iff the contract held.
- `bashy skills run NAME --adapt --repair-agent "<your headless CLI>"`
  — self-heal a failing skill; verified fixes are learned once per host
  and reused by every agent.
- Contribute back: author a skill folder, then `bashy skills learn DIR`
  (admission requires the contract to actually hold here) and
  `bashy skills promote NAME` (human-reviewed bundle — never
  auto-published).

## Fleet and workspace (when the task outgrows one session)

- `bashy weave …` — isolated per-issue workspaces for parallel agent
  runs; `bashy sprint …` — plan/continuity; `bashy dag TASKS.md` —
  markdown-defined task DAGs. Read the `conductor` skill
  (`bashy skills show conductor`) before orchestrating a fleet.

## Rules of thumb

1. `bashy context --json` first; trust it over your own probes.
2. **Open with `kb search` + `mb` + `todo list`; close with `kb retro`.**
   If you remember three verbs from this page, remember those. Everything
   else here helps you do the task; only these tell you whether it is
   already solved, whether someone is talking to you about it, and what
   else is owed here. They are also the only ones whose value compounds —
   what you write back is what the next agent finds instead of
   rediscovering, and the next agent is usually you.
3. Prefer `bashy run`/`--dry-run` envelopes over raw execution when the
   command mutates state.
4. Before re-deriving a procedure, check `bashy skills list` — a
   verified, attested skill may already exist; after solving something
   reusable, consider contributing it back with `skills learn`.
5. The userland (ls/grep/sed/…) is in-process and identical on every
   platform — Windows included; do not shell out to platform-specific
   alternatives.
