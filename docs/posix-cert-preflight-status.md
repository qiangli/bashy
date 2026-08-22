# POSIX cert (VSC-PCTS) pre-flight — GO status snapshot

Status: **historical GO pre-flight snapshot, superseded by the 2026-08-08 shell
milestone.** Agent-drivable pre-flight is complete; the licensed 493-TP shell
isolation arm now has 493 certification PASS-group outcomes and zero blockers.
The 116 utility sets and complete 117-set formal run remain. Results are
published in `vsc-pcts-run-status.md` under the consent grants.

## Go/no-go criteria (from vsc-pcts-readiness.md §Go/no-go)

| Criterion | Status | Evidence (re-measured 2026-06-25) |
|---|---|---|
| **POSIX-mode breadth sweep green** | ✅ **0 deviations** | `scripts/posix-parity.sh`: 22 match / 0 diff / 23 probed (bashy `--posix` vs bash `--posix`, docker bash:5.3 oracle) |
| **Oils mining → stable 0-deviations across the corpus** | ✅ **0 deviations / 719 scripts** | `scripts/oils-diff.sh`: 297 match / **0 deviation** / 422 ambiguous. bashy-vs-bash53 = **702/719 (97%)**. The 422 "ambiguous" are where the 5 oracle shells (bash53/dash/yash/mksh/zsh) **disagree** — bashy matches bash53 on them; NOT bashy bugs. |
| **Broadened 10-shell panel → 0 deviations** | ✅ **0 deviations / both panels** | `scripts/multishell-diff.sh` (43-case clean-room POSIX corpus) across **10 shells**: strict-POSIX (dash, ash/busybox, **posh**, yash) + feature-rich (bash 5.3, bash 5.2, zsh, **ksh93**, mksh, loksh). Two images: Alpine `bash:5.3` (40 match / 0 dev / 3 amb) + Debian (39 match / 0 dev / 4 amb). bashy match: bash 100%, ash 100%, posh **93%**, dash/yash/mksh/loksh 95%, ksh93/zsh 97%. The few AMBIGs are where the shells disagree among themselves; bashy sides with the majority. posh (deliberately rejects bashisms) + ksh93 (feature-rich reference) added 2026-06-25 to widen the oracle before the cert. |
| **`<<${a}` heredoc-delimiter decision** | ✅ **DECIDED — declared limitation** (see below) | bashy parse-errors an expansion in the heredoc delimiter word; bash treats it as a *literal* delimiter + EOF-warns. |
| **Declared-limitations list final** | ✅ list is stable (interactive job control; `((` nested-subshell ambiguity) | per `vsc-pcts-readiness.md` §Known limitations |
| **Apply for VSC-PCTS license** (Open Group) | ✅ **LICENSED + SUITE IN HAND**; the shell-scenario run is complete (results held privately — see `vsc-pcts-run-status.md`) | VSC-PCTS2016 OSS v1.4 agreement, signed 2026-06-28, countersigned by The Open Group 2026-07-03 (ticket **#279890**); suite downloaded by 2026-07-04. Agreement held privately (personal data — never committed). The 12-month clock started at the suite-access email (early July 2026 → expires ~July 2027), with a 10-day destroy obligation after. Binding terms — suite not redistributable; **publishing results CONSENT GRANTED 2026-07-16 (ticket #280298) for shell-utility tests, EXTENDED 2026-07-29 to the utility test sets; both for conformance-work purposes only; no "certified"/trademark claim regardless** — see `bashy-v1.0.0-readiness.md` §License terms + `dhnt/docs/legal/pcts-publication-consent-granted.md`. |
| **Stand up TET + wire bashy as SUT (POSIX mode)** | ✅ **493/493 shell certification pass group, zero blockers/caps** on 2026-08-08; historical utilities sweep also published | tag `vsc-pcts-posix-shell-2026-08-08`; full 117-set run still pending |

## What "0 deviations" means here (claim discipline)

Both differential harnesses run bashy in the **same environment** as the reference shells and find **0 cases where bashy diverges from bash 5.3** on the clean-room corpora. This is the strongest agent-drivable signal short of the official suite. It is **not** "POSIX certified" — that is the TET/Open-Group run (human step). Honest framing for any external claim:

> "Zero deviations from bash 5.3 on a 719-script clean-room differential,
> cross-checked against a 10-shell panel; the licensed 493-TP POSIX shell
> isolation milestone has zero certification blockers. The complete Shell and
> Utilities profile and formal Open Group submission remain pending."

Anchor: `make test-bash` 86/86 (bash's own 5.3 fixture suite) + drop-in fidelity 1096/1105 (99%) and climbing.

## RESOLVED decision: `<<${a}` heredoc delimiter → declared limitation

Investigated 2026-06-25. bash does **not** expand `${a}` in a heredoc delimiter — it treats it as a **literal** delimiter (the close-word is the bytes `${a}`) and emits `warning: here-document … delimited by end-of-file` if no matching line appears. bashy parse-errors it (`syntax/parser.go:1437` "expansions not allowed in heredoc words") and recovers. **Decision: declare it a known-limitation for the cert run, do NOT relax the parser pre-cert.** Rationale: (a) it is a *deliberate, tested* parser behavior — 6 `parser_test.go` cases assert the error — and matches upstream mvdan/sh, so relaxing diverges from upstream and rewrites tests; (b) it is rare in conformance corpora; (c) bashy errors **loudly and recovers** (no silent misbehavior). Relaxing to bash's literal-delimiter + EOF-warning semantics is a tracked **post-cert** follow-up (probe-gated, localized to the heredoc-delimiter lexer).

## Final declared-limitations list (for the conformance statement)

1. **Interactive terminal `fg` re-attach + Ctrl-Z `SIGTSTP` suspend** — the in-process runner doesn't re-attach stdio to a controlling terminal. Scriptable JC (`wait`/`$!`/`kill %n`/`jobs`/`bg`) and detached-job management (real PIDs via `coreutils/pkg/jobs`) **work**; only terminal-handoff `fg` + Ctrl-Z don't.
2. **`((` arithmetic-vs-nested-subshell ambiguity** — `((cmd)||(cmd))` needs spaces (streaming no-backtrack parser).
3. **Go-runtime fd footprint** (the 2 `/proc`-fd-census cases: `redirect__019`, `redirect__027`) — these do `ls /proc/$$/fd` and assert the host *process* has no stray fd 6/7 open. bashy's actual redirection **semantics** (`>&`, `exec n>`, "Bad file descriptor", fd dup/close) match bash exactly; the only difference is that the **Go runtime** — the scheduler/async-I/O machinery linked into *every* Go binary — opens low file descriptors for its own internal use: the network poller's `epoll` + `eventfd` (fds 5/6, held for the whole process lifetime) and the GOMAXPROCS cgroup-quota probe (`/sys/fs/cgroup/cpu.max`, fd 3). A freshly-forked C shell (bash) or CPython (Oils' OSH) has none of these, so its command-substitution capture pipe lands on a low free fd (3); bashy's lands on fd 7 — exactly where the census looks. **This is a property of the Go runtime, not the shell language.** `/proc` is Linux-specific and **not POSIX**, so VSC-PCTS does not exercise it; and the test is environment-fragile even for bash (Oils' own comment: *"descriptor 8 is open on Github Actions"*). Excluded from the drop-in fidelity probe with a transparent count; OSH passes only because CPython has a clean low-fd table. Declared here for the conformance statement.
4. **`<<${a}` expansion-shaped heredoc delimiter** — parse-errors instead of treating as a literal delimiter (above).
5. **stdout/stderr flush ordering** in a few mixed-stream cases (buffering).

## GO recommendation — agent-drivable criteria are GREEN

Both clean-room differentials are **at 0 deviations**, the `<<${a}` decision is
**made**, and the 493-TP shell-isolation arm is in the certification PASS group
with zero blockers. The active primary corrective arm is Profile B: Bashy `sh`
with frozen GNU/system providers and no Bashy Go multicall. Profiles C/D
separately exercise the Go-utility provider over the same 116-set/8,844-TP
inventory. See `posix-command-coverage.md`; the complete 117-set formal run and
submission/claim discipline remain separate, and no "POSIX certified" claim is
made.

## 2026-08-02 preflight re-run (workstream 1, single-host, serial)

Scope: re-confirm the two agent-drivable pre-flight gates named in this
doc and in `posix-cert-handoff-runbook.md` §Pre-flight gate, on this host,
without chunking/distribution. No `../sh`/`../coreutils` sibling edits, no
DKS/O3 work; this run does not stand in for distributed/chunked evidence
(that equivalence claim is tracked separately in
`workstream-a-equivalence.md`).

Environment: macOS (Darwin 25.5.0, arm64, host `dragon`), `go1.26.5`,
repo at `fc4b3c5` (branch `agent/weave-issue-34`). `PATH` has no `sh`
wrapper-shim ahead of `/bin/sh` (the documented local-env gotcha does not
apply here). `external/bash-5.3` resolves via the gitignored symlink to a
local Bash 5.3 source checkout's `tests/` dir.

### `make test-bash` — authoritative single-SUT serial gate

```sh
make test-bash
```

Started 2026-08-02T20:54:37Z, wall time **127.01s** (`real 127.01 / user
10.70 / sys 11.29`, per `/usr/bin/time -p`).

**Result: 86 passed, 0 failed, 0 skipped, 0 timed out — no regression**
against the 86/86 anchor cited above and in `docs/TODO.md`. Slowest
fixtures: `jobs` (62.3s), `trap` (17.1s), `func` (5.1s), `read` (4.8s) —
consistent with prior runs; no new slow or flaky fixture observed.

### `make test-yash` — POSIX (-p) scoreboard

The steward checkout already had the gitignored GPL Yash corpus materialized,
so the authoritative serial scoreboard was run there rather than treating an
isolated weave clone's missing ignored directory as a product blocker:

```sh
make test-yash
```

**Result: 1,832 passed, 0 failed — 100%, with zero Bashy-specific
failures.** Bashy and the Bash oracle each measured 1,832 cases and skipped
the same 33 cases. The previous 96% result and its proposed
`error-p`/`alias-p` worklists are superseded by this measurement; they are not
open implementation streams unless a later clean rerun reproduces them.

The corpus remains a gitignored runtime prerequisite rather than vendored
source. A fresh weave workspace must materialize or copy `.yash-tests/` before
running this gate. The umbrella campaign context lives one repository above at
`../docs/bashy-three-workstreams.md`; its absence inside an isolated bashy-only
clone is expected and is not a blocker.

### Current preflight conclusion

Both agent-drivable serial gates were green on 2026-08-02: Bash 5.3 was 86/86
and Yash POSIX mode was 1,832/1,832 with no Bashy-specific differential. At
that snapshot the frontier began with the `$!` TET-context defect. That shell
front is now closed by the 2026-08-08 493/493 shell milestone; the current
frontier is the 116-set/8,844-TP Commands and Utilities campaign. The assembled
provider shell rerun is its regression gate; the complete 117-set formal run
follows.
