# Sprint 561 A2: current-baseline results (reproduce + classify)

Story `78710ef53a1e`. Method per the revised manager brief (2026-11-03, no new
baseline run): read-only collection, no fixes. Lanes whose inputs are unchanged
since the migration-validated frozen tree reuse that evidence, labelled
unchanged-input reuse with run id/source — never presented as fresh results.
Licensed runs are blocked (no approval in A2); the licensed 493-TP shell arm
and the Profile D base run happen once, on the final candidate in A4. Raw logs
stay out of git. All commands below ran with `GOWORK=off`.

## 1. Baseline identity (verified locally 2026-11-04, Darwin/arm64)

- HEAD: `c3dd9e38` (`c3dd9e3822edf3692ad64d46176e48e1dab7be08`).
- Frozen-tree check: `git diff 5a4bd03f HEAD -- . ':!docs'` shows exactly one
  file, the root `KB.md` one-line kb-note rotation (another agent's kb sync;
  not a build input; left untouched). Everything else outside `docs/` is
  byte-identical to `5a4bd03f`. The `docs/` delta is plans and story cards
  only. The KB.md modification is reported here so the manager sees the full
  deviation; it is not mine to revert or commit.
- `go.mod` sha256 `bbd9d6b277694838e1b435d32760aa919a7e66d1dd9042e` and
  `go.sum` sha256 `5ee0b65c2d6dcb87f179ddeb97abcd6e736b17dba5d32b69fdeb370e`
  match `docs/s561/baseline-manifest.json` (A1, sibling worker, source commit
  `c3dd9e38`) exactly — verified with `sha256sum` in this checkout.
- Toolchain: `go version go1.27.2 darwin/arm64` — matches the manifest.
- Fixture lane inputs unchanged: `tools/bash53suite` tree
  `4c6670af6ce67afd2f635e6c4ff28dfb5999b161` matches the manifest
  (`bash53suite_last_change_commit f4add82e`); no diff vs `5a4bd03f` in
  `tools/bash53suite`, `scripts/test-bash-container.sh`, or `Makefile`.
  Fixture denominator in code (`bash53FixtureNames`, `tools/bash53suite/main_test.go`): 86.
- A1 inventory/manifest reused as the row definitions below (source:
  sibling issue-8 workspace `docs/s561/inventory.md` + `baseline-manifest.json`;
  A1 card `25dff6cbb152` still backlog at write time — manager acceptance of A1
  remains the precondition for A3/A4, but the rows themselves are fixed by the
  brief and the s561 plan and are sufficient for classification).

## 2. Baseline evidence table (one row per required A1 inventory entry)

| Lane | Verdict / reuse | Denominator | Hash / identity |
| --- | --- | --- | --- |
| GNU Bash 5.3 fixtures, authoritative (`make test-bash-container`) | Unchanged-input reuse: 86/86 Linux+macOS, CI run 41705603801 (S557 acceptance record, via A2 brief) | 86 fixtures, 86 executed | Tarball sha256 `0d5cd8695f...` (manifest); suite tree `4c6670af...` |
| Bash 5.3 fixtures, Windows | Unchanged-input reuse: 84/86, CI run 41708335900; the 2 are the known baseline `glob-test` and `printf` timestamp (via A2 brief) | 86 fixtures, 86 executed | Same inputs as above |
| VSC shell arm, Profile B, 493 TPs | BLOCKED — no operator approval, no current result (see §3 B1) | 493 shell TPs | Harness commit unknown on approved host |
| VSC base utilities, Profile D | BLOCKED — no operator approval, no current result (see §3 B2) | 102 mandatory targets + `gencat`; 7,831 raw base-and-shell rows before option filtering (A1 inventory) | Provider set empty by design; versions unknown until approved run |
| Standalone Go package tests (`GOWORK=off go test ./...`) | FRESH RUN 2026-11-04, exit 1: 29 ok / 3 FAIL / 11 no-test-files = 43/43 executed, no skips, no timeouts (§4 G5 + §4.1) | 43 packages | Log sha256 `906bca32c08bf987813f6cc45789c08295debbfc5b2f130a9efdd8c6d1844218` (859 lines, `/tmp/a2-gotest.log`, out of git) |
| Standalone host build (`GOWORK=off make build`) | Unchanged-input reuse of A1: passed | 2 product entries; `bash`/`bash.real`/`bashy` emitted, `bashy.real` absent by Makefile design | `bin/bash b0d19ed1...`, `bin/bash.real 87051878...`, `bin/bashy a5ec1740...` (manifest; binaries not present in this checkout, not rebuilt here) |
| Six-platform cross-build (`make dist`) | Deferred to A4 per A1 inventory | 12 binaries | — |
| Product-wrapper build contract | Unchanged-input reuse of A1 manifest | `cmd/bash`, `cmd/bashy`; wrapper sha256 `31f222f5...` | `scripts/go-product.sh`, `tools/productgo` |
| Advisory lanes (yash, XCU, Oils, Austin, multishell, zsh, uutils) | Out of scope — historical pointers only, not queued for A3 (§5) | Per-runner counts | See §5 |

