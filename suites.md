---
name: suites
description: Run the bashy/sh conformance test suites as independent, parallelizable targets
default: all
---

# Conformance test suites

Each target below is an **independent** suite — it builds its own `bin/bash` and
runs its own corpus, sharing no state with the others. They contend only for
CPU / podman / memory, never for logic, so they parallelize freely:

```bash
bashy dag suites.md -j8 -k           # whole matrix in parallel, locally (-k: one failure does not halt the rest)
bashy dag suites.md gotest test-bash # just a subset
bashy dag suites.md -j4 yash         # one suite (still runs its deps: none)
```

Caveat: the **timed** suites (`test-bash` has a 60s per-fixture timeout; `yash`
drives a multi-shell framework) can brush their timeouts under heavy parallel
load on a small box and throw *false* TIME/FAIL. Give them headroom — run with a
`-j` at/under the core count, and don't run them while an agent fleet is also
compiling. On a big box (many cores / lots of RAM) the whole matrix runs clean
at once; that is the intended `--mesh` use (see the "Mesh note" at the bottom).

The container suites auto-detect the runtime (`docker`, else `bashy podman`).

## Tasks

### gotest
sh engine unit tests (interp/syntax/expand); skips the docker-only *Confirm tests
that fail on a macOS host's bash 3.2. Green = the engine is unit-clean.
Tools: go
```bash
cd ../sh && PATH=/bin:/usr/bin:$(dirname "$(command -v go)") go test ./interp/ ./syntax/ ./expand/ -skip 'TestParseConfirm|TestRunnerRunConfirm'
```

### test-bash
Bash 5.3 fixture suite (86 fixtures), parallelized across cores. The 86/86 gate.
```bash
make test-bash-parallel
```

### parity
POSIX-mode parity probes: `bashy --posix` vs `bash 5.3 --posix`. 0-gate.
Tools: go
```bash
scripts/posix-parity.sh
```

### xcu-diff
Clean-room XCU corpus through the 5-oracle same-env differential. 0-gate.
Tools: go
```bash
scripts/posix-diff.sh
```

### oils-diff
Oils spec-test case code through the live 5-shell differential. 0-gate.
Tools: go
```bash
scripts/oils-diff.sh
```

### multishell
10-shell conformance panel (strict-POSIX + feature-rich). 0-gate.
Tools: go
```bash
scripts/multishell-diff.sh
```

### austin
Austin-Group defect/interpretation corner-case differential. 0-gate.
Tools: go
```bash
scripts/austin-defects.sh
```

### yash
yash POSIX `-p` suite — INFO (bashy pass rate vs the reference shells). The
conformance frontier metric, not a 0/1 gate.
Tools: go
```bash
scripts/yash-posix-suite.sh
```

### zsh
zsh-own-suite scoreboard (Tier 0 of the zsh-compatibility ladder) — zsh 5.9's
`Test/*.ztst`, non-interactive classes, bashy vs real zsh through the same
runner. INFO (never a gate; never quote a bare "N% zsh compatible" from it).
Tools: go zsh
```bash
scripts/zsh-scoreboard.sh
```

### uutils
uutils test-suite scoreboard — the MIT uutils/coreutils suite (cargo,
`UUTESTS_BINARY_PATH` override) run against the pure-Go coreutils multicall
from ../coreutils, the same tool registry bashy mounts in-process. INFO (many
cases assert uutils-specific diagnostics/extensions, so 100% is not the
target). Needs cargo + the gitignored clone at
../coreutils/reference/uutils-coreutils.
Tools: go cargo
```bash
scripts/uutils-scoreboard.sh
```

### chat-smoke
`bashy chat` interactive-launcher smoke — drives the governed native launch under
a real pty against an installed agent and asserts the whole contract (native
launch · live-sessions registry · mid-turn steer · capture tee · clean teardown).
INFO, and deliberately OUTSIDE `all`: it needs an installed third-party agent and
a real pty, so it SKIPs cleanly on a headless CI box. Run it on a dev machine with
`make smoke-chat` (or `bashy dag suites.md chat-smoke`); override the agent with
`AGENT=codex-gpt-5.5`.
Tools: python3
```bash
scripts/chat-smoke.sh
```

### all
Aggregate goal — depends on every suite, so `bashy dag suites.md` (default) runs
the full matrix. With `-jN` the independent suites fan out across N slots.
Requires: gotest test-bash parity xcu-diff oils-diff multishell austin yash zsh uutils
```bash
echo "all conformance suites complete"
```

<!--
Mesh note (run the matrix on a bigger box, leaving this machine free):
each suite target's body assumes it runs inside the bashy checkout. To dispatch
over `bashy dag --mesh suites.md HOST=<bigbox>`, give each target a `Host: ${HOST}`
line and prefix its body with the clone-and-build preamble from
../coreutils/examples/mesh-e2e-macos.md (the worker fetches bashy+sh from GitHub,
runs scripts/bootstrap-siblings.sh, sets up the external/bash-5.3 fixtures, then
runs the suite). Kept out of the default bodies so local `-j` runs stay simple;
wire it when the mesh path is set up.
-->
