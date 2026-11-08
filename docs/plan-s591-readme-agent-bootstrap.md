# Sprint 591 — README as agent bootstrap

## Goal

A user starts any AI coding agent and says *"Set up bashy from
https://github.com/qiangli/bashy"*. The agent reads the README as its skill,
asks the user **once** (a: bashy only; a+b: also an unpaired outpost boot
service), installs, wires itself (skill, MCP, shell), and keeps working
through bashy verbs. A human skimming the same page gets the features at a
glance and a copy-paste install.

## Shape of the page

1. Human hook: one-line pitch, the "tell your agent" sentence, manual
   one-liners (`--no-service`), link to `docs/install.md`, demo, *At a glance*,
   the bash / Bash# / Yoke table with evidence links.
2. Agent procedure, runnable as `bashy dag README.md <target>`: consent gate,
   `install`, `path`, `connect`, `bootstrap` (b only), `orient`, then a
   "user says → you run" table.
3. No hardcoded release version: the release badge and `releases/latest`
   links stay current on their own.

Long reference material moves, unchanged, to `docs/install.md` (install
routes, signing, from source, containers) and `docs/usage.md` (usage, flags,
mode selection, compatibility notes, development).

## Evidence so far

Cold start on 2026-11-07, draft mounted in a fresh non-root Debian container
with Claude Code: R1 one consent ask; R2 "a" installed and wired, all exits 0;
R3 new login session reported skill loaded, bashy MCP connected, Bash tool
running in bashy. An earlier draft failed R3 (bare `bashy` MCP command,
ENOENT), which produced the `path` step and a story.

## Stories

- README + docs split (this delivery).
- Cold start from the real URL with claude and codex.
- Bugs found by the cold start: installer default `--service`; `skill export
  --dry-run` scope; MCP bare command; `outpost service install` sleep mask;
  promoted release reports `-dev`.