Raw CI verdicts for runs 41705603801/41708335900 could not be re-fetched from
this host: no git remote is configured here and the runs are not visible under
`github.com/qiangli/bashy` via `gh`. The tallies above are brief-cited from the
S557 acceptance record, corroborated by unchanged inputs + in-tree 86-fixture
denominator + Makefile "86/86" closure notes — not by fresh execution.

## 3. Issue list (in scope of the Sprint 341 base profile + A1 inventory)

Classification: product defect | setup | applicability | manual review |
pre-existing/advisory. "Preserved patch" = exists on
`origin/s557-repair-stack-quarantined` (tip `8f20a0f8`) / `c3d3b95f` ancestry
(S557 story 6, `bf7624a5c2f2`: revert `56e622d6` restored the frozen tree and
kept fixes in prior commits for separate review). Both branches are ancestors
of HEAD; the frozen tree deliberately does not contain their working hunks.

Licensed Profile D dispositions (from the Sprint 560 plan's 3059/3063 receipt
summaries — receipts themselves live outside this host, consulted here only to
match causes, never imported as current verdicts):

- I1. 53 empty-symlink setup failures — setup. No patch (needs
  configuration/applicability disposition, not code). Next: disposition must be
  recorded by the approving authority before the A4 run; A3 only if a defect is
  confirmed.
- I2. `man` threshold failure — setup/applicability (`man` is mandatory and Go
  routed per coverage doc; threshold disposition pending). Next: same as I1.
- I3. Four EOF/editor failures — retained evidence exists; needs a bounded
  unchanged-PTY/locale diagnostic before proposing anything. Next: diagnostic
  in A3, no code yet.
- I4. Nine manual dispositions (INSPECT) — manual review. Next: human
  disposition closes each; unresolved items stay open into A4.
- I5. Retained full-D counts: 9,337 execution purposes with 4 FAIL, 54
  UNRESOLVED, 9 INSPECT (plan-560 §outcome; matches the October 6 archive on
  different product/toolchain/host identities — identical-cause claim
  explicitly disavowed there). Next: A4 reconciles against the final
  candidate; A3 triages the 4 FAIL first (see I9).

Module lanes (`go test ./...`, Gotreesitter/Yamux):

- I6. Known Gotreesitter/Yamux module-lane failures from the S557 record —
  detail lives in the unavailable acceptance record; in-repo pointers are
  plan-560 §3060 (Bonsai vet, Gotreesitter paths, Yoke platform failures; Yoke
  source-metadata mismatch, frozen pin vs `3fda1e1`/`89df` linked SHA) and §3063
  (Alpine ELF-loader mismatch, missing node/pnpm, Gotreesitter parity inputs,
  named-pipe prerequisites). Tentative: setup/pre-existing. No patch on the
  repair branches for these (branch hunks are shell/SQLite/task/metadata
  scoped). Compared against the §4 fresh run: no Gotreesitter/Yamux-attributed
  package failed there. Next: stays setup/pre-existing pending the S557 record
  check (G1); queue confirmed product defects to A3 only from new evidence.
- I7. Quarantined repair hunks (preserved patches, in ancestry, dropped from
  frozen tree by `56e622d6`): `2dd846f9` Desk fork-check serialization,
  `85855bdc` Sh pointer-metadata race, `68638904` shell signal + native worker,
  `b353e2df` migration-repair consolidation under acceptance owners,
  `7e206d9e` shell-dependency/terminal-test alignment, `69c1bfe9` hosted ELF
  build gates, `0096a97a` alias repair, plus tip-side `ed3902a8` Darwin
  descriptors, `3a765d7f` Darwin SIGPIPE, `bf471001`/`e85d23dc` SQLite sums,
  `b4506e33` task descriptor, `7a0c89e0` bash53 timeout evidence,
  `8f20a0f8` src-pin metadata regen. Next: A3 reviews each hunk against the
  fresh baseline before re-applying; nothing auto-merges.

