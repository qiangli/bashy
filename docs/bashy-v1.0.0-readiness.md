# bashy v1.0.0 — release readiness

Status: **draft, 2026-07-04.** The single source of truth for what "v1.0.0"
means and the gate to tag it. Ties the three release workstreams together:
**(1) POSIX cert · (2) benchmark-driven agentic uplift · (3) release prep.**
Update the checkboxes as items land; do not tag until §Gate is fully green.

> Naming: **bashy** = the pure-Go Bash 5.3 drop-in (`cmd/bash`) + AgentOS system
> shell (`cmd/bashy`). v1.0.0 covers both binaries built from this repo.

## What v1.0.0 asserts (the promise)

bashy v1.0.0 is a **drop-in Bash 5.3 you can trust as a foundation** — the
tier-1 userland keystone — with an agentic superset that measurably helps
agents, and **zero non-permissive code compiled in**. Three claims, each with
strict discipline (§Claim discipline):

1. **Bash 5.3 compatible + POSIX conformant** — runs Bash 5.3 scripts/sessions
   with matching semantics; POSIX-mode passes the conformance frontier.
2. **Agentic uplift** — agents complete shell-heavy tasks more reliably / with
   fewer doomed retries inside `bashy` than in stock Bash 5.3 + coreutils.
3. **Self-contained + permissive** — one pure-Go binary, cross-platform; every
   engine/tool is download+exec (never bundled non-permissive), self-provisioned.

## Current evidence (re-measure before tagging — snapshots go stale)

| Signal | Current | Source |
|---|---|---|
| Bash 5.3 fixture suite | **86/86** (100% of measured) | `make test-bash` (serial, the hard gate) |
| yash POSIX `-p` suite | **96%** (best of 10-shell panel, tied mksh) | `bashy dag dag.md yash` |
| Drop-in fidelity | **99%** (1096/1105) | drop-in-fidelity campaign |
| Clean-room differential | **0 deviations / 719 scripts** vs bash 5.3 | `scripts/oils-diff.sh` |
| 10-shell panel | **0 deviations** (strict-POSIX + feature-rich) | `scripts/multishell-diff.sh` |
| POSIX-mode parity sweep | **0 deviations / 23 probed** | `scripts/posix-parity.sh` |
| Agentic uplift | **not yet demonstrated** — last slice 12/12 both arms (non-discriminating) | `docs/agent-shell-eval/` |

## Workstream 1 — POSIX cert (VSC-PCTS)

- [x] Agent-drivable criteria green (differentials, parity, 10-shell panel).
- [x] License application submitted — Open Group **#279890** (2026-06-28).
- [x] Declared-limitations list final (§Declared limitations).
- [x] TET harness skeleton ready (`scripts/vsc-tet-build.sh`).
- [x] **Open Group countersignature received** — VSC-PCTS2016 Test Suite
      Time-Limited License Agreement v1.4.OSS, signed Qiang Li (Maintainer,
      bashy) 2026-06-28, countersigned Andrew Josey (VP Standards &
      Certification, The Open Group) **2026-07-03**. Licensed Product per
      Schedule 1 = bashy, `https://github.com/qiangli/bashy`.
- [x] **Suite access + tarball received** — suite in hand by **2026-07-04**
      (VSC-PCTS2016-3.1 + TET3.6-lite + VSXgen4.11). The suite-access email is
      what **started the 12-month clock** (§License terms) — see that section for
      the term/destroy dates.
- [x] **TET run against the licensed suite; SUT = bashy in `--posix` mode.**
      The shell-scenario campaign is complete. *(Results are not recorded in this
      public repo — see `docs/vsc-pcts-run-status.md` for why. The durable run
      record, harness runbook, and campaign plan are held privately.)*
- [ ] **Utilities fail-delta campaign** ← the active workstream. Two known
      engineering fronts, both stateable without suite results because they are
      our own code: **(a)** regex/text-engine depth — `pkg/bre`, driving `sed`
      and `grep`; **(b)** the NO-list lift — `find -exec`/`-ok`, `xargs`,
      `env COMMAND`, `nice`, `nohup`, which exec a utility directly by argv and
      so do *not* breach "never shell out"; one shared argv-runner routed through
      Tier-1 dispatch serves them all. Then a per-command long tail.
