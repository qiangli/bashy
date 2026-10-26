---
id: 93426a7377cf
kind: feature
title: 'bashy connect <host> [-- cmd]: ssh-shaped wrapper over outpost ssh (interactive shell or TTY command on the remote host; WSL default on Windows)'
seq: 1952
status: assigned
priority: p1
created: 2026-10-25T12:58:13.775422Z
weave: 282
assignee: codex-gpt6-astra
sprint: 505
sprint_id: 4354bb44-85fc-5fff-b997-58e305948c09
sprint_title: 'v1.0 user-path gaps: master lists every host a user owns across satellites; a release reaches every host and satellite'
---

status    assigned
  priority  p1
  created   2026-10-25T12:58:13Z
  assignee  codex-gpt6-astra
  sprint    #505 · 4354bb44-85fc-5fff-b997-58e305948c09
  weave     #282 (in flight)

Operator 2026-10-25: dogfood remote development the way ssh/sshd work. Agents and bashy run ON the remote host; the local machine is only a terminal.
Deliver ONE thin verb in bashy (no new transport): `bashy connect <host> [-- <cmd> [args...]]`
- no cmd: interactive login shell on <host> (bashy shell), TTY attached, like `ssh host`.
- with cmd: run it on <host> with a TTY (`ssh -t host cmd`), exit code propagated; e.g. `bashy connect elephant -- codex`.
- transport: the existing outpost ssh path (outpost sshd over the tunnel / LAN-direct); reuse its host resolution, user defaulting and elevation. On a Windows host default into WSL Ubuntu when present (`--native` to stay in Windows).
- `--help` with 3 examples; register in the command catalog.
Tests: argument parsing and the exact outpost ssh invocation built (no real network); TTY/exit-code passthrough with a fake transport. Gate: go test for touched packages + go build ./... (also GOOS=windows). Trailers for this story only.

KISS scope (operator 2026-10-25): interactive 'bashy connect <host>' is the deliverable (dev on elephant from Dragon); PTY '-- cmd' and WSL auto-select dropped (run wsl inside the shell). Code merged: bashy acec0926.
Precondition before touching elephant (operator 2026-10-25, elephant is headless): prove outpost auto-rollback LIVE on noviwin1 (Windows, like elephant) first. No live bad-candidate test exists yet (S497 gate was unit tests only; chaos matrix b10f7696 not run).
1. noviwin1 -> rc.14 outpost via --from + sha256 (also clears its bare-commit build). ssh back after.
2. Bad candidate: rc.14 source + exit-at-boot, version newer so the never-older guard admits it. upgrade --from. Expect revert to .previous, ledger + quarantine entry, ssh :2222 answers again within the confirm window.
3. Same bad candidate, then reboot noviwin1 before it confirms. Expect: comes back on the previous build, ssh answers (checks the supervisor is not the bad binary after reboot).
Only after 1-3 pass: elephant rc.12-dev -> rc.14 (announced to Lane 1), then the live 'bashy connect elephant' check.

Status 2026-10-26 (s505-manager): code merged (bashy acec0926). Blocked on 6cba9217: the outpost hang-revert fix must be released and the hang test re-run on noviwin1 before elephant is upgraded to rc.15 (elephant runs rc.12-dev; zoo, its satellite, is now v0.18.39). Lane 1 (s499-manager) has not answered the elephant share proposal. Then: live 'bashy connect elephant' from Dragon.
