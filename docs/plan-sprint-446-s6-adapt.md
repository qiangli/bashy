# Sprint 446 S6: first adapter implementation

Story #2123 / af9e53925904. Governing contracts are the umbrella's
`docs/common-options.md`, `docs/sprint-446-master-execution-plan.md` S6,
`docs/bashy-command-ecosystem-epic.md`, and the accepted decisions at the end of
`docs/wave0-joint-contract-review.md`.

Implementation plan: register one fleet kind with embedded YAML rows and the
existing generic CRUD/claim machinery; implement exact-prefix planning and
verbatim subprocess execution in yoke/pkg/adapt; mount adapt and adapter through
Bashy's front door, discovery, and schema traversal. Validate with fake child
processes plus real local Git, fleet CRUD/ring tests and focused Bashy discovery
checks. Run the gate, commit both repositories locally, and report integration
needs. Do not touch S3 source files or add service-specific Sprint 457 rows.

The sibling yoke link points at the shared umbrella main checkout. All yoke
changes therefore live in `/tmp/s446-adapt-yoke` on
`agent/weave-issue-201-adapt`. The assigned Bashy branch remains unchanged.
Testing uses `/tmp/s446-adapt.work`, a private copy of the umbrella workspace
with only Bashy and yoke redirected; no workspace or dependency pin edits are
committed.

## Deliberate acceptance limits

- S3 must connect `adapt.PrepareLens` in Bashy's `adapt.NewCmd` call after checking
  parser availability and applying its shared bounds/redaction policy. The hook
  is preflighted before execution and returns a streaming conversion writer.
  Post-execution conversion errors are diagnostic; foreign status still wins.
- Generic `--fields` projection awaits S3. Native mappings already work.
- S2 may replace the narrow common-option binder with its accepted shared helper;
  `Request` and the required `--` split remain stable. No common vocabulary changed.
- S19 must supply cursor binding/validation. `--since` fails 125 today even if a
  custom row declares it. There is no private cursor format.
- `--explain` reports declarations, omitting raw argv; full flag/operand explanation
  needs a foreign argument schema. It never executes or prints credential values.
- Listing is a provider API template, not a new common option. Its first form
  accepts exactly one non-option archive operand. Tar/unzip JSON awaits parser rows.
- Make `-n` is not safe enough for the no-mutation contract; rsync `-0` is not NUL
  output. Both are explicit refusals, not pretend support.
- Installed Docker/kubectl/rsync/apt mappings are covered as deterministic plans;
  no remote/account actions or package mutations are performed in tests. Git is
  additionally exercised against the native local executable.
- Conductor must integrate the yoke commit and update Bashy's yoke pin after that
  commit is published. This worker does not push or invent an unpublished pin.

These are gaps within the existing story, not new stories or claims of complete
S6/sprint acceptance.

## Local verification (2026-10-17)

- Final focused gate: **passed**, actual `bashy-gate-v2` verdict inspected in
  `/tmp/s446-final-gate.json`. Command:
  `go test ./pkg/adapt ./pkg/fleet -run '^(TestAdapter|TestCurated|TestCLITransparent|TestCLIRefusals|TestNative|TestNoInput|TestLongest|TestHelper)' -count=1 -timeout=60s`
  under `GOWORK=/tmp/s446-adapt.work`. Both packages emitted `ok` (92 output bytes).
  This includes CRUD, ring override, YAML normalization/null preservation, argv,
  stdin, exit 37, marker-based no-execution refusals and native Git behavior.
- Focused Bashy discovery/schema regression:
  `go test ./internal/agentos -run 'Test.*(Atlas|Command|Cobra)' -count=1`
  **passed** (`/tmp/s446-bashy-tests.log`).
- `GOOS=windows GOARCH=amd64 go vet ./pkg/adapt ./pkg/fleet` **passed**
  (`/tmp/s446-windows-vet.log`).
- Full yoke `scripts/crossvet.sh` cannot establish a complete verdict: its initial
  `go list` reports ambiguous `github.com/ugorji/go/codec` from both
  `github.com/ugorji/go v1.1.4` and `github.com/ugorji/go/codec v1.3.1`. The script
  continues and prints per-OS PASS lines, but that does not prove all packages
  were selected. `/tmp/s446-crossvet.log` retains the diagnostic.
- Broad fleet runs are not counted: one ended with `signal: interrupt`; the
  duplicate old run was explicitly stopped. The bounded focused gate above is
  the final candidate evidence, not an assertion that the entire fleet suite passed.

Yoke delivery commit: `9797313cb8e4e46dd62873eaf1a269fa31c54bf0`
(`agent/weave-issue-201-adapt`, `/tmp/s446-adapt-yoke`), with verified Sprint/Story
trailers. Integrate this commit before the Bashy front door. The crossvet script
ultimately returned 0 and printed windows/linux/darwin/aix PASS, but the package
discovery ambiguity above still prevents claiming a complete crossvet pass.

- Full AgentOS regression was attempted with `-timeout=180s`; it did not pass.
  It reported `TestDocumentedExternalCount` (documentation says 49, live registry
  says 50) and ultimately timed out in the fleet example lifecycle tests. See
  `/tmp/s446-agentos-all.log`. No full-suite pass is claimed.
