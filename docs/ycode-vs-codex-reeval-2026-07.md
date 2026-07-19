# ycode (full-stack) vs codex — head-to-head re-eval (2026-07-19)

A same-model A/B of the first-party harness (ycode) against codex, run after
ycode gained the full agentic stack. **Model held constant** (OpenAI gpt-5.6);
codex on subscription, ycode on an OpenAI API key. The question: has the stack
closed the gap the earlier bake-off measured (ycode ~2–4× codex on tokens)?

## What changed in ycode since the last head-to-head

- **Responses API** (`/v1/responses`) — gpt-5 gets function tools + reasoning
  together, matching codex's wire (was stranded on chat/completions).
- **Efficiency fixes** — bash batching guidance, subagent necessity-gate, edit
  batching (measured −46% tokens / −54% calls on the L4 trap earlier).
- **In-process composable structured verbs** — the exec handler now actually
  runs coreutils in-process (it was forking system tools); `grep --json`, `ast`,
  `graph` resolve and compose in one call.
- **The hint engine** wired into ycode (steers classic grep/find → `--agentic`/
  `--json`/`ast`; heeding measured ~2/3).
- **The self-heal family** — autofix (wrong-dialect flag), autoretry (transient
  error), recommender (not-found → did-you-mean from ground truth).

## Method

- **L4 planted-trap** (steward judgment, gpt-5.6-sol): a Go module whose tests
  are GREEN but `Median` is wrong for even-length input. Pass = catch AND fix the
  bug (independent even-median gate), not trust the green.
- **L3 search task** (gpt-5.6-terra): list every file under `svc/` that calls
  `Compute`, as sorted JSON. Pass = all 5 caller files, not the definition.
- 2 trials each per tool per tier, **run in parallel**; ycode with
  `BASHY_HINTS=on` (its differentiator). Every result **independently gated** by
  this steward — never the self-report.

## Results

| tier | metric | codex | ycode (full stack) |
|---|---|---|---|
| **L4** | trap caught+fixed | **2/2** | **2/2** |
| | wall (mean) | 115s | **111s** |
| | tokens (mean) | 36,985 | 66,330 (**1.79×**) |
| | subagents | — | 0 (gate held) |
| **L3** | correct (5/5 files) | **2/2** | **2/2** (cleaner — no spurious `core.go`) |
| | wall (mean) | 14.5s | **8.5s** |
| | tokens (mean) | 4,012 | 12,072 (**3.0×**, variance 7k–17k) |

Raw: L4 codex 50099/23870 tok · 141/89s; L4 ycode 68706/63953 tok · 122/101s ·
0 subagents. L3 codex 4501/3523 tok · 16/13s; L3 ycode 17120/7023 tok · 4/1
calls · 11/6s. ycode cost this eval ≈ $0.61.

## Verdict

1. **Quality: full parity.** Both catch the steward trap (4/4) and solve the
   search task correctly (4/4); ycode's L3 answers were cleaner.
2. **Speed: ycode at parity or faster** — Responses API + in-process shell; L3
   notably faster (8.5s vs 14.5s).
3. **Tokens: gap narrowed, not closed** — from ~2–4× down to **1.8× (L4) /
   3× (L3)**. The residual is codex's fundamentally leaner one-shell-tool
   architecture; ycode trades those tokens for typed tools + the self-heal/hint
   machinery codex lacks.
4. **New machinery is live** — hints fired (2/run on L3), subagent gate held (0
   spawns), batching worked (one L3 run: a single compound `grep`, 7k tokens).
   But structured-verb *heeding* was 0/2 in these L3 runs (probabilistic; the
   model kept to classic `grep`).

## Caveats (do not over-read)

- **2 trials each = high variance** (L4 codex 24k–50k tokens; L3 ycode 7k–17k).
  Ratios are directional, not precise.
- This eval **did not exercise the self-heal features** (autofix / autoretry /
  recommender). Those cut round trips on *wrong-flag / transient / not-found*
  conditions the median+search tasks never hit — their savings show elsewhere.
- Anthropic-operated steward judging OpenAI-vs-OpenAI; the gate is objective
  (compile + even-median assertion + file-set), so the quality verdict is
  harness-independent, but the token/wall figures are single-host, low-n.

**Bottom line:** ycode has closed the quality and speed gap with codex on the
same model, and halved the token gap. It is now a genuine peer — matching
outcomes, matching/beating wall-time — at a token premium that buys the
differentiators (hints, structured verbs, self-heal) a raw shell-only harness
structurally cannot offer. The next lever is heeding (make the model actually
take the structured-verb hint) and a self-heal-exercising task to measure the
round-trip savings this eval didn't trigger.

Companion: `harness-ab-deepseek.md` (three-harness A/B), `band-ladder.md`
(L1–L4 ladder), `first-party-harness.md` (why ycode is in the fleet).

## Addendum — X4 cascade (glm-5.2 → terra → sol) vs codex, clean timing

Re-run after the coach-termination fix (ycode 35d6590 turn.end emit + coreutils
ee45e12 turn-boundary teardown), so wall-time is real, not the old 540s ceiling.
The cascade runs via `bashy coach --agent ycode-cascade-x4` (glm base; escalates
to terra then sol only when the coach's reflex fails to break a loop).

| tier | X4 quality | X4 wall (mean) | codex | escalation |
|---|---|---|---|---|
| **L4** trap (steward) | CAUGHT+FIXED 2/2 | **109s** | 2/2 · ~115s | none — glm solved it |
| **L3** search | 5/5 correct 2/2 | **54s** | 2/2 · ~14.5s | none |

Verdict: on the HARD steward task, X4 matched codex-sol on quality AND wall-time
(~109 vs ~115s) with a cheap glm base — the terra→sol ladder never fired, so the
premium tier was an unused safety net (the cost win realized). On the TRIVIAL L3
task codex was ~4× faster: the cascade's fixed launch overhead (coach + glm
startup, ~40s) dominates a task that's over in seconds. Cascades earn their keep
where the base does real work, not on quick lookups. Escalation stayed dormant
both tiers — glm-5.2 (L2) cleared L4 unaided, which is itself the finding.

## Anthropic head-to-head — PREPPED, blocked on access (2026-07-19)

The claude (opus4.8/fable5) vs ycode head-to-head is fully prepared but cannot
run: ycode has no legitimate access to the Anthropic subscription models.
- Subscription OAuth (`ycode login`) authenticates AS Claude Code (Anthropic's
  Claude-CLI client ID) and 403s without the Claude-Code-only beta header +
  system-prompt identity. Completing that = impersonating Claude Code to bypass
  Anthropic's access control that ties the Max subscription to their first-party
  client — declined (ToS circumvention).
- The supported path (ANTHROPIC_API_KEY with credits) is unavailable: the
  operator's Anthropic API recharge errors out; the $200/mo Max subscription is a
  separate product with no API credits.

What IS ready for the instant access appears: agents ycode-opus4.8 (Esme, L3),
ycode-fable5 (Roan, L4), the glm-based claude cascades ycode-cascade-claude-x3/x4
(Vesna/Vera), and the three ycode gap fixes (edit correctness, LLM compaction
default, tool-loop detection — ycode 079c62e) applied from the open-claude-code
study. Run `bashy agents whois ycode-opus4.8` for the contact; the only missing
piece is a funded ANTHROPIC_API_KEY.
