---
id: 07012890-6b3a-4f43-a7d0-cff34a891aef
seq: 31
form: page
type: runbook
title: 'Give your console a shaping agent (bring your own key or subscription)'
description: Wishes on the Shape page fail until the console has a shaping agent; pick one the user is entitled to (own key via an OpenAI-compatible route, own subscription CLI, or a local model), bind its credential by reference, and start the console with it — WHEN the Shape page or bashy shape wish says "no shaping agent is configured", or a user wants wishes on their own cost/privacy terms (keywords shape-agent, genie, byok, openai-compat, model add, agent add, secrets.map, service set apps, bedrock, gemini, ollama).
tags:
    - shaping
    - console
    - models
    - byok
status: candidate
evidence: 'Live run on a Windows owner outpost: console had no agent; a free-tier key hung (rate limit), a BYOK OpenAI-compatible route with tool calls worked end to end (wish to ready candidate in about a minute).'
created: "2026-11-19T04:40:40Z"
updated: "2026-11-19T04:40:40Z"
---

The console delegates every wish to one agent (`--shape-agent TOOL:MODEL`). With none set, wishes are refused ("no shaping agent is configured on this console"). The route is the user's choice — their key, their subscription, or a local model — for their own cost, privacy and governance reasons.

1. **Pick a route the user owns.** Any OpenAI-compatible endpoint that supports tool calls works with bashy's built-in agent (genie). Check it once with a tiny tool-call request before wiring it in. Free tiers may rate-limit or hang under an agent's load; prefer the user's own paid key or subscription for real use.
2. **Register the model and binding** (names are the user's):
   `bashy model add MY-MODEL --provider openai-compat --kind api --base-url https://ENDPOINT/v1 --api-key-ref VAULT_KEY --upstream PROVIDER_MODEL_ID`
   `bashy agent add genie-MY-MODEL --tool genie --model MY-MODEL --permission auto`
3. **Bind the credential by reference** (never the value): add `VAULT_KEY_API_KEY=@VAULT_KEY` to `~/.config/bashy/secrets.map`. Smoke it: `bashy delegate --agent genie:MY-MODEL "Reply with exactly: SMOKE-OK"`.
4. **Start the console with it.** `bashy service show apps` prints the console's argv; `bashy service set apps --exec …` repeats every element and appends `--shape-agent=genie:MY-MODEL`; then `bashy service restart apps`. Confirm the running console's command line carries the flag.
5. The CLI does not use the console's flag: `bashy shape wish APP "…" --user PRINCIPAL --agent genie:MY-MODEL`.

A subscription CLI (claude, codex, opencode, …) is the same step 4 with `--shape-agent=TOOL:MODEL` once that CLI is installed and signed in on the host. A local model behind `bashy llm` is an OpenAI-compatible route like any other.
