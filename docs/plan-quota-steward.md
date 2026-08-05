# Plan: Quota Steward (`bashy ration`)

## Goal

Use every provider subscription efficiently without exhausting its rolling
allowance in the middle of an agent task. Admission must account for whether a
task can reach a safe checkpoint, not merely whether the next model call fits.
When capacity becomes uncertain or scarce, preserve enough allowance to verify,
checkpoint, summarize, and hand off consistent work.

This is an evolution of `coreutils/pkg/llmbudget`, not a parallel meter. The
existing package already performs pre-call checks, records post-response
tokens/requests/cost, distinguishes subscription/API/local lanes, returns
allow/downgrade/queue/block/route decisions, persists local counters, gates
chat turns, and emits OTel bound events.

## Provider truth boundary

Keep API rate limits separate from consumer subscription allowances:

- Anthropic's API publishes request/input/output-token limit, remaining, and
  reset headers. Claude subscription products use separate rolling/session and
  weekly limits; no supported exact remaining-subscription API is assumed.
- OpenAI's API publishes request/token limit, remaining, and reset headers and
  has historical Usage/Costs APIs. Codex subscription usage is a separate pool;
  no supported exact remaining-subscription API is assumed.
- Gemini API limits are project RPM/TPM/RPD limits, with configured limits
  available through relevant Google surfaces. Gemini CLI/Code Assist has a
  shared subscription request pool where one prompt may cause multiple calls;
  no per-response remaining balance is assumed.

An observation records `source` (`provider_header`, `provider_api`,
`provider_cli`, `provider_ui`, `manual`, or `inferred`), confidence, observation
time, remaining amount or percentage, window, and next-reset timestamp. Claude
and Codex subscription surfaces observed in practice expose how much quota is
left before daily/session and weekly resets; those values are first-class inputs
even where the vendor does not publish a supported machine API. A versioned
adapter may capture them from the provider CLI/status surface, and `ration
observe` may accept a manual/UI reading. Reconcile either against local metering
and retain its provenance rather than treating scraped UI text as an API
contract. `unknown` is a real state distinct from zero and unlimited.

Research references:

- <https://platform.claude.com/docs/en/api/rate-limits>
- <https://claude.com/pricing>
- <https://platform.openai.com/docs/api-reference/usage>
- <https://help.openai.com/en/articles/11369540-using-codex-with-your-chatgpt-plan>
- <https://ai.google.dev/gemini-api/docs/rate-limits>
- <https://developers.google.com/gemini-code-assist/resources/quotas>

## Model

Maintain a local-first capacity ledger keyed by account, provider, surface
(`subscription`, `api`, or `local`), plan, model, and quota window. Hash account
identities and store no prompt, response, credential, or secret-bearing raw
header. State remains mode `0600` with configurable retention/export.

Budgets are hierarchical:

```text
account → provider/surface → campaign → story → run → turn
```

Before dispatch, the conductor requests a lease containing estimated p50 and
p90 tokens, requests, weighted credits, and wall time. Estimates combine task
class/difficulty, repository/context size, model, prior outcomes, and OTel/log
history. Reserve the p90 demand atomically, reconcile actual provider-reported
usage after every turn, and refund unused capacity when the lease closes.

Use weighted fair scheduling, priority/deadline policy, and concurrency limits
so one story cannot consume the pool. With uncertain capacity, use conservative
adaptive envelopes based on rolling quantiles/EWMA plus observed reset and
exhaustion events—never a fabricated exact balance.

Keep a configurable 15–25% emergency reserve, increasing as confidence falls.
It may fund only bounded verification, checkpoint, commit/stash, summary,
baton/handoff, or cancellation. It cannot fund new scope.

## Decisions and continuity

Admission may:

- allow with a lease;
- shrink/decompose scope;
- select a cheaper qualified model;
- route to another provider that satisfies capability and data policy;
- queue until a known or inferred reset; or
- refuse when the task cannot safely reach a checkpoint.

Never silently cross from a subscription into metered API spend. Overrides such
as `--borrow-reserve` or `--allow-metered-overrun` are explicit, scoped,
expiring, and audited.

At the soft threshold or any quota/429 signal, stop starting mutations, reach a
safe point, run a bounded verifier, and record the working tree, commands,
evidence, remaining task, lease state, and next action in the sprint/weave
checkpoint. Release or park the lease and notify the steward. Cancellation,
checkpoint, and handoff must never be blocked by the normal task budget.

## Proposed surface

```sh
bashy ration status [--json]
bashy ration plan --task ID --agent MODEL --points N
bashy ration lease acquire|renew|release ...
bashy ration observe --remaining 37% --reset 2026-08-07T00:00:00Z \
  --window weekly --source provider-ui ...
bashy ration explain LEASE
bashy ration override --expires DURATION ...
```

Lease records include ID, task/campaign, account/surface/model, p50/p90
estimates, reserved/used values, soft/hard thresholds, expiry, checkpoint
threshold, policy version, evidence source, and confidence.

## Ownership

- **`pkg/llmbudget` / Bashy:** atomic ledger, provider observations, rolling
  windows, admission primitives, leases/reservations, reconciliation, status,
  audit and OTel binds.
- **Conductor/steward:** task sizing judgment, priority/fairness policy,
  provider/model choice, checkpoint and handoff decisions.
- **ycode/provider clients:** pre-request checks, actual per-turn token/request
  reporting, rate-limit/reset observations, and OTel correlation.
- **`chat`/`invoke`/`meet`/`foreman`/`weave`:** one shared admission path; no
  separate counters or silent bypasses.

## Existing gaps to close

- Fixed local calendar day/week counters do not model provider rolling windows,
  reset zones, DST, or clock skew.
- Process-local locking plus direct state writes can lose concurrent updates or
  leave a torn file.
- There are no task/campaign leases, hierarchy, reconciliation/refund, fairness,
  or checkpoint-only reserve.
- Unknown model/price/subscription metadata currently fails open.
- There are no confidence-tagged provider observation adapters.
- Claude/Codex remaining-and-reset readings are not yet captured or reconciled
  with local counters.
- A reserved rate estimate is not reconciled with actual usage.
- Subscription exhaustion must not be assumed to spill safely into paid usage.

## Acceptance criteria

1. Competing processes acquire capacity atomically; corrupt/torn state recovers
   without creating capacity.
2. Rolling/fixed reset tests cover DST, timezone and clock-skew boundaries.
3. Unknown is never rendered or treated as zero/unlimited; tighter provider
   evidence immediately constrains new leases. Daily/session and weekly
   remaining/reset observations independently constrain the same allocation.
4. A p90 reservation is reconciled/refunded from actual usage, including usage
   above estimate and agent-crash lease expiry.
5. Emergency reserve refuses normal turns but completes verifier, checkpoint
   and handoff paths.
6. A 429/reset observation trips a circuit breaker, records the reset evidence,
   queues safely and produces no retry storm.
7. Rerouting preserves minimum capability and privacy policy and never incurs
   metered spend without a live explicit override.
8. Concurrent campaigns demonstrate weighted fairness and no starvation.
9. Restart/replay preserves leases and redelivers unresolved handoff work.
10. Telemetry outage degrades confidence and capacity conservatively; prompts,
    responses, credentials and account identifiers never appear in state/logs.
11. Every launch/turn surface is proven to use the shared admission path, and a
    task denied at admission makes no workspace mutation.