Bash fixture lane:

- I8. Windows 84/86: `glob-test` baseline + `printf` timestamp — recorded in
  the S557 record as baseline (pre-existing/platform), not product defects.
  Mechanism notes in-tree: `glob-test` byte-transparency per LC_CTYPE
  (`Makefile` closure comment), `printf %(fmt)T` datetime support present
  (`docs/TODO.md`); Windows locale provisioning limits corroborated by repo kb
  (Big5-HKSCS not serviceable end-to-end). No patch. Next: A4 re-observes on
  the final candidate; not queued for A3.

Harness-kit applicability (Sprint 341 base scope routed via
`docs/posix-command-coverage.md`; scope doc itself, `vsc-pcts-harness-kit`,
is not on this host — §4 G3):

- I9. `find -exec`/`-ok` (~+40) and `env COMMAND` (~+16) NO-list deltas from
  the 2026-08-01 re-measure — applicability: recorded as data-justified
  candidates for a command-wrapper exception, pending decision (s561 plan §1).
  Not an A1 addition, not A3 code until decided. Next: decision before A4
  scoring.
- I10. `date +34` locale-dependent delta (`de_DE` provisioned on the measure
  host) — setup/untriaged. `at`/`diff`/`tail` per-tset cap artifacts —
  setup (truncation, excluded from delta reading). Next: A3 triage for `date`;
  caps stay setup with cause.
- I11. `sh` utility tset 7 fails (GA26 #5, `sh -s` #46, PATH_MAX #59,
  `-c`/`-s` stdin #67/#68, subshell-syntax exit #244, async-events #801;
  `vsc-pcts-run-status.md`) — untriaged real-bug vs declared-limitation vs
  environment. Next: A3 triage, then route (defect → fix, else limitation
  record).
- I12. `$!` after backgrounded subshell group returns `g<N>` sentinel instead
  of a PID (suite-found; verdicts unaffected) — suspected product defect,
  untriaged. Next: A3 confirms against current `sh` pin and fixes at the
  owning layer if reproduced.
- I13. Shell-scenario residual floor shared with certified Bash 5.3 under the
  identical harness (`shell_no12` {379 ctime flapper, 421, 450, 458, 520};
  `sh_12` 12 trap/signal fails; `sh_05` +2 only real delta) — pre-existing /
  declared-limitation bucket. Next: carry into the conformance statement's
  limitations; A4 confirms no movement.

Historical references only (different tree, not current verdicts):

- S557 story 233: licensed 493/493 shell run on `c3d3b95f` — NOT current main
  (HEAD `c3dd9e38`); cited for lineage, not reuse.
- 2026-08-08 shell-isolation milestone: 493/493, 0 blockers (Bashy `1e3a14be`,
  sh `beb0b465`, harness `10c2ab5d`) — superseded identities, engineering
  milestone only, no certification claim.

## 4. Gaps (named before running; fresh run only where no reuse exists)

- G1. The S557 acceptance record itself (umbrella `docs/todo/`, story
  `bf7624a5c2f2`) and its unchecked child rows are not on this host
  (no umbrella checkout; Cloudbox workspace dir empty). Issue detail above
  comes from the A2 brief + plan-560 summaries. Manager must verify against
  the record before A3 acceptance.
- G2. Sprint 560 receipts seq 3059/3063: no such cards in this repo (max local
  seq 3050); consulted via plan-560 §§3059/3063 summaries only.
- G3. Licensed harness checkout path/commit on the approved host unknown here
  (A1 cites inspected `0b2b185d...` without a path); Profile B/D provider
  versions and scored TP denominators unknown until the approved A4 run.
- G4. Raw CI logs for runs 41705603801/41708335900 not retrievable here (no
  remote configured; not visible via `gh` under the manifest repository).
- G5. Module-lane evidence: no reusable `go test ./...` verdict accessible for
  the frozen tree — filled by the fresh run below, the only fresh execution
  in A2.

Fresh run (gap G5): `GOWORK=off go test -timeout 20m ./...` on Darwin/arm64,
go1.27.2, source `c3dd9e38`. Full log in `/tmp/a2-gotest.log` (out of git).

