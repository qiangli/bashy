---
id: 01a21822-3c26-7e11-9ec4-ac323d01427b
seq: 38
form: page
type: runbook
title: 'Build an app from a shapeware spec (forward: spec to source), the builder''s guide'
description: 'WHEN you are asked to build or rebuild an app from a shapeware spec folder and it will be judged by conform at a stated venue: compile the spec first, read the rules and the lifecycle, build for the venue, write files early, run conform while building, implement a rule as a rule, deliver source, a launcher and a working lifecycle with no host-built binaries and no processes left running'
tags:
    - shapeware
    - conform
    - builder
    - spec2src
status: candidate
evidence: '2026-11-26: failure classes from 43 rebuild runs of examples/desk/form across five vendors; the guide itself has not yet been given to a builder'
source:
    tool: claude
created: "2026-11-26T13:53:46Z"
---

Goal: build an app from a shapeware spec folder (`CONTRACT.md`, `features.yaml`, `examples/`, `dag.md`, a task file) so that it passes the judge at the stated venue and can be installed and run through its lifecycle. Lessons are from rebuilding bashy's `form` from its spec by agents of four vendors. Related: [[shapeware-spec-from-app]], [[shapeware-spec-from-plan]], [[shapeware-operate-from-dag]] to install and run what you built, a rebuild test (a builder that has not seen the source rebuilds the app and the judge decides).

Tools and format: the judge is `bashy conform` (`spec check`, `run`, `cert`, `mutate`). Where a step says `conform ...`, type `bashy conform ...`; on a machine without bashy use the standalone `conform` binary (github.com/dhnt/conform). The format on one page is `examples/desk/form/references/shapeware.md` in the bashy source, and the folder around it is the worked example, with a full `CONTRACT.md`, `features.yaml` and `dag.md`. `bashy src fetch bashy` puts the source under `${BASHY_HOME:-$HOME/.bashy}/src/bashy`.

## Steps
0. **Compile the spec you were given:** `conform spec check .` (the judge also says so when you run it: "spec check reports N finding(s)"). Each finding is a place where the spec does not fully decide the behaviour. Do not guess silently there: take the reading that the examples and the rule's purpose both support, and write the choice in your notes.
1. **Read the rules section of `CONTRACT.md` before the assertions.** Envelope keys, projection flags, open/closed outputs and error form apply to every command.
2. **Read the task file and `dag.md` for the venue and the lifecycle:** OS, interpreter and toolchain versions, network, what is absent, and the steps your build must satisfy (build, test, install, run, serve). Build for that venue, not for the machine you are on.
3. **Write files early.** Start the launcher and a first command within the first minutes, then extend. A long plan with nothing on disk can be cut off.
4. **Run the judge while you build:** `conform run CONTRACT.md --path . --fresh-home`. Fix by assertion name until all pass.
5. **Cover every example file and every input format** in `examples/`, including each syntax the examples use.
6. **Implement a rule as a rule.** Where the spec defines a set (which names are refused, which kinds are valid), implement the definition and test both sides of its boundary, not only the example shown.
7. **Errors:** exactly the stated stream, text prefix and exit code, and nothing else on that stream (no warnings from your interpreter).
8. **UI:** follow the stated label, required and control rules; the judge reads the accessible inventory, not your markup.
9. **Deliver source, the launcher and a working lifecycle.** Before you finish:
   - remove build outputs and caches from the workspace, or make the launcher rebuild when the cached binary does not run here;
   - make sure the launcher works from a fresh HOME with no network;
   - use only the standard library or dependencies vendored in the workspace;
   - stop every server or background process you started.

## Failure causes seen
- A binary built on the session host was shipped in the workspace and could not execute at the venue.
- A module deprecated in the venue's interpreter version printed a warning on the error stream and failed every error assertion.
- An input format named in the spec was not implemented because only the other format had been tried.
- A projection flag was honoured on some commands only, or dropped the always-present key.
- Nothing was delivered: the session ended during a long first planning turn.
- A rule given by example was implemented for the example's form only (`api_key` refused, `stripe-api-key` accepted).
- Servers started for self-checks were left running after the session.
