---
id: 48c023478a34
kind: bug
title: S537.W5b desk launchers build + exec correctly on Windows (POSIX -o path, .exe)
seq: 2963
status: todo
priority: p1
labels:
    - desk
    - windows
created: 2026-10-30T20:45:16.190107Z
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, chart, whiteboard, brainstorm) on one contract + UI kit'
---

Bug found by the W5 Windows run (manager, 2026-10-30, noviwin1, Windows 11 amd64, Go 1.27.0, candidate bashy.exe built from bashy e070246d): `bashy dag roundtrip` fails at build for every desk tool. Repro:
  BASHY=$NB $NB --bashsharp examples/desk/notepad/notepad.bsh unsupported-desk-verb --json
  -> notepad.bsh: line 58: /c/Users/noviadmin/.cache/bashy/desk-notepad/notepad: No such file or directory (rc 127); cache dir empty.
Cause: each desk .bsh does `(cd "$here" && GOWORK=off CGO_ENABLED=0 go build -trimpath -o "$bin" .)` with bin=$HOME/.cache/... — native Windows go receives the POSIX /c/Users/... path unconverted (bashy converts /c/ argv only for binmgr-managed tools, by design) and writes elsewhere; the binary also lacks the .exe suffix that exec needs.
Fix in all six launchers (examples/desk/{notepad,form,chart,viewer}/*.bsh, examples/whiteboard/whiteboard.bsh, examples/brainstorm/brainstorm.bsh), the same way, no per-OS branches beyond the suffix: pass go an output path it can resolve on every OS (e.g. relative to the build dir: `rel=$(realpath --relative-to="$here" "$bin")`, then `cd "$here" && go build -o "$rel$exe" .`), with exe=$(go env GOEXE); test/exec "$bin$exe". Also make examples/desk/dag.md default BASHY to the running shell when `bashy` is not on PATH.
Red/green: a Go test or Bash# check that the launcher's computed build output path is relative/resolvable and carries GOEXE (simulate GOOS=windows via `go env GOEXE` override or a seam); manager reruns the recipe on noviwin1 for the real proof.
Gate: gofmt/vet/test in the touched modules; bash -n on every .bsh/.sh; local `bashy dag roundtrip` on macOS still 6/6.