- [ ] Final scored run (default timers).
- [ ] Conformance statement finalized (`docs/conformance-statement.md`).
- [ ] **Open Group submission** — the human step (journal + conformance
      statement + declared limitations). Not agent-completable.

The bar for this campaign is **zero fails that certified bash 5.3 / GNU do not
also share** — not literal 100% PASS. Chasing raw GNU parity is a moving target,
and PCTS charges some behaviors that exceed POSIX.

**Cert is NOT a v1.0.0 blocker.** v1.0.0 ships with cert *pending* and the
honest differential claim; certification is a follow-on badge (§Claim
discipline).

### License terms (binding — read before touching the suite or publishing a number)

The executed agreement is **not in this repo** and must not be committed: it is
a signed two-party contract carrying the maintainer's home address, phone,
email, and both parties' signatures, and this repo is public. It lives in the
maintainer's private storage. The terms that constrain engineering work:

- **The Test Suites are not redistributable.** No use/copy/modify/distribute
  outside the license, no sublicense/rent/lease, no reverse-engineering, no
  export. **Never commit the tarball, its extracted tree, or TET binaries** —
  keep them outside the repo or behind a `.gitignore` entry, the same discipline
  as the gitignored `external/bash-5.3` symlink.
- **No publishing results, no "passed" claim, without prior written consent** of
  The Open Group (§1). This binds *even after a clean run* — a green PCTS score
  is not publishable on its own.

  ✅ **CONSENT GRANTED 2026-07-16 (ticket #280298) — SCOPED.** Between 2026-07-04
  and 2026-07-11 this repo published PCTS tallies + assertion identifiers without
  consent; on noticing we removed them and asked The Open Group. They have now
  granted permission to publish results **"for the purposes of conformance work,
  limited to the relevant tests related to the shell utility, other existing
  requirements unchanged."** So: **shell-scenario results MAY be published**
  (`shell_no12`, `sh_12`). **The coreutils/utilities sweep results remain
  withheld** — "the shell utility" does not clearly cover the utility programs;
  a scope follow-up on the ticket is needed before publishing those. And the
  standing bars are **unchanged**: never a "certified" / "POSIX certified" /
  "passes the Open Group suite" claim, no Open Group mark/badge, and the suite is
  never redistributed. Our own measurements (bash-5.3 fixtures, yash, the
  differential + 10-shell panels, POSIX-mode parity) were always unaffected.
- **No certification-program trademarks.** The license grants zero rights to the
  Open Group cert marks/badges.
- **Term: 12 months** from the email telling us how to obtain the suites (not
  from countersignature). That email landed in **early July 2026** — the suite
  was in hand by 2026-07-04 — so the clock is **running**, expiring **~July
  2027**. (Fill in the exact email date; it sets the deadline.) Within **10
  days** of the term ending we must **destroy the Test Suites** unless The Open
  Group says in writing we may retain them.
  The suite may also carry a disabling device that trips at expiry — do not
  tamper with it, and don't leave data you care about only inside a suite tree.
- **Feedback is assigned to The Open Group** (§4 Rights In Data): any data,
  suggestions, or written material we send them about running the suite becomes
  theirs. Send bug reports knowing that.
- Use is limited to **testing bashy for conformance to IEEE Std 1003.1-2016**.

## Workstream 2 — benchmark-driven agentic uplift

Goal: **demonstrate + improve** the agentic uplift (claim 2) before v1.0.0.

- Harness: `eval/agent-shell/run-container-task.sh` — container-enforced shell
  selection via `bashy podman`, host agents, independent verification. Two arms:
  `bashy-current` vs `gnu-bash53`. Fleet: agy · claude · codex (subscription,
  watch rate limits) + opencode · aider (API budget — approval-gated).
- **Problem to fix:** tasks must *discriminate*. A round where both arms pass
  100% (2026-07-03) proves nothing. v1.0.0 tasks must stress the features only
  bashy has, and mine `coreutils-gap-log.md`.
- **The loop (TDD-at-fleet-scale):** design discriminating tasks → run the
  fleet in both arms → every bashy-loss or gap becomes a fix + a regression
  test → re-run; the green suite is the next round's guard.
- [ ] Discriminating task set defined (advisor-on-failure, dry-run gating,
      cwd/space advisor, `bashy check` closure, structured `run` envelopes,
      graph verbs, gap-log items).
- [ ] ≥1 round showing a measurable bashy advantage (reliability / fewer doomed
      retries / fewer tokens) on the discriminating set.
- [ ] Every gap surfaced is fixed or filed as a declared limitation.

**Operating rules (user-set):** never schedule benchmark runs without prior
approval; prefer the subscription fleet (agy/claude/codex) and watch quota/
rate-limits; use opencode/aider (DeepSeek/Kimi API) only on explicit approval
and watch token budget.

## Workstream 3 — release prep

- [ ] `docs/conformance-statement.md` finalized with claim discipline.
- [ ] CHANGELOG / release notes for v1.0.0.
- [ ] Version stamp: `-X …/internal/cli.bashVersion=5.3.0(1)-bashy-v1.0.0`.
- [ ] `THIRD_PARTY_LICENSES` current; supply-chain policy re-checked.
- [ ] Cross-compile all 6 platforms clean (`make dist`), lean sizes sane.
- [ ] Release CI green on ubuntu/macos/**windows** (unit + e2e dispatch).
- [ ] README + `bashy commands` surface accurate (tiers 1–6 verbs, account).

## The Gate (do not tag v1.0.0 until ALL green)

1. **`make test-bash` = 86/86, serial, clean PATH** — the mandatory hard gate
   before ANY bashy tag (emphatic user rule; I've skipped it before — never
   again). Re-run, don't trust a quoted count.
2. **yash POSIX suite ≥ 96%** (no regression from the headline).
3. **Differential + 10-shell panel = 0 deviations** (re-measured).
4. **Agentic uplift demonstrated** on the discriminating benchmark set (claim 2
   is real, not aspirational) — or claim 2 is softened in the release wording.
5. **All 6 platforms cross-build; CI green incl. Windows.**
6. **Claim discipline honored** in every external string (§Claim discipline).

## Declared limitations (carry into the conformance statement + release notes)

- **Interactive job control** — `fg`/`bg`/`jobs` can't own the controlling
  terminal (subshells are goroutines); real-PID job control is the supported
  path via `bashy jobs|fg|bg|kill` on the shared registry.
- **`((` nested-subshell ambiguity** — a documented parser edge.
- **`<<${a}` heredoc delimiter** — bashy parse-errors an expansion in the
  heredoc delimiter word (matches upstream mvdan/sh; loud + recovers); bash
  treats it as a literal delimiter + EOF-warns. Post-cert follow-up.

## Claim discipline (external wording)

- ✅ "Zero deviations from Bash 5.3 across a 719-script clean-room differential,
  cross-checked against a 10-shell panel; 86/86 on Bash's own 5.3 fixture suite;
  yash POSIX suite 96% (best of the panel)."
- ❌ Do **not** say "100% POSIX compatible" or "POSIX certified" until Open
  Group certification is actually granted. 86/86 is *our measured fixtures*, not
  total POSIX fidelity.
- ❌ Do **not** publish a VSC-PCTS score, pass rate, or any "passes the Open
  Group suite" claim outside the 2026-07-16 scoped consent. Shell-scenario
  results may be published only within that grant; utilities results remain
  withheld pending scope follow-up. The differential/yash/fixture numbers above
  are ours to publish.
- Agentic uplift: state it only with the benchmark evidence behind it (arm,
  task count, verifier, sample) — self-reports are not evidence.

## Related

- `docs/TODO.md` — live PASS/FAIL/SKIP headline (read first).
- `docs/posix-cert-preflight-status.md` · `docs/posix-cert-handoff-runbook.md`
  · `docs/fidelity-ceiling-assessment.md` — cert workstream.
- `docs/plan-agent-shell-evaluation-sprint.md` · `docs/agent-shell-eval/` —
  benchmark workstream.
- `docs/cross-shell-conformance-baseline.md` · `docs/yash-conformance-gap.md`
  — the POSIX frontier.
