# Fleet fences and embed: implementation checkpoint

Sprint #381, Story #1569, Story-ID e7aea0d9cfca.

This is a partial implementation checkpoint, not S8 acceptance or release
approval. The required target remains all four fleet kinds (model, tool,
agent, skill), in inline and embedded forms, with existing definitions,
governed execution, measured spend, and the full regression gates.

## Delivered slice

The engine methods protocol accepts an optional boolean `agentic`, beside
`effects` / `effect`. The interpreter refuses a flagged call outside an
explicit agentic region or marked function before invoking the host. Unmarked
functions do not inherit permission. Flagged methods currently refuse lowering
by name with an interpreted route.

Host runtimes can release module-owned resources through `Embedded.CloseFunc`.
An explicitly declared `(string, error)` method preserves partial output and a
typed error, including `.Error()`. Caller-context cancellation remains a runner
failure and is not converted into a successful script exit.

The new agent row accepts a registered fleet binding name, or its existing
fleet YAML binding-reference fields (`name`, matching `tool` / `model`, and
`instruction.content`). It does not redefine the fleet format or change the
parent binding. Other policy-bearing fields are refused instead of ignored.
A clone is minted lazily after the interpreter's admission checks, reused for
serial calls in that module, and removed on module teardown. The call uses
`chat.Invoke`, the same one-shot harness as `chat --agent --instruction`, with
no unsafe-launch, premium-budget, dry-run, or argv override.

```bash
~~~agent as searcher
registered-search-binding
~~~

@guard(effects: "exec,net,spend")
@ensure('test -n "$RESULT"')
agentic func search() string {
    answer, err := searcher.run("Find the requested evidence")
    if err != nil { panic(err); }
    return answer
}
agentic { answer := search(); echo "$answer"; }
```

`embed agent "./binding.yaml" as searcher` uses exactly the same row. The
file may contain the binding name or its supported existing YAML fields.
This example needs a real registered binding; it is not the required recorded
fixture tour chapter, and is not evidence of complete S8 acceptance.

## Evidence from the isolated worker

Engine protocol commit: `f184bb2ae`.
Engine lifecycle/error/lowering commit: `f17849216`.

Bounded commands run locally:

```text
sh:
go test -tags full ./interp ./lower ./polyglot -run '^(TestBashPPAgentic|TestBashPPRunnerFence|TestBashPPForeignDeclaredStringError|TestBashPPPredeclaredErrorInterface|TestBashPPErrorInterfaceTypedNil|TestBashPPTextRow|TestBashPPForeignEffectGate|TestBashPPEmbedRunnerFence|TestLowerTextRowAndRunnerFence|TestLowerRunnerFenceRefusals|TestAgenticForeignLoweringRefusesByName|TestParseMethods|TestEmbeddedModuleClose)' -count=1 -timeout=120s
ok mvdan.cc/sh/v3/interp
ok mvdan.cc/sh/v3/lower
ok mvdan.cc/sh/v3/polyglot

bashy:
go test ./internal/agentos -run '^(TestAgentFence|TestFenceEffectGate|TestDagMethods|TestTextRowsRegistered)' -count=1 -timeout=120s
ok github.com/qiangli/bashy/internal/agentos
```

The agent tests cover inline/embed execution parity, missing/invalid
definitions, named lowering refusal, agentic denial, read-cap denial,
permitted `exec,net,spend`, ensured output, clone reuse and deletion after
success/failure/nonzero status/cancellation, and interpreter teardown on
cancellation. The metering test substitutes only chat's external process
transport: real catalog resolution, launch governance, reservation and
settlement run. It reads the actual meter file and requires positive token and
cost counters. A hard-spend policy then denies the opaque turn before transport.
These are the existing harness's explicitly estimated accounting figures,
not asserted provider-reported usage or a fabricated hard spend bound.

Raw failure excerpts retained from development (all covered by the final
passing selection):

```text
replacement directory ../gotreesitter does not exist
TestBashPPRunnerFenceAgentic/command_denied:
  "searcher.run": executable file not found in $PATH
TestAgentFenceInlineEmbedGovernance/read_denied:
  calls=1 out="recorded answer\n" diag="" err=<nil>
TestAgentFenceHarnessMetersSpend:
  tool "fixture" cannot select a model ... launch template has no {model}
TestBashPPForeignDeclaredStringError:
  BASHPP-EINTERFACE-VALUE: promoted interface method Error has no interface storage
TestBashPPForeignDeclaredStringError:
  type polyglot.error has no method Error
TestAgentFenceInterpreterCancellationCleanup:
  did not cancel a started call ... err=<nil>
TestLowerTextRowAndRunnerFence:
  generated source lacks ... Result: ""}
```

The missing private sibling was cloned locally. The command fixture was
corrected to call `searcher.run()`. The guard test now installs the same fence
seam as the CLI and uses the named effect-cap argument. The fixture launch
now declares model selection. The error carrier and cancellation findings
were fixed in the interpreter. Unmarked text-row literals retain their previous
lowered spelling. No denial or cancellation assertions were removed.

## Remaining acceptance and integration blockers

- Model and tool rows are not implemented. The existing chat seam takes a
  tool/model binding; a model-only row must integrate the existing routing
  path without choosing an arbitrary tool. Fleet vendor `ToolCommand` records
  currently have no effect field; resolve the existing authoritative effect
  discovery/runner contract before exposing those methods.
- Full ycode/genie `kind: Harness` YAML is not accepted by this slice. It needs
  the strict ycode compiler and a governed launch/configuration seam preserving
  its routing, authorization and policy. Do not parse it as a fleet binding
  or treat the YAML as a prompt. Any missing direct-model/spec metering
  prerequisite needs a linked owner story before its implementation.
- Script-local exported environment changes are not explicitly projected into
  this direct chat call; chat currently reads its process environment. Review
  that seam with catalog scoping when completing inline spec integration.
- S7 owns skill target methods. Its implementation and final skill/embed
  integration must be consumed through the manager; the existing skill row
  file has not been edited here.
- The complete eight-combination tour, provider fixture matrix, and full
  model/spec accounting proof remain open.
- No complete `make test`, 86/86 Bash compatibility gate, Bash# harness gate,
  installed-binary smoke, remote independent gate, push or story closure is
  claimed. The manager owns those gates and integration.

The next integration step is to review these commits, resolve the model/tool
and strict-spec seams above, consume S7, then run the full acceptance matrix on
the manager's claimed test host. Keep S8 open until those checks pass.
