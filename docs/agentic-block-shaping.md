# Bash# agentic blocks and Software Shaping

Sprint 564 spike `167b0f281c49`, source inspection dated 2026-11-05.
This document records composition and follow-up options. It adds no language,
runtime behavior, automatic repair, or rollout requirement.

## Two existing mechanisms

An `agentic { … }` block opts its statements into Bash#'s agentic execution
scope. A marked shell/typed function requires that scope; an ordinary helper
runs with assistance off unless it explicitly enters a block itself. The block
uses the current shell, with normal arguments, variables, streams and status.
It does not itself call a model or grant network, spending or write authority.

Software Shaping changes an application's source through a different explicit
lifecycle: request/import, isolated candidate, inspection, trusted verification,
preview, approval, scoped activation, history and rollback. `bashy shape` calls
the same service as the web UI. Its authorizations and verifier remain necessary
whether a command is written inside or outside an agentic block. The shaping
service does not currently consume `HandlerContext.Agentic` as a permission or
an automatic self-modification trigger.

| Intended user/workflow | Existing mechanism | Boundary |
| --- | --- | --- |
| A person changes a web app using ordinary words | Web shaping UI | Review and verification precede activation for that principal |
| An agent or assisted developer scripts that lifecycle | Explicit `bashy shape` commands | Same identity, tracking and gates; an agentic block supplies no extra authority |
| An app author changes `.bsh` source containing agentic functions | Shape the source artifact through the ordinary lifecycle | A declaration remains code to review; it cannot replace the frozen verifier |
| An action fails and its caller needs input | Existing yield/status conventions | Yield is not permission to rewrite, retry, approve or activate |

## Explicit composition example

Use a Sprint 564 H2-capable Bashy build; an installed release may predate
`bashy shape`. Register the [Hello World app](../examples/shaping/hello-world/README.md)
and configure its shaping agent first. Set `SHAPE_USER` to the selected app
principal, `SHAPE_ACTOR` to the caller's existing authorized identity, and
`SHAPE_AGENT` to an installed `tool:model` binding. Agent callers retain the
existing live-session identity requirements.

Save this as a `.bsh` script and run it with `bashy --bashsharp SCRIPT`:

```bash
set -eu
: "${SHAPE_USER:?set the app principal}"
: "${SHAPE_ACTOR:?set the authorized caller identity}"
: "${SHAPE_AGENT:?set the configured tool:model}"

agentic {
    bashy shape status hello-world \
        --user "$SHAPE_USER" --as "$SHAPE_ACTOR" --json
    bashy shape wish hello-world \
        'Change the displayed greeting from "Hello, World!" to "Hello, Shaping!".' \
        --user "$SHAPE_USER" --as "$SHAPE_ACTOR" \
        --agent "$SHAPE_AGENT" --json
}
```

The result is a tracked candidate, not an activated change. Inspect the returned
ID, run `show`, `diff`, `verify` and `preview`, then approve explicitly if the
result is wanted. Undo uses the same ordinary shaping interface. Follow the
Hello World walkthrough for the complete recipe and expected browser results.
The commands are also explicit valid requests outside the block; this example
shows composition without introducing a second shaping API or new syntax.

Existing runnable language examples provide the smaller building blocks:

- [judge.bsh](../examples/quickstart/judge.bsh): deterministic contracts,
  read-effect guard and status 6 yield without a model.
- [actions.bsh](../examples/agentic/actions.bsh) and
  [summarize.bsh](../examples/agentic/summarize.bsh): a marked shell function
  plus an entry block that explicitly invokes governed chat.
- [typed.bsh](../examples/agentic/typed.bsh): the same boundary for typed
  functions and methods. [Native embedding notes](agentic-action-example.md)
  explain how the handler carries the scope bit to cooperating tools.

## Inspected seams and limits

The pinned shell baseline `74586fde5f24` already contains the inspected
scope semantics. Its [block/call implementation](https://github.com/qiangli/sh/blob/74586fde5f2497fa89d83b142438daf7f00ef05c/interp/runner.go)
saves and restores the agentic scope; its [handler context](https://github.com/qiangli/sh/blob/74586fde5f2497fa89d83b142438daf7f00ef05c/interp/handler.go)
exposes the boolean without granting permissions. The existing
[language contract](https://github.com/bashsharp/bashsharp/blob/45ae8e2d0f1d0ad384355859a8f0eb4fed2a893c/docs/bashpp-agentic-mvp-plan.md)
keeps provider, budget, retry and permission policy outside the modifier.

`internal/agentos/agentic.go` is the separate `bashy agentic ACTION` command
shim: its bounded failure path emits a non-executed `input_required` proposal,
not a persisted repair. It must not be confused with the source block.
`internal/agentos/shape_local.go` and `yoke/pkg/shape/service.go` are the
current explicit CLI and shared shaping service seams; the
[shaping contract](shape-contract.md) defines their lifecycle and identity.
These are source-inspection findings, not new cross-platform execution evidence.

## KISS follow-ups

Keep the ordinary shaping commands as the MVP interface. An agentic source
artifact can be a later example once its app kind and declared verifier are
supported; H3 owns CLI/service activation, and H5/H7 own platform and agent E2E
proof. This spike does not claim those checks passed.

Automatic assistance after a failed action would need a separate decision about
its target artifact, request signal, retry/budget limits, permission envelope,
review policy and durable recovery. Any resulting candidate would still use the
same trusted verification and scoped activation. Do not infer that design from
the presence of an `agentic` block, and do not change the frozen shell/parser
baseline to implement it during this documentation spike.
