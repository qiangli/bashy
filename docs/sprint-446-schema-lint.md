# Sprint 446 S1, Part B: derived Cobra schemas and vocabulary report

Story: #2118 / 706ab589a6cf. Design: the dhnt umbrella's
`docs/sprint-446-master-execution-plan.md`, Part 3, and `docs/common-options.md`.

Implementation plan: pin coreutils to Part A's `2d851701aec0`, walk the actual
Cobra constructors and child trees, publish their flag metadata in front-door
MCP/help descriptions, and audit vocabulary without changing command behavior.
Run only the requested package tests and module builds, then commit locally.

`cobraSchemas` returns `tool.ArgSchema` per canonical command path. The walker
combines local, persistent and inherited flags, preserving local overrides and
pflag types/defaults. Hidden/deprecated flags follow `tool.SchemaFromFlags`'s
visibility rules. Hidden commands remain walkable. Build-tagged engine trees
register their constructors only where those commands are compiled in.
Hand-parsed commands receive no invented schema.

MCP descriptions contain the serialized path-to-schema map, also consumed by
skill/MCP help. The existing `args` transport remains intact. These flag-only
schemas are deliberately not passed to `RegisteredCommand.Schema`: its binding
injects defaults, lacks arbitrary operands, and rejects pflag duration/slice
values. Descriptive schemas are not a declaration of typed invocation support.
No command is executed to derive a Cobra schema.

`TestCommonOptionsVocabularyReport` logs `--dryrun`, non-bool `--quiet`, and
short-option audit candidates across `tool.Names()` and the Cobra trees.
Existing short letters are not automatically declared violations; the report
is the review inventory for the rule against adding new letters. Registry flag
sets are invocation-local, so their own help is collected in bounded child
processes. The two POSIX harness tools excluded by coreutils' `man` are also
excluded here and explicitly logged as unavailable. Custom help that does not
use pflag's declaration format cannot provide typed flag metadata to this audit.
Findings never fail the report and no spelling is rewritten.

Certification signals: `bashy_cert` selects certification implementations;
individual applets check `POSIXLY_CORRECT` presence. There is no common runtime
profile switch in `tool`. This delivery neither reads nor changes those signals.

Verification (2026-10-17, all with `GOWORK=off`; no gate was piped):

| Module | Command | Exit |
| --- | --- | --- |
| coreutils at `2d851701aec0` | `go test ./tool/...` | 0 |
| coreutils at `2d851701aec0` | `go build ./...` | 0 |
| bashy | `go test -v ./internal/agentos/` | 0 |
| bashy | `go build ./...` | 0 |

The final AgentOS package run passed in 158.907s. Its report visited 181
registered tools and 798 Cobra command paths. Findings include `dag --dryrun`,
duration-valued `coach --quiet`, string-valued `feed channel add --quiet`, and
existing short-option candidates. `posix-gate` and `posix-providers` were logged
as unavailable, as described above. The report itself passed.

The first AgentOS run exited 1 solely on the sprint skill-help golden, which
needed its new schema metadata recorded; the updated golden passed in the final
run. A preliminary coreutils build in Go's module cache exited 1 because the
module zip omits its nested `third_party/goawk` replacement. The clean coreutils
checkout at the exact requested commit contains that nested module; both required
checks passed there, without changes to coreutils. All other dependency versions
and replacements remain pinned; tidy only promoted the existing pflag version
from indirect to direct in addition to the requested coreutils bump.

Local transcripts: `/tmp/s446-agentos-final.log` (complete package run) and
`/tmp/s446-schema-lint.log` (earlier focused walker/lint run).
