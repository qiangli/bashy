---
id: 01a2186e-40e2-73d8-9e80-2d5795636b18
seq: 40
form: page
type: runbook
title: 'Operate a shapeware folder: build, test, install, run and serve from its dag.md'
description: 'WHEN an agent is asked to take an already built shapeware app folder to an installed command or a running service (operate, install, deploy, serve; the dag.md lifecycle; the judge against the installed app): get consent, run each lifecycle step exactly as dag.md writes it, gate on exit codes and on the contract against the installed app, and stop what you started; with the traps (an app that starts its own server, a display on a headless host, step limits, leftover processes, quota)'
tags:
    - shapeware
    - conform
    - operate
    - dag
    - install
status: candidate
evidence: '2026-11-26: on examples/desk/form, a plain script with no model carried out the whole lifecycle, and two lower-tier agent bindings of different vendors did it cold, 3 of 3 runs each; one app only'
source:
    tool: claude
created: "2026-11-26T15:16:48Z"
---

Goal: take a shapeware folder that is already built from its spec to an installed command and a running service, by carrying out its lifecycle exactly. This needs less skill than building: on the worked example a plain script with no model passes, and lower-tier agents passed cold. Building the folder from its spec is a different job: [[shapeware-source-from-spec]]. Writing the spec: [[shapeware-lift-app]], [[shapeware-spec-from-plan]].

Tools and format: the judge is `bashy conform` (`spec check`, `run`, `cert`, `mutate`). Where a step says `conform ...`, type `bashy conform ...`; on a machine without bashy use the standalone `conform` binary (github.com/dhnt/conform). The format on one page is `examples/desk/form/references/shapeware.md` in the bashy source, and the folder around it is the worked example. `bashy src fetch bashy` puts the source under `${BASHY_HOME:-$HOME/.bashy}/src/bashy`.

## Steps
1. **Read `dag.md` first, and only `dag.md` for the commands.** Each step is a `### name` heading with a purpose line, `Effects:`, `Band:` and one code block. Run a step with `bashy dag -f dag.md <step>` from the folder; without bashy, run the step's code block with `sh`, exactly as written. Never replace a step with commands of your own, and never change a file that came with the folder.
2. **Consent before the first step that writes or runs anything on a machine.** Show the person who answers for the machine the `Effects:` line of each step and get a yes. Ask again only for something beyond what was approved.
3. **Check the band.** A step whose `Band:` is above your own is not yours to run; say so and hand it on. `none` needs no model; `unmeasured` means nobody has measured it yet, so report how it went.
4. **`build`.** Exit status 0 is the gate. A build with nothing to compile still says so.
5. **`test`.** It runs the judge on the contract against the fresh build, before anything is installed. Read the PASS and FAIL lines. Any FAIL: stop and report it; do not go on to install.
6. **`install`.** Use the default place unless told otherwise. When the step installs a launcher, check that its directory is on the PATH.
7. **`run`.** Pass the app's arguments as one value, `ARGS='...'`; the step splits them and never evaluates them. Keep the output when a later step needs a value from it (an id, a URL).
8. **`serve`.** It runs in the foreground: start it in the background of your shell, keep its process id, use the service, then stop it with TERM. A stopped service reports the signal's status (143), not 0.
9. **Check the installed app, not the folder.** Run the contract again with the installed command first on the PATH (`conform run CONTRACT.md --path <bin dir> --fresh-home`). This is the only run that proves the trip.
10. **Report** each step with its exit status, and leave nothing running that you started.

## Traps
- **An app that starts its own background server.** The page may answer before `serve` is run, and `serve` may then return at once or fail on a busy port. If `serve` exits early, read its output before calling the step failed. The spec should say what `serve` does in that case and how the app is stopped; if it does not, report that as a gap in the spec.
- **An older bashy.** Before the release that carries this page, bare `conform` in a bashy shell ran bashy's own test batteries (now `selftest`). If the judge's output looks like a test suite list, update bashy, or call the judge binary by its path.
- **A display on a headless machine.** With `DISPLAY` set and no working display server, a desktop query (opening a browser is one) can hang for tens of seconds and the judge reports a timeout. Unset `DISPLAY` and `WAYLAND_DISPLAY` on a test host, and set `BROWSER` to a no-op where no window should open.
- **Step limits.** The judge's default limit is 30 seconds per command. A first command that compiles or waits can exceed it; that is a condition of the venue to state, not an app failure to hide.
- **Leftover processes.** Stop what the steps started, by the folder's path, never by a broad name.
- **Quota.** A provider refusal at the first command means nothing was measured. Switch model or wait, then rerun.
