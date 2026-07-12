---
id: 3e91c872c8c0
kind: bug
title: 'CI is red: .sibling-pins is stale, so CI builds bashy against a coreutils that predates gate/issue/judge'
status: triaged
stage: test
priority: p0
refs:
    - ../coreutils
    - ../sh
reporter: qiangli
created: 2026-07-12T22:54:10.674823Z
weave: 10
---

## Symptom

`test.yml` has failed on EVERY bashy commit today (e4f4d2c, 709f899, dc0cc2a, 6e1d934),
on BOTH the ubuntu and windows legs. The auto-heal loop filed it as
qiangli/dhnt-ci-failures#2.

    internal/agentos/agentos.go:74: no required module provides package .../pkg/gate
    internal/agentos/agentos.go:76: no required module provides package .../pkg/issue
    internal/agentos/agentos.go:78: no required module provides package .../pkg/judge
    ... also pkg/lexicon, pkg/handoff, pkg/policy/audit, pkg/policy/coord

## Root cause (already diagnosed — do not re-derive)

CI has no umbrella. `scripts/bootstrap-siblings.sh` clones the siblings at the SHAs in
`.sibling-pins`, and that file is STALE:

    .sibling-pins            reality (origin/main, all green locally)
    sh=c6c7ba84              bece9532   (c6c7ba84 is an ancestor — just behind)
    coreutils=14bbc0df       1b2a301a   (predates gate/issue/judge/lexicon/coord)
    readline=36b5a209        36b5a209   (correct)

So CI clones a coreutils from before this week's packages existed. Local builds pass
because the umbrella mounts the LIVE siblings as submodules — the pins are never
consulted. That is exactly why this went unnoticed for a dozen commits.

## Task

1. Bump `.sibling-pins` to the current sibling HEADs.

2. THE REAL FIX — make it impossible to recur. Add a committed pre-push hook that
   REFUSES a push when `.sibling-pins` disagrees with a sibling's actual HEAD.
   coreutils already has this pattern: `scripts/hooks/pre-push` + `core.hooksPath`.
   Mirror it. The check must:
     - be a no-op when a sibling directory is absent (a standalone clone has none);
     - name the drifting sibling and print the exact fix;
     - be overridable (`--no-verify`) — a guard nobody can bypass is a guard people
       delete.

   Rationale: local builds CANNOT catch this class of bug (the umbrella shadows the
   pins), and CI catches it ten minutes too late. Push time is the only honest moment.

3. Note in `CLAUDE.md` §Module wiring that bumping a sibling means bumping
   `.sibling-pins` in the same breath.

## Verify

    ./scripts/bootstrap-siblings.sh && go build ./... && go test ./internal/agentos/
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /dev/null ./cmd/bashy

## Out of scope

Do not touch the umbrella. Do not change any package under ../coreutils.
