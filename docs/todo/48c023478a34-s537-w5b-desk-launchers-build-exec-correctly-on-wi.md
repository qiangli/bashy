---
id: 48c023478a34
kind: bug
title: S537.W5b desk launchers build + exec correctly on Windows (POSIX -o path, .exe)
seq: 2963
status: done
priority: p1
labels:
    - desk
    - windows
created: 2026-10-30T20:45:16.190107Z
weave: 385
assignee: codex-gpt6-sol
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, chart, whiteboard, brainstorm) on one contract + UI kit'
closed: 2026-10-30T21:59:10.699563Z
closed_by: s537-manager
---

Bug found by the W5 Windows run (manager, 2026-10-30, noviwin1, Windows 11 amd64, Go 1.27.0, candidate bashy.exe built from bashy e070246d): `bashy dag roundtrip` fails at build for every desk tool. Repro:
  BASHY=$NB $NB --bashsharp examples/desk/notepad/notepad.bsh unsupported-desk-verb --json
  -> notepad.bsh: line 58: /c/Users/noviadmin/.cache/bashy/desk-notepad/notepad: No such file or directory (rc 127); cache dir empty.
Cause: each desk .bsh does `(cd "$here" && GOWORK=off CGO_ENABLED=0 go build -trimpath -o "$bin" .)` with bin=$HOME/.cache/... — native Windows go receives the POSIX /c/Users/... path unconverted (bashy converts /c/ argv only for binmgr-managed tools, by design) and writes elsewhere; the binary also lacks the .exe suffix that exec needs.
Fix in all six launchers (examples/desk/{notepad,form,chart,viewer}/*.bsh, examples/whiteboard/whiteboard.bsh, examples/brainstorm/brainstorm.bsh), the same way, no per-OS branches beyond the suffix: pass go an output path it can resolve on every OS (e.g. relative to the build dir: `rel=$(realpath --relative-to="$here" "$bin")`, then `cd "$here" && go build -o "$rel$exe" .`), with exe=$(go env GOEXE); test/exec "$bin$exe". Also make examples/desk/dag.md default BASHY to the running shell when `bashy` is not on PATH.
Red/green: a Go test or Bash# check that the launcher's computed build output path is relative/resolvable and carries GOEXE (simulate GOOS=windows via `go env GOEXE` override or a seam); manager reruns the recipe on noviwin1 for the real proof.
Gate: gofmt/vet/test in the touched modules; bash -n on every .bsh/.sh; local `bashy dag roundtrip` on macOS still 6/6.

=== REWORK (manager, Windows proof on noviwin1 at bashy d929f8b3) ===
d929f8b3 (realpath --relative-to + GOEXE) is merged but still fails on Windows: bashy's realpath cannot resolve an existing /c/Users/... path ("realpath: /c/Users/noviadmin/s537/bashy/examples/desk/notepad: No such file or directory"; `realpath .` returns C:\Users\...). cd, mv and cp DO handle /c/ paths (verified). Fix all six launchers WITHOUT realpath: build into a relative path inside the module, then move it into the cache, e.g.
  (cd "$here" && GOWORK=off CGO_ENABLED=0 go build -trimpath -o ".desk-build/notepad$exe" . && mkdir -p "$cache" && mv -f ".desk-build/notepad$exe" "$bin")
(add .desk-build/ to the tools' .gitignore). Update launcher-path-test.sh to assert no realpath use and a relative -o. Do NOT work around realpath elsewhere — it is filed as its own bug.
Manager reruns the recipe on noviwin1 after merge.

=== REWORK 2 (manager, noviwin1 proof at bashy 36b1e125: build OK; notepad + viewer PASS; 4 FAIL) ===
1. chart, whiteboard, brainstorm launchers rebuild on EVERY call (no staleness check; notepad has one). On Windows a running .exe cannot be replaced: `mv: cannot move '.desk-build/chart.exe' to '/c/Users/noviadmin/.cache/bashy/desk-chart/chart.exe': Permission denied` while that tool's server is running -> CLI calls fail ("wrong JSON schema"). Fix every launcher the same way: rebuild only when the cached binary is missing or a source file is newer (use -f, not -x — Windows exec bits are not reliable); if the move fails because the target is busy, keep using the existing binary (or build to a unique name and exec that); never leave a call without a binary. Unix behaviour unchanged.
2. form: `dag: effect cap: target "roundtrip": "form.exe" needs unknown not in Effects: cred,destroy,exec,net,read,write` — bashy dag's effect classifier does not know the suffixed name. Do not fight the classifier: in examples/desk/dag.md invoke the tools through their launchers / the same command names on every OS (no .exe in the recipe). (The classifier gap is filed separately by the manager.)
Gate: launcher-path-test.sh extended for (1) (stale check present, no unconditional rebuild); bash -n; macOS roundtrip 6/6. Manager reruns noviwin1.

=== REWORK 3 (manager, noviwin1 at bashy ddfb857a: notepad, chart, viewer, brainstorm PASS; form + whiteboard FAIL) ===
The roundtrip picks `addr=127.0.0.1:$((30000 + RANDOM % 20000))` without checking it is free. On noviwin1 the picked ports were in use: form "listen tcp 127.0.0.1:42567: bind: Only one usage of each socket address ... is normally permitted", whiteboard same on 44998; the recipe then waited ~340 s for health. (No leftover desk servers were running; the only excluded range is 50000-50059.) Fix in examples/desk/dag.md only: start each server with a fresh random port and, if it exits or logs a bind failure before it is healthy, retry with another port (bounded, e.g. 5 attempts), with a short health timeout per attempt; same logic on every OS. Keep the tools unchanged.
Gate: macOS `bashy dag roundtrip` 6/6 (simulate a busy port: hold one port open with a tiny listener and force the first pick onto it to show the retry). Manager reruns noviwin1.
