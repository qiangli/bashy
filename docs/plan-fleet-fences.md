# Fleet fences and embed: implementation and evidence

Sprint #381, Story #1569, Story-ID e7aea0d9cfca.

The four fleet kinds — model, tool, agent and skill — support inline text
fences and file-backed `embed`. Both forms use the same row and expose the
same methods, effects and results. This records worker delivery, not S8
acceptance, independent-review approval or completion of manager gates.

## Implemented behavior

The methods protocol accepts an optional boolean `agentic` alongside
`effects` / `effect`. The interpreter rejects flagged calls outside an
explicit agentic region or marked function before invoking the host. An
unmarked function does not inherit that permission. All four fleet rows are
interpreted-only; lowering names the unsupported row and the interpreted route.

Model and tool rows resolve existing catalog definitions, including registered
nested definitions and defaults. A model resolves one eligible registered
agent binding; missing or ambiguous bindings fail explicitly. Tool methods
come from the catalog's commands and declared effects. Their analyzed metadata
is frozen and catalog drift is rejected. Model calls use governed `chat.Invoke`;
tool calls use the existing governed tool-command runner. Agentic calls carry
`exec,net,spend`, plus declared tool effects. Neither row supplies an unsafe
launch, premium-budget override or a direct interpreter-to-model shortcut.

The agent row accepts a registered binding reference, supported existing fleet
binding YAML, or a complete existing ycode/genie `kind: Harness` YAML document.
Binding references validate matching tool/model and supported instruction
fields; unsupported policy fields fail instead of being ignored. The binding
path uses `chat.Invoke` and lazily creates a script-scoped fleet clone after
admission, reuses it for serial calls, and removes it on teardown.

Full YAML goes through the strict ycode compiler and existing one-shot harness.
Imports, routing, permission policy, budgets and retry settings remain part of
the compiled definition; malformed/unknown fields and multiple documents fail.
Relative paths resolve beside the embedded definition or inline script. A full
YAML run creates a fresh durable session of the compiled roster, preserving
the authored agent reference rather than inventing a fleet parent. Script
teardown cancels and settles its session; durable usage and event evidence
remains. Provider attempts reserve budget and settle receipts through the spend
gate. A budget gate cannot silently change the authored YAML route.

`Embedded.CloseFunc` releases module resources. Declared `(string, error)`
methods preserve partial output and typed errors, including `.Error()`.
Caller-context cancellation remains a failure. The tour checks returned errors
before passing answers to `@ensure`.

Skills retain `run`, `verify` and `probe`. S7 owns task-target discovery and its
final integration; this delivery does not edit that implementation or the
`08-text-fences` chapter. The new matrix uses `verify` with a checked outcome.

## Recorded tour and focused tests

The sibling tour's `11-fleet` chapter now registers all eight kind/form
combinations in its existing `cases.tsv` inventory, with `.expected` transcripts.
Its `fleet` runner mode creates an isolated catalog and spend state, imports a
deterministic local tool/model/agent using catalog verbs, and invokes the tested
CLI through the real governed harness. The transport is a local Bashy script
returning `recorded answer`; no model API or billable request is involved.
Three additional transcripts exercise agentic denial, read-cap denial and an
`@ensure` mismatch. Fixture assertions require transport evidence and positive
metered tokens for executed calls, and no transport evidence for denied calls.
The bounded native smoke passed 11/11 (eight success, three expected refusal),
with zero skips; shell syntax and diff checks also passed. The CLI was built
with `GOMAXPROCS=2 go build -p=2 ./cmd/bashy`, not a full test suite. Run the bounded chapter without island provisioning:

```sh
TOUR_FILTER=fleet/ sh bashsharp-tour/check.sh /absolute/path/to/bashy
```

Worker evidence preceding the tour includes these focused selections:

- `bashy/internal/agentos`: `TestAgentFence*`, `TestModelFence*`,
  `TestToolFence*`, fence effect gates and row registration. Covers both forms,
  invalid definitions, named lowering refusals, effect/agentic denials,
  catalog drift, result/error preservation, clone reuse and cleanup, and
  real harness spend reservation/settlement with fixture transports.
- `bashy/cmd/bashy`: `TestYAMLAgentFenceCompilerRunner`, both complete YAML
  formats in both forms with fixture providers, CLI enforcement and judged output.
- `ycode/pkg/ycodecli`: `TestTextAgent*`, provider usage, hard-spend rejection,
  source mutation and cancellation.
- `sh`: focused agentic runner, typed-error, text-row, lowering and embedded
  lifecycle checks. The full suite is not implied by these selections.

The model/tool review fix is `bashy c0b7915` with `yoke f8d87af`;
full YAML delivery is `bashy 9737eee`, `ycode 3e8b22f`, `sh 7114c7e03`.
These identify implementation evidence, not a claim of final manager review.

## Known limits and outstanding gates

Opaque CLI transports have estimated token/cost accounting, not invented
provider-reported usage or a guaranteed hard spend bound. Hard limits refuse
turns whose pre-call bound cannot be established. Full YAML provider receipts
are settled when supplied; missing usage is marked estimated and uncertain
interrupted requests retain reservations for reconciliation.

Binding-based chat reads the process environment; script-local exported
environment projection is not claimed. The tour sets its catalog environment
before starting the CLI. The offline tour proves real CLI wiring and outcome
checks; it does not test an external provider or substitute for the complete
backend matrix. The full-YAML fixtures remain in the implementation tests.

Independent full-spec/backend review, final S7 integration, synchronized
published sibling pins, complete `make test`, the 86/86 Bash compatibility
gate, the Bash# harness, installed-binary smoke and remote acceptance remain
manager-owned. No push, broad gate completion or story closure is claimed here.
