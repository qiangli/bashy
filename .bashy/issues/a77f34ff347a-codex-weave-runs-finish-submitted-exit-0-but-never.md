---
id: a77f34ff347a
kind: bug
title: codex weave runs finish 'submitted exit 0' but never commit — work stranded uncommitted in workspace
status: open
reporter: qiangli
created: 2026-07-12T23:23:15.112131Z
---

On dragon-2, THREE codex runs this session (#11,#12,#13) exhibit the same pattern: the codex agent does real, substantial work and exits 0 (weave marks the run 'submitted'), but never runs 'git commit'. Result: `git log main..HEAD` is empty (0 commits) while `git status` shows a full diff of completed work.

Evidence:
- #13 (version stamp): 7 files modified + 1 new test (version.go buildID, Makefile ldflags, context.go runtime.build, version_test.go) — all uncommitted, 0 commits, state=submitted exit 0.
- #12 (bashy claim wiring): 3 files modified (agentos.go, commands.go, commands_e2e_test.go) — all uncommitted, 0 commits, submitted exit 0.

Impact (severe): weave's merge path (`weave pull`) operates on COMMITS. A submitted-exit-0 run with 0 commits looks 'done' but has nothing to merge — the work is invisible to the pipeline and lost on `weave abandon`/`prune`. Compounds bug 8d598429 (submitted-with-0-commits marked done): the operator gate 'require commits>0' correctly rejects these, but then GOOD WORK is discarded because the agent wrapper omitted the commit step.

Root cause candidate: the codex registry agent's headless argv (the expansion of 'codex-gpt-5.5') does not include a commit instruction, or codex's headless mode does not auto-commit like claude-opus does. Either the weave codex wrapper must commit-on-submit (git add -A && git commit) after the agent exits 0, or the agent prompt/argv must mandate a commit. claude-opus does not show this (it commits its own work).

Workaround used this session: operator finalizes by committing the workspace diff before gating.
