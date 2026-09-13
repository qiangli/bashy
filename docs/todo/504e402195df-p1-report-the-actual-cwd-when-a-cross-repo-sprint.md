---
id: 504e402195df
kind: task
title: 'P1: report the actual cwd when a cross-repo sprint gate fails'
seq: 233
status: todo
priority: p1
created: 2026-09-06T01:08:49.784406Z
sprint: 130
---

FOUND while stopping sprint 127 — the gate refused three times before I understood why.

SPRINT 127 SCOPE: fix the misleading failure by reporting the actual gate cwd and
documenting the explicit `cd` workaround. `--gate-cwd`, named-repo resolution, or any
other new gate capability is a deferred feature and is not part of this story.

`sprint stop 127 --gate "go test ./pkg/weave/ && scripts/crossvet.sh"` fails with
"stat /Users/qiangli/projects/poc/dhnt/bashy/pkg/weave: directory not found". The gate runs
in the sprint TRACKED REPO (bashy), while the code this sprint touched lives in coreutils.

A sprint is explicitly CROSS-REPO — sprintRun carries a Repo per run and the umbrella has
fifteen submodules — so "the repo" is not a well-defined place for a gate to run. Today the
answer is silently "whichever repo you happened to track first", which is right by accident
at best.

THE REFUSAL BEHAVIOUR IS CORRECT AND SHOULD NOT CHANGE: it ran the gate, the gate failed, it
REFUSED to stop and said workers stay parked. That is the drain design working exactly as
written — a red gate must not close a sprint. The defect is only that the failure looks like
a regression in the code when it is actually a wrong-directory error.

FIXES, cheapest first:
  1. When the gate fails with a not-found on its own command/path, SAY WHERE IT RAN:
     "gate ran in <cwd> (sprint tracked repo)". One line, and it would have saved three
     attempts. This alone may be enough.
  2. Consider `--gate-cwd <dir>`, or resolving a gate relative to a named tracked repo.
     Only if 1 proves insufficient — a flag is a cost.

Workaround that works today and should go in the help text: prefix the gate with an explicit
cd, e.g. --gate "cd /path/to/repo && go test ./...". That is what finally passed here.

MY OWN ERROR, recorded so the round is honest: after the first failure I re-sent the SAME
gate string twice more believing I had changed it. The tool was not at fault for those two;
they are why finding 1 above is about making the failure self-explanatory.
