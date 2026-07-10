---
name: bashy
description: Drive bashy, the agentic shell — a drop-in Bash 5.3 with agent-native extensions. Use whenever bashy is the shell you are running in (or is on PATH) to cut probing round-trips, preview destructive commands, run with structured result envelopes, navigate code without grep dances, and use environment-gated verified skills. Start every session with one `bashy context --json` call.
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
- `bashy list-symbols PATH` / `bashy search-symbols PATTERN` /
  `bashy find-references SYMBOL` / `bashy repo-map` — treesitter-backed,
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
2. Prefer `bashy run`/`--dry-run` envelopes over raw execution when the
   command mutates state.
3. Before re-deriving a procedure, check `bashy skills list` — a
   verified, attested skill may already exist; after solving something
   reusable, consider contributing it back with `skills learn`.
4. The userland (ls/grep/sed/…) is in-process and identical on every
   platform — Windows included; do not shell out to platform-specific
   alternatives.
