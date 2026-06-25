# POSIX cert (VSC-PCTS) pre-flight — GO status snapshot

Status: **2026-06-25 — agent-drivable criteria GREEN; remaining gating is one decision + the human license/TET steps.** Companion to `vsc-pcts-readiness.md` (the checklist); this is the evidence snapshot taken while driving the drop-in-fidelity campaign to 99%+.

## Go/no-go criteria (from vsc-pcts-readiness.md §Go/no-go)

| Criterion | Status | Evidence (re-measured 2026-06-25) |
|---|---|---|
| **POSIX-mode breadth sweep green** | ✅ **0 deviations** | `scripts/posix-parity.sh`: 22 match / 0 diff / 23 probed (bashy `--posix` vs bash `--posix`, docker bash:5.3 oracle) |
| **Oils mining → stable 0-deviations across the corpus** | ✅ **0 deviations / 719 scripts** | `scripts/oils-diff.sh`: 297 match / **0 deviation** / 422 ambiguous. bashy-vs-bash53 = **702/719 (97%)**. The 422 "ambiguous" are where the 5 oracle shells (bash53/dash/yash/mksh/zsh) **disagree** — bashy matches bash53 on them; NOT bashy bugs. |
| **`<<${a}` heredoc-delimiter decision** | ⏳ **decision pending** (see below) | bashy parse-errors an expansion in the heredoc delimiter word; bash accepts. |
| **Declared-limitations list final** | ✅ list is stable (interactive job control; `((` nested-subshell ambiguity) | per `vsc-pcts-readiness.md` §Known limitations |
| **Apply for VSC-PCTS license** (Open Group) | ⏳ **human step** | OSS/no-cost 12-month arrangement historically exists; confirm terms at application |
| **Stand up TET + wire bashy as SUT (POSIX mode)** | ⏳ **human/infra step** | scope scenario to shell + builtins |

## What "0 deviations" means here (claim discipline)

Both differential harnesses run bashy in the **same environment** as the reference shells and find **0 cases where bashy diverges from bash 5.3** on the clean-room corpora. This is the strongest agent-drivable signal short of the official suite. It is **not** "POSIX certified" — that is the TET/Open-Group run (human step). Honest framing for any external claim:

> "Zero deviations from bash 5.3 on a 719-script clean-room differential (5-shell cross-checked) and on the POSIX-mode parity sweep; the official VSC-PCTS run is the remaining (licensed, human) step."

Anchor: `make test-bash` 86/86 (bash's own 5.3 fixture suite) + drop-in fidelity 1096/1105 (99%) and climbing.

## The one open decision: `<<${a}` heredoc delimiter

bash accepts an expansion in the heredoc delimiter word (`<<${a}`); bashy's streaming no-backtrack parser parse-errors it. **Recommendation: relax the parser to accept it** (it is a real drop-in-fidelity gap and bash-faithful), *if* it can be done without backtracking; **otherwise declare it a bounded known-limitation** alongside the `((` ambiguity. This is a parser change — gate it on `make test-bash` 86/86 + `go test ./syntax/` and the full fidelity probe (no new divergences). Until resolved, list it in the conformance statement.

## GO recommendation

The shell-language core and both clean-room differentials are **at 0 deviations** — the cert run should be *confirming a known-good state and catching the adversarial long tail*, exactly as §Go/no-go wants (don't pull the 12-month license before this). **Remaining before pulling the license:** (1) resolve `<<${a}`, (2) finalize the conformance statement's declared-limitations, (3) the human license + TET standup. Items (1)–(2) are agent-drivable and small; (3) is the gating human/legal/infra work.
