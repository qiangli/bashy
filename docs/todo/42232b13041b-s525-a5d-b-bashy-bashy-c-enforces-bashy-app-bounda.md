---
id: 42232b13041b
kind: bug
title: 'S525.A5d-b bashy: bashy -c enforces BASHY_APP_BOUNDARY (shaped units actually confined)'
seq: 2837
status: done
priority: p0
labels:
    - app
    - windows
created: 2026-10-29T19:32:01.050591Z
weave: 340
assignee: claude-opus5.5
sprint: 525
sprint_id: 9a9a4443-02c8-53a3-b0bd-96bd67980643
sprint_title: 'Software Shaping foundation: verified personal features and same-host promotion'
closed: 2026-10-29T20:44:27.813981Z
closed_by: s525-manager
---

Part of S525.A5d (yoke b8759986, goal a5d), not a new feature. Yoke run #720 (a66d24e3, rebasing onto yoke main) added pkg/shape/boundary: the console passes each unit its policy in $BASHY_APP_BOUNDARY, and yoke's own runner enforces it via boundary.FromEnv(env) -> *Guard and boundary.Run / boundary.Decorators() (interp middlewares: open/stat/cd/symlink path checks, exec gate for @runs / @sandbox islands, /dev/tcp refusal). But production units execute as 'bashy -c' in this repo, which ignores the variable, so nothing is enforced on a real host. Fix (keep it small, reuse yoke's package; no new isolation layer, no sandbox): when bashy -c starts with BASHY_APP_BOUNDARY set, build the Guard with boundary.FromEnv and run the script under the same handlers (identical on every OS). Malformed or unreadable value = refuse to run (fail closed), never run unconfined. Unset = behaviour unchanged. Tests (table-driven, no POSIX-only helpers, run on Windows + Linux): through the real bashy -c entry point, other principal write, other app read, other app write, write outside grants, undeclared external program, /dev/tcp -> all DENIED with reason; own app data read/write -> OK; app call to another app's unit as the same principal -> OK. Gate: make test in bashy (focused packages) + GOOS=windows go vet ./... . Depends on yoke #720 landing on yoke main (pin bump in bashy go.mod in the same change). Do not touch bashy's shell lookup order.

BUG FOUND IN LIVE PROOF 2026-10-29 20:2xZ (elephant WSL, A9 browser-run, bashy f413bdcb) — fix in THIS story: under BASHY_APP_BOUNDARY the exec gate refuses `awk` as "undeclared external program" (exit 126), so the generic notes app's list unit (`awk ... "$BASHY_APP_DATA/notes.txt"`) returns [] and A9 baseline.alice_sees_own / bob_sees_own fail. Write, [ -f ], tr and cat work. Local repro (darwin, f413bdcb build): BASHY_APP_DATA=$D BASHY_APP_BOUNDARY='{"app":"notes","principal":"device:alice","data":"'$D'","home":"'$H'"}' bashy -c 'printf "n1\n" >> "$BASHY_APP_DATA/notes.txt"; awk "{print}" "$BASHY_APP_DATA/notes.txt"' x  -> "boundary: exec awk denied: undeclared external program". Cause: runBounded's ServedInProcess predicate is coreutils tool.Lookup, but bashy serves awk (and possibly other utilities) through another in-process path that the guard does not see. Fix: the guard must treat as in-process EXACTLY the set of commands bashy's runner executes without starting a new process (same predicate/dispatch the normal runner uses), so file access still goes through the guard's open handler; anything that really starts an external process stays gated. Add table-driven cases through the real entry point for every in-process utility used by Bash# units, at least awk, sed, grep, sort, head, tail, wc, cut, tr, jq, date, plus one genuinely external program still DENIED. Keep everything else unchanged. Gate as before (focused cmd/bashy + GOOS=windows vet + windows cross-build; NO full internal/agentos rerun on Dragon unless src pins change).
