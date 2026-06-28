# Shell conformance comparison — bashy vs gosh vs zsh

Measured 2026-06-28 on darwin/arm64 (bashy `4e31ce1`, sh fork `36c3fc07`).
Reproduce with the commands in *Methodology* below.

## bash 5.3 compatibility — the GNU bash 5.3 test suite (86 fixtures)

Driven by `make test-bash` (each fixture is GNU bash's own `*.tests` run through
the shell-under-test and diffed against the `*.right` expected output).

| Shell under test | Score | What it is |
|---|---|---|
| **bashy** (`cmd/bash`, the drop-in) | **86 / 86 — 100%** | the full Bash 5.3 drop-in CLI |
| **gosh — fork** (`mvdan.cc/sh/v3` fork `cmd/gosh`, our patched interp) | **56 / 86** | the bare proof-of-concept shell on our interp |
| **gosh — pristine upstream** (`mvdan.cc/sh/v3/cmd/gosh@latest`, no fork patches) | **3 / 86** | what upstream ships today |
| **zsh** | **N/A** | not a bash-family shell — the bash test framework hangs it (86 timeouts ≠ a conformance score) |

### The progression (the upstream-PR story)

```
pristine upstream gosh                3 / 86
  + our interp bash-5.3 patches   →  56 / 86   (gosh, fork)     ... +53 fixtures
  + the bashy drop-in CLI layer   →  86 / 86   (bashy)          ... +30 fixtures
```

- **+53 from the interp patches** — the unmerged Bash 5.3 `interp`/`expand`/`syntax`
  patches carried in the [`qiangli/sh`](https://github.com/qiangli/sh) fork. This is
  the portion that benefits *any* consumer of the engine, `gosh` included.
- **+30 from the bashy CLI** — fixtures that need the full drop-in's CLI layer that
  the minimal `gosh` PoC lacks: bash flag parsing, startup-file loading, prompt
  expansion, history (`histexpand`/`history`), `bind`, etc.

## POSIX-mode parity vs bash 5.3 — all shells IN the Linux container

The cleanest comparison runs **every** shell inside the same `bash:5.3` Linux
container as the reference, removing all host/OS skew (the candidate-on-macOS /
oracle-in-Linux asymmetry of the host harness). 39 probes, reference =
`bash 5.3 --posix`, container = linux/aarch64:

| Shell (POSIX mode, in-container) | Match / 39 |
|---|---|
| bash 5.3 (self-check) | 39 / 39 |
| **bashy** `--posix` | **38 / 39** (only #58 — see below) |
| **gosh — fork** `--posix` | **34 / 39** |
| **gosh — upstream** `--posix` (parse-only) | **12 / 39** |
| dash | 27 / 39 |
| zsh (sh-emulation, `ARGV0=sh`) | 19 / 39 |

> The `--posix` flag on `gosh` is a **fork addition** — pristine upstream `gosh`
> rejects it (`flag provided but not defined: -posix`). And even *with* the flag,
> upstream gosh is only **12/39**: the flag is parse-only (LangPOSIX), because
> upstream interp has **no POSIX runtime mode** (its `set -o` table has no `posix`
> entry). The 22-point gap from 12 → 34 is our fork's `interp.WithPosixMode`.

### Probe #58 `kill -l` — fixed (sh `7f5871d6`)

bashy's `kill -l` now lists the full Linux signal set — `SIGSTKFLT`/`SIGPWR` and
the realtime signals `SIGRTMIN..SIGRTMAX` (named `RTMIN+n`/`RTMAX-n` like bash) —
**matching glibc Linux bash byte-for-byte** (verified vs Debian glibc 2.41).
Against a glibc `bash` the in-container score is the clean **39/39**.

The table above uses the official `bash:5.3` image, which is alpine/**musl**
(SIGRTMIN=35) — bashy, a static pure-Go binary with no libc, follows the standard
**glibc** convention (SIGRTMIN=34), so against the musl oracle one realtime
boundary signal still differs (38/39). That residual is purely a musl-vs-glibc
libc convention, not a bashy gap.

## yash POSIX corpus (`make test-yash`, bashy only)

| Metric | Value |
|---|---|
| bashy pass rate | **1763 / 1825 = 96%** |
| bashy-specific failures (bash OK, bashy FAIL) | **0** |

## Takeaway

**bashy is the closest to bash 5.3 on every axis** — 86/86 bash compat, 38/39
POSIX parity, 0 bashy-specific yash failures. `gosh` shares bashy's interp so it
tracks closely on POSIX (34/39) but lags on full bash compat (56/86 — the CLI
gap, by design: it is a minimal PoC, not a drop-in). `zsh`, a non-bash-family
shell, is the most divergent (21/39 POSIX; the bash suite is N/A for it).

## Upstreamable to `mvdan/sh`

Two clean contributions raise the bare upstream `gosh` substantially:

1. **A `--posix` flag for `gosh`** — small, self-contained; enables POSIX-mode use
   and lets `gosh` be measured against POSIX suites at all.
2. **The Bash 5.3 `interp`/`expand`/`syntax` patches** — the engine-level work that
   takes `gosh` from **3 → 56** on the bash suite (and underpins bashy's 86). The
   fork is maintained close to upstream specifically so these can be offered back
   as single-commit topic branches (see `qiangli/sh` CLAUDE.md § Workflow).

## Methodology (reproduce)

```sh
# bash 5.3 suite against an arbitrary shell: override BASHY (the harness sets
# THIS_SH=$(pwd)/$BASHY), pointing it at a binary copied into ./bin/.
make build-bash                                 # bashy: bin/bash
cp /path/to/gosh bin/gosh && make test-bash-run BASHY=bin/gosh

# pristine upstream gosh (no fork patches):
GOBIN=/tmp/up go install mvdan.cc/sh/v3/cmd/gosh@latest
cp /tmp/up/gosh bin/gosh-up && make test-bash-run BASHY=bin/gosh-up

# POSIX parity (oracle via embedded podman, no Docker needed):
PATH="$PWD/bin:$PATH" BASHY=./bin/bash    scripts/posix-parity.sh   # bashy
PATH="$PWD/bin:$PATH" BASHY=/path/to/gosh scripts/posix-parity.sh   # gosh --posix
# zsh has no --posix; run it in sh-emulation via a wrapper:
#   #!/bin/sh
#   [ "$1" = --posix ] && shift
#   exec env ARGV0=sh zsh "$@"
PATH="$PWD/bin:$PATH" BASHY=/path/to/zsh-posix-wrapper scripts/posix-parity.sh

# yash POSIX corpus:
make test-yash
```

zsh on the bash 5.3 suite is **N/A**, not 0: the suite's test framework assumes a
bash-like shell and blocks zsh (every fixture times out), so the bash suite cannot
produce a meaningful conformance number for a non-bash shell. The POSIX-parity
figure (zsh in sh-emulation) is the fair cross-shell comparison.