Result: exit 1. 43/43 packages executed (29 `ok`, 3 `FAIL`, 11 no-test-files),
no skips, no `panic`, no per-package timeout. Slowest packages:
`internal/agentos` 702.9 s, `examples/desk` 436.1 s, `cmd/bashy` 240.4 s,
`internal/cli` 154.9 s — inside the 20 m per-package bound, but A4 scheduling
should note the F5 concern (agentos-scale packages near the 10 m preflight
budget; collect sharded durations, no silent budget growth).

### 4.1 Fresh-run failures (all current, all on the frozen tree)

- F1. `cmd/bashy` — `TestYAMLAgentFenceCompilerRunner`: opens
  `.../workspaces/ycode/examples/agent.yaml`, missing in this checkout
  (`polyglot_agent_test.go:64`). Classification: setup (absent sibling
  `ycode` workspace, expected layout `workspaces/ycode`). Not a product
  defect; no patch on the repair branches. Next: A3 runs it in a checkout
  with the documented sibling layout, or records the layout prerequisite.
- F2. `internal/agentos` — `TestCommandDocSourceNames`
  (`commanddocs_test.go:31`): `Names()` returns `desk` extra vs the want
  list. Classification: suspected product defect (registered-command vs
  documented-command contract drift), A3 triages; ownership note: agentos is
  outside the shell+coreutils base route, so a confirmed fix may belong to
  the boundary owner (A1b), not A3 — record the routing decision.
- F3. `internal/agentos` — `TestRunSignaledIsNonLossy` (`run_test.go:51`):
  a SIGKILL'd process does not report `signaled=true`. Classification:
  suspected product defect (Darwin/arm64 observed), A3 triages and fixes at
  the owning layer if reproduced.
- F4. `internal/agentos` — `TestSrcPinsMatchLinkedModules`
  (`src_mount_test.go:12`): yoke pin out of sync with linked module
  `v0.0.0-20261031184001-89df473c6035`. Classification: setup (source-pin
  metadata) — matches plan-560 §3060's frozen-vs-`89df` mismatch. Preserved
  patch: quarantined tip `8f20a0f8` (src-pin metadata regen). Next: A3
  reviews that hunk before re-applying; no uplift beyond the reviewed pin.
- F5. `tools/bash53suite` —
  `TestBashPPGateInputFixtureGetsExplicitSelector/0` (`main_test.go:296`):
  stdin fixture mode 0 `status=TIME`. Classification: suspected harness/lane
  setup (fixture timeout inside the gate tooling), A3 diagnoses; the 86/86
  gate verdict itself is unaffected (reused S557 evidence stands), but the
  gate tool changed, so A4 must re-observe this test on the final candidate.

F2/F3 are the only fresh-run items that smell like product defects; neither
is in the shell or coreutils base route. No Gotreesitter/Yamux-attributed
package failed in this run (I6 stays setup/pre-existing pending the S557
record check).

## 5. Out of scope (listed, not queued for A3)

- The 14 optional targets (`ar bg ctags ex fc fg jobs localedef make more nm
  strip talk vi`) and `docs/posix-command-coverage.md` external-provider
  mechanism — advisory by profile decision.
- Advisory lanes: yash/XCU/Oils/Austin/multishell differentials, zsh, uutils
  scoreboard (contained-runner only) — historical preflight pointers in
  `docs/posix-cert-preflight-status.md`, `docs/cert-dash-sh-findings.md`,
  `docs/public-claims-evidence.md`; inform, do not gate.
- Sprint 539 F-cards: F8/F6 sandbox+guard (product-island scope), F1 native
  git flags, F2/F3 fence-engine fixture (562 scope), F4 airgap table regen,
  F5 agentos durations (lane ops: collect sharded durations, no silent budget
  growth — relevant when A4 schedules gates), F0 ledger, F7 rc promotion
  history. None is a 341-base-profile non-pass.
- Cross-build lane: deferred to A4 by A1, not an A2 gap.

## 6. Acceptance accounting

Every required inventory row has an execution result or an explicit blocker
with cause + next action: fixtures Linux/macOS (reuse) ✓, fixtures Windows
(reuse, 2 known baseline) ✓, Profile B (blocked: approval) ✓, Profile D
(blocked: approval) ✓, `go test ./...` (fresh run exit 1: 29 ok / 3 FAIL / 11 no-test, findings F1–F5 in §4.1) ✓, host build (A1
reuse) ✓, cross-build (A4-deferred) ✓, wrapper contract (A1 reuse) ✓.
No fixes made in A2; no vertex asserts changed; no timeout raised to green.
