---
id: 76ed1bb95163
kind: bug
title: 'fleet registry ships stale model bindings: aider/opencode can''t launch (deepseek-v4 rejected; kimi server error)'
status: open
priority: p1
refs:
    - ../coreutils
reporter: qiangli
created: 2026-07-12T23:50:05.564993Z
---

## Observed — capability probes for aider + opencode both failed at the MODEL layer, not the task

- aider (agent `aider-deepseek-v4`): launched cleanly headless (aider v0.86.2), then:
    litellm.BadRequestError: DeepseekException - "The supported API model names are
    deepseek-v4-pro or deepseek-v4-flash, but you passed deepseek-v4."
  The binding names a model the provider does not accept.

- opencode (agent `opencode-kimi-k2.7-code`): exited 1 with
    {"name":"UnknownError","message":"Unexpected server error","ref":"err_7f5aeb73"}
  Possibly the kimi binding, possibly transient — inconclusive.

## Why this matters

The fleet LOOKED unreliable (agy hangs, opencode fails, aider fails), but the two that
WORK -- codex-gpt-5.5, claude-opus -- are exactly the two with valid bindings. The
others fail at config, not capability. We cannot measure a tool's capability while its
model binding is broken, and we were about to write off good tools for a registry bug.

## Root cause

The embedded baseline agent registry (`coreutils/pkg/fleet/baseline/agents|models/`)
carries model names that are stale or wrong for the current provider APIs:
`deepseek-v4` should be `deepseek-v4-pro` / `deepseek-v4-flash`; the kimi binding needs
verification.

## Task

1. Audit every baseline agent's model binding against what the provider API currently
   accepts. Fix the stale names. deepseek-v4 -> a valid tier (pro or flash -- see open
   question).
2. Add a `bashy agents doctor` (or extend `weave fleet`) that PROBES each agent's
   binding with a trivial 1-token request and reports which actually resolve -- so a
   broken binding is caught at check time, not 8 minutes into a run. This is the same
   lesson as the whole session: 'available' must mean 'verified to launch', not 'name
   is in a file'.

## Open question for the human (tier = cost/quality choice)

deepseek-v4-pro vs deepseek-v4-flash, and the kimi tier -- which does the user want as
the fleet default? That is a cost/quality decision, not a code one.

## Verify

    # after the fix, both probes re-run and actually attempt the task:
    bashy weave start --run <n> -- aider-deepseek-v4      # reaches the task, not a 400
    bashy weave start --run <n> -- opencode-kimi-k2.7-code