- A final review added a forwarding-error regression: an executed child that
  succeeds still returns 0 when its stdout writer fails, rather than returning
  wrapper-only status 125. The final yoke gate includes this test.
- Final Bashy gate **passed** after updating both exact embedded-command-document
  expectations to include adapt: `go test ./skills ./internal/agentos -run
  'Test.*(Atlas|Command|Cobra)' -count=1 -timeout=90s`, recorded in
  `/tmp/s446-bashy-final-gate.json`. Its inspected verdict is `passed`; this is
  separate from the unsuccessful broad regression above.

## Same-holder continuation (2026-10-18)

S3 is accepted on published yoke a3e6733f. Coreutils S2/S7 is published at
02b24ff1db5d4128685727c42657cd55377b735c. Rebased the existing yoke branch
onto S3, correcting the unpublished first-phase commit's mistaken Qiang Li
attribution: 9797313cb8e4e46dd62873eaf1a269fa31c54bf0 became
78ca3220c001fc27ed9bd0fd836ebf4840222f5a, author and committer
codex-gpt6-astra <codex-gpt6-astra@users.noreply.github.com>. Published history
was not rewritten. Pin candidate 31b2cbf54035f039a27c845799a8690ad7c79751
updates both coreutils and its goawk module to the same published commit;
checksums were obtained from Go module downloads. This candidate was reported
to s446-manager early for Sprint 453's build dependency.

The continuation supersedes the earlier S3 integration gaps: adapt's
PrepareJSONLens uses lens.Route before exec and lens.Convert after capture.
S3 currently has no cmd: parser rows, so declared missing parsers still refuse
125 without executing. No private parser or service-specific row is introduced.
Generic fields now projects objects, arrays and NDJSON, with native mappings
preferred. Literal top-level keys, absent-key omission, exact JSON numbers,
8 MiB input/output limits, cancellation, drain-on-overflow and value-free
failure diagnostics are explicit. Child stdin, argv and executed status remain
transparent. Explicit stdout and child stderr are not secret-scrubbed. Since
remains refused until S19 supplies binding.

Bashy dc70eca2 was brought forward onto current main e46f80a0 in this run's
assigned branch. Both front-door and schema factories select PrepareJSONLens.
The committed dependency pin must wait for manager publication of yoke; local
checks use /tmp/s446-adapt-207.work (same private workspace as phase one, with
this Bashy workspace substituted). No pushes, broad suites, podman or ollama.

Continuation focused gates passed (inspected `bashy-gate-v2` verdicts):
- `/tmp/s446-s6-pin-gate.json`: adapt/fleet first-phase tests, 31.697s.
- `/tmp/s446-s6-lens-gate.json`: adapt/fleet plus projection, missing-parser
  no-exec, cancellation, precision, output bounds and CLI input/status checks,
  5.197s. Both packages emitted `ok`.
- `/tmp/s446-s6-bashy-gate.json`: skills and AgentOS discovery/schema checks,
  35.014s. Both packages emitted `ok`.

These are focused Dragon gates, not whole-package or crossvet acceptance.
Manager owns the claimed remote-host gate and publication. No yoke pin is
committed before publication. The yoke tree is `/tmp/s446-adapt-yoke`;
continuation SHA is reported in the delivery message rather than self-embedded.

### Recovered continuation verification (2026-10-18)

The repeated continuation request found the changes already committed and both
worktrees clean; no implementation was duplicated. Yoke main had advanced to
1137f7a890d5c3973e47732ef12869ed754aa015. Rebased the three unpublished
commits onto that published main without conflicts:

- Correctly attributed first phase: 78ca3220c001fc27ed9bd0fd836ebf4840222f5a
  became 1df3a26c71278a90b83637741c7ea542fb04f1c1 (original incorrectly
  attributed commit remains 9797313cb8e4e46dd62873eaf1a269fa31c54bf0).
- Consistent dependency pins: 31b2cbf54035f039a27c845799a8690ad7c79751
  became 78ef7f24edd05b072e02b4a30b7b9fa01b970db7.
- Lens integration: 00fc0d8a62482da872912738a2a39bde3d97e239 became
  68c9aaf80b8f2b03adf6e05f1758afe0fc955fb0.

All retain codex-gpt6-astra author/committer and the exact S6 trailers. The new
pin candidate was delivered directly to s446-manager for early publication and
Sprint 453 consumption. Bashy front-door commit d6ef480243812cd9d1be8c46b70384d916efb19c
already follows current Bashy main e46f80a0; no replay was needed. The manager
must publish yoke before the Bashy dependency pin can be committed.

Rechecked focused Dragon gates after the rebase; inspected both JSON verdicts:
`/tmp/s446-s6-recheck-yoke.json` passed (34.178s, adapt/fleet), and
`/tmp/s446-s6-recheck-bashy.json` passed (41.813s, skills/AgentOS). They use
GOWORK=/tmp/s446-adapt-207.work; these establish focused workspace compilation
and behavior, not standalone module or whole-package/crossvet acceptance.
At the final publication check origin/main remained 1137f7a8, so the Bashy
published yoke pin remains the outstanding manager-dependent integration step.
