# Sprint 561 A4: dev-host install gate on the frozen candidate

Operator direction (2026-11-04): install and verify the candidate BEFORE the
long licensed run, so the run measures a build already proven on the dev host.

- Candidate: bashy `f1bcd711` (sh `74586fde`, coreutils `211789fa`), built
  `GOWORK=off make build` in a clean worktree; `bash --version` reports
  `5.3.0(1)-bashy-dev (f1bcd71)`.
- Installed with `make install` (atomic `tools/installbashy`, previous pair
  kept) into `~/.local/bin` and `~/bin`. The separate bare `~/bin/sh`
  (previously `rc.23-dev`, not managed by `make install`) was replaced with the
  candidate `bash.real`; the prior binary is kept as `sh.previous-s561-rc23`.
- Gate (`bashy gate`, manager-run, default PATH so `sh` resolves to the
  installed candidate): `GOWORK=off go test ./internal/agentos -run
  '^TestRunSignaledIsNonLossy$'` — **passed**. This was A2 finding F3, red on
  the baseline because `kill -KILL $$` exited 137 instead of dying by signal.
- Installed-PATH smoke: `bash`/`bashy` report `f1bcd71`; `sh -c` arithmetic and
  pipeline; `bashy commands`; `bashy cat`; `bash -c 'kill -KILL $$'` now dies
  by SIGKILL (parent reports `Killed`, status 137 = 128+9).
- The 2026-08-28 board report (`${BASH_SOURCE[0]:-d}` under `set -u`, and the
  launcher resolving `bash.real` through PATH) was re-checked on the candidate:
  both fixed since bashy `94f3309f` / sh `1600181d` and still correct — `-c`,
  script file, function frame, and a Homebrew-first PATH `make` recipe all
  match GNU Bash 5.3.15. No change needed.
