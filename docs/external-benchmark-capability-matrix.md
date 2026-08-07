# External-benchmark capability matrix — fleet external priors v0

*Retrieved 2026-08-06. Algorithm: `ebc-v1.0.0` (see
`external-benchmark-composite-method.md`). Fleet snapshot: 26 model records, 38
`tool:model` bindings (`coreutils/pkg/fleet/baseline/`), including 4 composite
cascade bindings. Status: **external PRIOR data — never a measurement**; nothing
here sets `band_source: measured`, and host-measured findings outrank every row
below (method §7).*

**v0 application note (disclosed deviation):** capability cells are published as
*per-instrument* values + an evidence grade, not as a single pooled number.
Coverage across models is too unbalanced for cross-benchmark pooling to be
honest (a model measured only on a hard benchmark would rank below a model
measured only on an easy one). Pooled composites per §6.3 will be published once
instrument coverage is comparable across the models being compared; until then,
compare models **within one instrument column only**.

Provenance markers used throughout:

- **[I]** independent (maintainer-run or third-party-run, primary source read)
- **[Iᵖ]** independent lab's result, but relayed via press/aggregator because the
  primary was unreachable (weight-discounted; verify before load-bearing use)
- **[V]** vendor-reported
- **—** missing: no row found under the exact identity. Never zero.

## 1. Identity mapping (fleet name ↔ external identity)

Ingestion rule (§2.2): a row only attaches on an exact, unambiguous match.

| fleet model | confirmed public identity (2026-08-06) | mapping notes |
|---|---|---|
| `fable5` | `claude-fable-5` (anthropic.com/claude/fable) | exact string confirmed in Epoch/DeepSWE data. METR lists only "Claude Mythos Preview (early)" — related but NOT mapped to fable5 |
| `opus5` | `claude-opus-5` (2026-07-24) | exact |
| `opus4.8` | `claude-opus-4-8` (2026-05-28) | exact. Epoch's SWE-V has opus-4-**7**, not 4.8 — not mapped |
| `sonnet5` | `claude-sonnet-5` (2026-06-30) | exact |
| `haiku4.5` | `claude-haiku-4-5-20251001` | dateless id is an alias for the pinned snapshot |
| `opus4.6` | `claude-opus-4-6` (2026-02-05) | exact |
| `sonnet4.6` | `claude-sonnet-4-6` (2026-02-17) | exact |
| `gpt-5.5` | `gpt-5.5` (2026-04-23) | Epoch ran `gpt-5.5-pre-release_xhigh` — flagged, GA may differ. "GPT-5.5 **Pro**" rows are a different variant, not mapped |
| `gpt5.6-sol` / `-terra` / `-luna` | `gpt-5.6-sol/-terra/-luna` (2026-07-09) | terra/luna verified verbatim in OpenAI's Codex changelog |
| `gpt5.4-mini` | `gpt-5.4-mini` (2026-03-17) | "GPT-5.4" (base) rows NOT mapped to mini |
| `gpt5.3-spark` | public spelling "GPT-5.3-Codex-Spark" (preview, 2026-02-12) | **no benchmark row found under any Spark identity.** `gpt-5.3-codex` rows are the bigger sibling — NOT mapped |
| `gpt-oss-120b` | `gpt-oss-120b` (Aug 2025, open weights) | fleet binds the agy "medium" variant; external rows rarely state reasoning tier — flagged |
| `gemini3.1` | Gemini 3.1 Pro (2026-02-19) | leaderboards use "Gemini 3.1 Pro (Preview)" / "-customtools" variants; fleet binding is the High effort tier |
| `gemini3.5-flash` / `-low` | Gemini 3.5 Flash | external rows carry their own effort tags (`_high`, `_medium`); only effort-tagged rows map, and none maps to the `-low` tier |
| `gemini3.6-flash` / `-low` | `gemini-3.6-flash` (2026-07-21) | same tier rule; "3.5/3.6 Flash-**Lite**" is a different model, never mapped |
| `deepseek-v4-pro` / `-flash` | `deepseek-v4-pro` / `-flash` (preview 2026-04-24) | exact |
| `deepseek-chat` | routing alias → `deepseek-v4-flash`; vendor retires the alias after 2026-07-24 | **identity rot**: historical rows (aider 70.2 in 2025) measured a different underlying model. No stable external identity — registry should migrate this record |
| `kimi-k2.6` | `kimi-k2.6` (~2026-04) | leaderboard "Kimi K2.5" is a different model, not mapped. Snorkel's "kimi-2-6" mapped with a flag |
| `kimi-k2.7-code` | `kimi-k2.7-code` (vendor page 2026-07-22) | `-highspeed` variant exists, not bound |
| `kimi-k3` | `kimi-k3` (2026-07-16) | "kimi-k3-max" arena rows: effort variant, flagged |
| `glm-5.2` | `glm-5.2` (2026-06-13) | "GLM-5" / "GLM-5.1" rows are earlier models — never mapped to 5.2 |
| cascades (`ycode-cascade-*`) | none | composite bindings have **no external identity**; any prior is `derived` from constituents |

## 2. Sources (header blocks)

All retrieved 2026-08-06. `q` = source-quality weight (§6.1).

| id | source | version | harness | n | CI | provenance | q | freshness note |
|---|---|---|---|---|---|---|---|---|
| `tb21` | tbench.ai/leaderboard/terminal-bench/2.1 | TB 2.1 | per-entry agent (Claude Code, Codex, Terminus 2, …), Harbor, team-verified | 89 tasks | ± per entry | independent | 1.0 | current |
| `tb20` | tbench.ai/leaderboard/terminal-bench/2.0 | TB 2.0 | as above | 89 | ± | independent | 1.0 | one major back (v-penalty 0.5) |
| `fb01` | frontierbench.ai/announcement | Frontier-Bench v0.1 (TB 3) | Codex / Claude Code / Cursor CLI | 74 tasks | none | independent | 0.9 | live board fetch-resistant; announcement rows |
| `aa-tb` | artificialanalysis.ai/evaluations/terminalbench-v2-1 | TB 2.1 re-run | Terminus 2 in e2b, pass@1 ×3 | 89 | none | independent | 0.9 | only top-3 rendered |
| `tau3` | Sierra S3 `sierra-tau-bench-public…/submissions/` | τ³ v1.0.1, banking_knowledge | tau2-bench, gpt-5.2 user-sim, 4 trials | ≈97 tasks (inferred from denominators — flagged) | none; pass^k decay instead | independent (Sierra-run) | 1.0 | current; τ² classic domains null for mid-2026 models |
| `bfcl4` | gorilla.cs.berkeley.edu (CSVs) | BFCL V4 | Berkeley-run, pinned commit | 665 agentic cases | none | independent | 1.0 | **stale: newest models are Dec-2025 era** |
| `mcpu` | mcp-universe.github.io | MCP-Universe | 3 tracks, Salesforce-run | 231 tasks | none | independent | 0.9 | stale for 2026 frontier |
| `swb` | swebench.com (leaderboards.json) | SWE-bench Verified + bash-only | bash-only = mini-SWE-agent for all | 500 | none | independent; `checked` flag per row | 1.0 | last data commit 2026-02-27 — stale |
| `epoch` | epoch.ai/benchmarks/swe-bench-verified (data zip) | SWE-V, Epoch scaffold v2.0.0+ | basic-agent / Claude Code / Codex; Docker, no net | 484 validated | stderr per run | independent | 1.0 | no Anthropic-5-series / gpt-5.6 rows yet |
| `swbpro` | labs.scale.com SWE-bench Pro public/private | Pro | SWE-Agent / mini-swe-agent, 250 turns | 731 / ≈276 | ± per entry | independent | 0.9 | OpenAI disputes task quality (~30% claimed broken) — recorded, not adjudicated |
| `dswe` | deepswe.datacurve.ai | DeepSWE | mini-swe-agent for all; pass@1 ×4 | 113 tasks | 95% CI half-width | independent | 1.0 | updated 2026-08-06 |
| `fcode` | cognition.com/frontiercode (via Epoch mirror) | FrontierCode | **mixed per-row harness** | n/s | none | independent | 0.8 | harness varies per row — flagged |
| `fswe` | frontierswe.com (via Epoch mirror) | FrontierSWE | **per-model native harness** (Claude Code vs Kimi CLI etc.) | n/s | none | independent | 0.7 | measures model+its-own-tool, not scaffold-controlled |
| `curb` | cursor.com/cursorbench (via Epoch mirror) | CursorBench | Cursor's own agent | n/s | none | independent-adjacent (commercial harness owner) | 0.7 | |
| `gso` | gso-bench.github.io (via Epoch mirror) | GSO opt@1 | OpenHands | n/s | none | independent | 0.8 | |
| `lcbp` | livecodebenchpro.com (API) | LCB-Pro | Codeforces-style Elo, contest-fresh | 72 models | Bayesian MAP | independent | 0.9 | no 2026-frontier Anthropic/OpenAI rows |
| `aider` | aider.chat/docs/leaderboards | polyglot, 225 ex. | aider | 225 | none | independent | 1.0 | **frozen 2025-11-20** |
| `crb` | codereviewbench.com (Kodus) | CodeReviewBench | dual LLM judges | 75 cases | none | independent-adjacent (code-review vendor; LLM-judge) | 0.6 | only 2 fleet models covered |
| `osw2` | snorkel.ai/leaderboard/os-world-2-0 | OSWorld 2.0 | standard/batched tool, 500 steps; binary + partial | 108 tasks | none | independent | 1.0 | vendor OSW2 claims use an undisclosed other metric — never mix |
| `oswv` | leaderboard.steel.dev + llm-stats (OSWorld-Verified) | OSW-Verified | **per-vendor harnesses** (Anthropic "revised harness" etc.) | 361 | none | vendor (llm-stats: "0 verified, 22 self-reported") | 0.5·ind | effectively vendor-scored now |
| `om2w` | leaderboard.steel.dev/online-mind2web | Online-Mind2Web | system-level, judge varies per row | 300 | none | mixed per row | 0.6 | rows not mutually comparable |
| `hle-s` | labs.scale.com/leaderboard/humanitys_last_exam | HLE no-tools | closed-book | 2,500 q | ±95% | independent | 1.0 | with-tools protocol is a different test |
| `hle-v` | vendor launch pages (relayed) | HLE with-tools | per-vendor scaffolds | n/s | none | vendor [Iᵖ relay] | — | not comparable across vendors |
| `bcomp` | benchlm.ai/benchmarks/browsecomp (relaying vendor) | BrowseComp | per-vendor | 1,266 q (orig paper) | none | vendor-relayed | — | openai.com primary 403'd |
| `metr` | metr.org/time-horizons (TH 1.1) | 50% horizon | HCAST+RE-Bench+SWAA, 228 tasks | 228 | 95% bootstrap | independent | 1.0 | newest per-model values graph-only; >16h unreliable per METR |
| `vb2` | andonlabs.com Vending-Bench 2 / Arena | VB2 | 365 sim days, mean of 5 runs | 5 runs | none | independent — **but site 403'd; all rows are [Iᵖ]** | 0.8·relay | verify before load-bearing use |
| `orchb` | arxiv.org/html/2607.25656v1 (OrchBench) | v1 | DAG-simulated orchestration plans | 100 tasks | none | independent academic, one-shot | 0.7 | no live leaderboard; narrow score spread |
| `arena` | arena.ai/leaderboard (ex-lmarena) | text / coding / webdev | pairwise human preference Elo | votes per row | ± per row | independent | **0.5 cap (preference, §4)** | re-baselined 2026-07-12 |

Unreachable primaries recorded per §8: openai.com + help.openai.com (403),
andonlabs.com (403), os-world.github.io leaderboard (JS-only),
GAIA/Gaia2 HF Spaces (dynamic), osu-nlp-group.github.io/Online-Mind2Web (404),
WebArena Google-Sheet board, Martian code-review table (JS),
frontierbench.ai live table (canary/fetch-resistant), aider board cross-checked
via Epoch mirror.

## 3. Raw rows

Row ids `C`=coding, `T`=terminal, `U`=tool-use, `P`=planning, `V`=review,
`B`=browser/computer, `R`=research, `H`=long-horizon. Cluster = lineage cluster
(§6.2); rows sharing a cluster collapse to one effective row.

### 3.1 Coding / bug fixing

| id | source | model string as listed | value | metric | date | prov | cluster |
|---|---|---|---|---|---|---|---|
| C01 | dswe | claude-opus-5_max | 0.7365 ±0.0387 | pass@1 (pass@4 0.885) | 2026-07 | I | dc |
| C02 | dswe | gpt-5.6-sol_max | 0.7267 ±0.0283 | pass@1 | 2026-07 | I | dc |
| C03 | dswe | claude-fable-5_xhigh / _max | 0.6991 ±0.0324 / 0.6972 ±0.0403 | pass@1 | 2026-07 | I | dc |
| C04 | dswe | gpt-5.6-terra_max | 0.6962 ±0.0256 | pass@1 | 2026-07 | I | dc |
| C05 | dswe | kimi-k3_max | 0.6851 ±0.0454 | pass@1 | 2026-07 | I | dc |
| C06 | dswe | gpt-5.6-luna_max | 0.6719 ±0.0399 | pass@1 | 2026-07 | I | dc |
| C07 | dswe | gpt-5.5_xhigh | 0.6704 ±0.0647 | pass@1 | 2026-07 | I | dc |
| C08 | dswe | claude-opus-4-8_max | 0.5897 ±0.0176 | pass@1 | 2026-07 | I | dc |
| C09 | dswe | claude-sonnet-5_max | 0.5385 ±0.0424 | pass@1 | 2026-07 | I | dc |
| C10 | dswe | gemini-3.6-flash_high | 0.4856 ±0.0500 | pass@1 | 2026-07 | I | dc |
| C11 | dswe | glm-5.2_max | 0.4378 ±0.0173 | pass@1 | 2026-07 | I | dc |
| C12 | dswe | gemini-3.5-flash_medium | 0.3739 ±0.0179 | pass@1 (effort ≠ fleet tiers — flagged) | 2026-07 | I | dc |
| C13 | dswe | kimi-k2.7-code | 0.3053 ±0.0050 | pass@1 | 2026-07 | I | dc |
| C14 | dswe | gemini-3.1-pro-preview | 0.1175 ±0.0249 | pass@1 — **outlier vs C22/C24, likely harness incompatibility; excluded from grading** | 2026-07 | I | dc |
| C15 | epoch | glm-5.2_max | 0.787 ±0.019 | SWE-V mean (n=484) | 2026-06-25 | I | ep |
| C16 | epoch | gemini-3.5-flash_high | 0.7934 ±0.0184 | SWE-V | 2026-06-01 | I | ep |
| C17 | epoch | deepseek-v4-pro_max | 0.7764 ±0.0190 | SWE-V | 2026-06-18 | I | ep |
| C18 | epoch | kimi-k2.6 | 0.7665 ±0.0192 | SWE-V | 2026-05-08 | I | ep |
| C19 | epoch | gpt-5.5-pre-release_xhigh | 0.8058 ±0.0180 | SWE-V (pre-release build — flagged) | 2026-04-24 | I | ep |
| C20 | epoch | claude-opus-4-6 | 0.7872 / 0.7562 (two runs) | SWE-V | 2026-02 | I | ep |
| C21 | epoch | claude-sonnet-4-6 | 0.7521 ±0.0196 | SWE-V | 2026-02-21 | I | ep |
| C22 | epoch | gemini-3.1-pro-preview-customtools | 0.7562 ±0.0195 | SWE-V | 2026-02-24 | I | ep |
| C23 | swb | Claude Opus 4.6 | 75.6 | SWE-V bash-only %resolved (unchecked) | 2026-02-17 | I | swb |
| C24 | swbpro | gemini-3.1-pro (thinking)* | 46.10 ±3.60 pub / 32.20 ±5.69 priv | Pro %resolved | 2026 | I | sc |
| C25 | swbpro | claude-opus-4-6 (thinking)* | 51.90 ±3.61 pub / 47.10 ±6.07 priv | Pro %resolved | 2026 | I | sc |
| C26 | swb | Claude 4.5 Haiku (high reasoning) | 66.6 bash-only (unchecked); 64.7 multilingual (checked) | %resolved | 2026-02 | I | swb |
| C27 | swb | gpt-oss-120b | 26.0 | bash-only %resolved (checked) | 2025-08 | I | swb |
| C28 | aider | gpt-oss-120b_high | 41.8 | polyglot % correct | 2025 | I | ai |
| C29 | lcbp | Gemini 3.1 Pro Preview | 2887 | Elo | ≤2025-11 contests | I | lc |
| C30 | lcbp | GPT OSS 120B | 1299 (inactive) | Elo | dated | I | lc |
| C31 | fcode | claude-fable-5 (claude-code, xhigh) | 0.535 | Mean@5 (mixed harness board) | 2026 | I | cg |
| C32 | fcode | claude-opus-5_max 0.534 · gpt-5.6-sol 0.475 · claude-opus-4-8 0.465 · kimi-k3 0.442 · gpt-5.5 0.430 · claude-sonnet-5 0.427 · gpt-5.6-terra 0.413 · gpt-5.6-luna 0.398 · kimi-k2.7-code 0.301 · glm-5.2 0.245 · deepseek-v4-pro 0.176 | as listed | Mean@5 | 2026 | I | cg |
| C33 | fswe | claude-fable-5 0.90 · claude-opus-4-8 0.78 · gpt-5.5 0.76 · glm-5.2 0.72 · gemini-3.1-pro-preview 0.41 · deepseek-v4-pro 0.31 · kimi-k2.6 0.28 | dominance (native per-model harness — model+own-tool, flagged) | 2026 | I | fs |
| C34 | curb | claude-fable-5_max 0.729 · claude-opus-5_max 0.700 · gpt-5.6-sol_max 0.672 · gpt-5.5_xhigh 0.643 · claude-opus-4-8_max 0.638 · claude-sonnet-5_max 0.612 · glm-5.2_max 0.546 · gemini-3.5-flash 0.498 · kimi-k2.6 0.476 | CursorBench score | 2026 | I-adj | cu |
| C35 | gso | claude-opus-4-8 0.4706 · gpt-5.5_xhigh 0.402 · claude-sonnet-5 0.3725 · gemini-3.1-pro-preview 0.2255 | OPT@1 | 2026-07 | I | gs |
| C36 | vendor (relayed) | Claude Fable 5: SWE-V 95.0 / Pro 80.3 (contested) · Claude Opus 5: SWE-V 96.0 / Pro 79.2 · DeepSeek V4 Pro: SWE-V 80.6 · Kimi K3: SWE-V 76.8 · GPT-5.6 Sol: Pro 64.6 (OpenAI's own framing) | %resolved, own scaffolds | 2026 | V | ven-c |
| C37 | vendor (docs.z.ai) | GLM-5.2: SWE-bench Pro 62.1 · FrontierSWE ~84 | own scaffold | 2026-06 | V | ven-c |

Note the two number families in C36 vs `swb`/`epoch`: vendor 95–96% SWE-V vs
best independent 78–83% — different scaffolds, no independent reproduction of
any 5-series claim yet. They are recorded, weight-discounted, and **not**
averaged together.

### 3.2 Terminal / shell

| id | source | agent+model as listed | value | metric | date | prov | cluster |
|---|---|---|---|---|---|---|---|
| T01 | tb21 | Claude Code + Fable 5 | 83.8 ±1.2 | task acc | 2026-06-07 | I | tb |
| T02 | tb21 | Codex + GPT-5.5 | 83.1 ±1.1 | task acc | 2026-05-01 | I | tb |
| T03 | tb21 | Terminus 2 + Fable 5 | 80.4 ±1.2 | task acc | 2026-06-05 | I | tb |
| T04 | tb21 | Claude Code + Opus 4.8 | 78.9 ±1.3 | task acc | 2026-07-09 | I | tb |
| T05 | tb21 | Codex + GPT-5.6 Terra | 78.4 ±1.3 | task acc | 2026-07-11 | I | tb |
| T06 | tb21 | Codex + GPT-5.6 Luna | 75.7 ±1.3 | task acc | 2026-07-11 | I | tb |
| T07 | tb21 | Claude Code + Sonnet 5 | 74.6 ±1.6 | task acc | 2026-07-09 | I | tb |
| T08 | tb21 | Gemini CLI + Gemini 3.1 Pro 65.8 ±1.7 · Terminus 2 + Gemini 3.1 Pro 65.6 ±1.7 | task acc | 2026-05-05 | I | tb |
| T09 | aa-tb | GPT-5.6 Sol (xhigh) 89.5 · Claude Opus 5 (max) 89.1 · GPT-5.6 Sol (max) 88.0 | pass@1 ×3, Terminus-2/e2b | 2026-08-06 | I | aa |
| T10 | fb01 | Codex + GPT-5.6 Sol 34.4 · Claude Code + Fable 5 33.8 · Claude Code + Opus 4.8 21.1 · Codex + Terra 20.8 · Claude Code + Sonnet 5 14.6 · Codex + Luna 14.3 · Claude Code + GLM 5.2 5.1 | Frontier-Bench v0.1 pass rate | 2026 | I | tb (same team — collapses with tb for independence counting) |
| T11 | tb20 | Meta-Harness + Claude Opus 4.6 76.4 ±2.4 (best of 3 entries) · Simplai + Claude Sonnet 4.6 53.4 ±2.8 · Goose + Claude Haiku 4.5 35.5 ±2.9 · Terminus 2 + GPT-OSS-120B 18.7 ±2.7 · Codex CLI + GPT-5.5 82.2 ±2.2 · TongAgents + Gemini 3.1 Pro 80.2 ±2.6 | task acc (TB2.0, one major back) | 2025-11→2026-05 | I | tb |
| T12 | vendor | Kimi K3: TB 2.1 88.3 · GLM-5.2: TB 2.1 81.0 · Haiku 4.5: TB(1) 40.2/41.75 (Terminus 2, 11 runs) | own runs; **no official tb21 rows exist for K3/GLM-5.2 — unverified** | 2026 | V | ven-t |

Conflict recorded: aggregator claims of "GPT-5.6 Sol 91.9 / Mythos 5 88.0 on
TB2.1" could not be confirmed on the official board and are excluded.
T12's GLM-5.2 vendor TB2.1 81.0 coexists with T10's independent Frontier-Bench
5.1 — different benchmarks, but the spread is the strongest vendor-vs-independent
tension in this matrix; noted for the refresh queue.

### 3.3 Tool use (τ³ banking_knowledge, pass^1 / pass^4; Sierra-run)

| id | model string as listed | pass^1 | pass^4 | eval date | prov | cluster |
|---|---|---|---|---|---|---|
| U01 | Claude Opus 5 (max) | 48.71 | 31.96 | 2026-08-03 | I | si |
| U02 | GPT-5.6-sol (xhigh) | 46.91 | 27.84 | 2026-07-22 | I | si |
| U03 | GPT-5.5 (xhigh) | 44.6 | 29.9 | 2026-05 | I | si |
| U04 | Claude Opus 4.8 (max) | 39.69 | 22.68 | 2026-07-23 | I | si |
| U05 | Claude Fable 5 (max) | 39.69 | 28.87 | 2026-07-23 | I | si |
| U06 | Kimi K3 (max) | 37.11 | 17.53 | 2026-07-24 | I | si |
| U07 | GLM-5.2 (xhigh) | 37.11 | 13.40 | 2026-07-24 | I | si |
| U08 | Claude Opus 4.6 (max) | 27.3 | 11.3 | 2026-05 | I | si |
| U09 | Gemini 3.1 Pro Preview (high) | 26.0 | 9.3 | 2026-05 | I | si |
| U10 | bfcl4: Claude-Haiku-4-5-20251001 (FC) | 68.70 overall / 68.95 agentic | — | 2026-04-12 | I | bk |
| U11 | vendor: Kimi K3 MCPMark-Verified 94.5 · MCP-Atlas 84.2 · Kimi K2.7-Code MCP Atlas 76.0 · Opus 4.6 MCP Atlas 63.2 | — | — | 2026 | V | ven-u |

**The pass^1→pass^4 decay column is the external echo of the host's convergence
findings.** glm-5.2 decays hardest of its cohort (37.1 → 13.4); the host
measured the same model as "cannot converge / cannot stop" (its `measured` L2).
fable5 and opus4.8 tie at pass^1 39.69 but fable5 keeps 28.87 at pass^4 vs
opus4.8's 22.68 — a repeat-reliability difference invisible in single-run
scores. τ² classic domains (retail/airline/telecom) were **not run** for any
mid-2026 model (nulls in Sierra's S3 records) — recorded absence.

### 3.4 Planning / orchestration

| id | source | model | value | metric | prov |
|---|---|---|---|---|---|
| P01 | orchb | Gemini-3.1-Pro 0.573 · GPT-5.5 0.558 · DeepSeek-V4-Pro 0.547 · Claude-Opus-4.8 0.544 (glm-**5.1** 0.563 — not mapped to 5.2) | composite, n=100, no CIs, one-shot academic | I |
| P02 | — | **no maintained public instrument exists** (MARBLE dormant; τ² proxies only) | — | — | — |

OrchBench's spread (0.544–0.573) is narrower than any plausible error bar —
treat as "no external discrimination on this axis." The host's repeat-ratio
metric (`band-ladder.md`) remains the only discriminating instrument.

### 3.5 Code review

| id | source | model | value | prov |
|---|---|---|---|---|
| V01 | crb | Claude Haiku 4.5 | 85.0 (coverage 88.8 / validity 81.2 / cross-file 89.1) | I-adj (LLM-judge) |
| V02 | crb | Gemini 3.1 Pro | 84.2 (77.1 / 91.3 / 80.5) | I-adj |
| V03 | — | Martian CRB scores **tools**, not models; academic review benchmarks list no 2026 frontier model | — | — |

**No reputable code-review benchmark covers any other fleet model.** The host's
own evidence (glm-5.2's measured review strength — two real bugs an L4 missed)
has no external counterpart. This is the largest capability gap in the matrix.

### 3.6 Browser / computer use

| id | source | model as listed | value | metric | prov | cluster |
|---|---|---|---|---|---|---|
| B01 | osw2 | claude-opus-4-8 (batched tool, max) | 20.6 binary / 54.8 partial | OSWorld 2.0, 500 steps | I | sn |
| B02 | osw2 | gpt-5-5 (batch, xhigh) | 13.0 / 49.5 | " | I | sn |
| B03 | osw2 | claude-sonnet-4-6 (std, max) | 8.3 / 41.5 | " | I | sn |
| B04 | osw2 | kimi-2-6 (mapped to kimi-k2.6, flagged) | 4.6 / 22.1 | " | I | sn |
| B05 | oswv | Claude Fable 5 85.0 · Claude Opus 4.8 83.4 · Gemini 3.6 Flash 83.0 · Claude Sonnet 5 81.2 · GPT-5.5 78.7 · Claude Sonnet 4.6 78.5 · Gemini 3.5 Flash 78.4 · Kimi K2.6 73.1 · Claude Opus 4.6 72.7 (orig OSWorld) · GPT-5.4 mini 72.1 · Claude Haiku 4.5 50.7 (orig) | success rate, **per-vendor harnesses, self-reported** | 2026 | V | ven-b |
| B06 | vendor OSW2 | Opus 5 70.6 · Fable 5 66.1 · GPT-5.6 Sol 62.6 · Opus 4.8 55.7 | OSWorld 2.0, **metric undisclosed — irreconcilable with B01's binary scale; never mix** | 2026-07 | V | ven-b |
| B07 | om2w | ABP + Claude Opus 4.6 | 90.53 | Online-Mind2Web, publicly verified artifacts | 2026-03 | I | om |

### 3.7 Research

| id | source | model as listed | value | metric | prov | cluster |
|---|---|---|---|---|---|---|
| R01 | hle-s | gemini-3.1-pro-preview (thinking high) | 46.44 ±1.96 | HLE no-tools | I | sl |
| R02 | hle-s | claude-opus-4-6-thinking-max | 34.44 ±1.86 | HLE no-tools | I | sl |
| R03 | hle-v | Opus 5 64.7 (56.3 no-tools) · Fable 5 63.9 (also quoted 64.5 — conflict recorded) · Opus 4.8 57.9 · GPT-5.6 Sol 53.6 (also 52.7) · Terra 50.4 · Luna 50.3 · GLM-5.2 54.7 · Opus 4.6 53.0 | HLE with-tools, per-vendor scaffolds | V | ven-r |
| R04 | bcomp | GPT-5.6 Sol 92.2 (Anthropic's comparison quotes 90.4 — conflict recorded) · Kimi K3 91.2 · Opus 5 90.8 · Sonnet 5 84.7 · Opus 4.8 84.3 · Opus 4.6 83.7 · DeepSeek V4 Pro 83.4 (max) / 80.4 (high) · Kimi K2.6 83.2 · DeepSeek V4 Flash 73.2 (max) / 53.5 (high) | BrowseComp, vendor-relayed | V | ven-r |
| R05 | — | GAIA/Gaia2/DeepResearch Bench: no 2026-fleet rows on any primary board (GAIA aggregator rows are explicitly "display only, not verified" — excluded) | — | — | — |

### 3.8 Long-horizon reliability

| id | source | model as listed | value | metric | prov | cluster |
|---|---|---|---|---|---|---|
| H01 | metr | Claude Opus 4.5 320 min [170, 729] · GPT-5 214 [117, 480] · o3 121 [74, 201] | 50% horizon, TH1.1, 95% CI | I | me |
| H02 | metr (graph-only, press-relayed) | Claude Opus 4.6 ~14.5 h [6, 98] · GPT-5.3-Codex (high) ~6.5 h [3, 17] · "Claude Mythos Preview (early)" 16+ h [8.5, 55] (at suite ceiling; METR flags >16 h unreliable; **not mapped to fable5**) | 50% horizon | Iᵖ | me |
| H03 | vb2 | Opus 4.6 $8,017.59 · Sonnet 4.6 $7,204.14 · Gemini 3.1 Pro (Custom Tools) $3,774.25 | VB2 mean final balance, 5 runs | Iᵖ (site 403'd) | an |
| H04 | vb2 | Opus 5 $11,182 (search-snippet only — **unverified**); Arena: Opus 5 ≈ GPT-5.6 Sol (near-tie 1st), Kimi K3 3rd | VB2 / Arena | Iᵖ | an |
| H05 | arena | claude-fable-5 1507 ±6 overall / 1553 ±9 coding (5,234 votes) · claude-opus-5-high 1493 ±6 / 1530 ±10 · claude-opus-4-8-thinking 1535 ±7 coding · claude-sonnet-4-6 1528 ±6 coding · kimi-k3-max 1542 ±12 coding | preference Elo (weight-capped 0.5; not task completion) | I | ar |
| H06 | — | METR has **no** measurement for fable5, opus5, opus4.8, sonnet5, or any OpenAI 5.5/5.6 under those names | — | — | — |

## 4. The matrix — evidence grade per model × capability

Grades per §6.3 (applied per capability across independent instrument clusters):
**A** ≥3 independent clusters w/ CI + tight agreement · **B** ≥2 independent ·
**C** 1 independent · **D** vendor-only · **—** no evidence. Cell shows grade +
headline independent value (rows cited). Compare within one instrument only.

| model (band) | coding | terminal | tool use | plan/orch | review | browser/CU | research | long-horizon |
|---|---|---|---|---|---|---|---|---|
| `fable5` (L4~) | **B** dswe .699 [C03,C31,C33,C34] | **A** tb21 .838±.012 [T01,T03,T10] | **C** τ³ 39.7/28.9 [U05] | — | — | **D** [B05,B06] | **D** [R03,R04] | **C** arena 1507/1553 [H05] |
| `opus5` (L4~) | **B** dswe .737 [C01,C32,C34] | **C** aa-tb .891 [T09] | **C** τ³ 48.7/32.0 — best in fleet [U01] | — | — | **D** [B06] | **D** [R03,R04] | **C**ᵖ VB2 unverified [H04,H05] |
| `opus4.8` (L4~) | **B** dswe .590 [C08,C32,C33,C34,C35] | **B** tb21 .789±.013 [T04,T10] | **C** τ³ 39.7/22.7 [U04] | **C** orchb .544 [P01] | — | **C** osw2 20.6/54.8 [B01] | **D** [R03,R04] | **C** arena [H05] |
| `sonnet5` (L2~) | **B** dswe .539 [C09,C32,C34,C35] | **B** tb21 .746±.016 [T07,T10] | — | — | — | **D** [B05] | **D** [R04] | — |
| `haiku4.5` (L1~) | **C** swb 66.6/64.7 [C26] | **C** tb20 35.5 (2025) [T11] | **C** bfcl4 68.7 — only fleet model on BFCL [U10] | — | **C** crb 85.0 [V01] | **D** [B05] | — | — |
| `opus4.6` (L3~) | **A** epoch .787 · swb 75.6 · pro 51.9±3.6 [C20,C23,C25] | **B** tb20 76.4 [T11] | **C** τ³ 27.3/11.3 [U08] | — | — | **C** om2w 90.5 [B07] | **C** hle-s 34.4±1.9 [R02] | **B**ᵖ metr ~14.5h · vb2 $8,018 [H02,H03] |
| `sonnet4.6` (L2~) | **C** epoch .752 [C21] | **C** tb20 53.4 [T11] | — | — | — | **C** osw2 8.3/41.5 [B03] | **D** | **C**ᵖ vb2 $7,204 [H03] |
| `gpt-5.5` (L4~) | **A** epoch .806 (pre-rel) · dswe .670 [C07,C19,C32,C33,C34,C35] | **B** tb21 .831±.011 · tb20 .822 [T02,T11] | **C** τ³ 44.6/29.9 [U03] | **C** orchb .558 [P01] | — | **C** osw2 13/49.5 [B02] | — ("GPT-5.5 Pro" rows not mapped) | — |
| `gpt5.6-sol` (L4~) | **B** dswe .727 [C02,C32] | **B** aa-tb .895 · fb 34.4 [T09,T10] | **C** τ³ 46.9/27.8 [U02] | — | — | **D** [B06] | **D** [R03,R04] | **C**ᵖ VB Arena [H04] |
| `gpt5.6-terra` (L3~) | **B** dswe .696 [C04,C32] | **B** tb21 .784±.013 [T05,T10] | — | — | — | — | **D** [R03] | — |
| `gpt5.6-luna` (L2~) | **B** dswe .672 [C06,C32] | **B** tb21 .757±.013 [T06,T10] | — | — | — | — | **D** [R03] | — |
| `gpt5.3-spark` (L1~) | — | — | — | — | — | — | — | — |
| `gpt5.4-mini` (L1~) | — | — | — | — | — | **D** oswv 72.1 [B05] | — | — |
| `gpt-oss-120b` (L2~) | **B** swb 26.0 · aider 41.8 · lcbp 1299 [C27,C28,C30] | **C** tb20 18.7 [T11] | — | — | — | — | — | — |
| `gemini3.1` (L2~) | **A** epoch .756 · pro 46.1±3.6 · lcbp 2887 [C22,C24,C29; C14 excluded as outlier] | **B** tb21 .658±.017 · tb20 .802 [T08,T11] | **C** τ³ 26.0/9.3 [U09] | **C** orchb .573 [P01] | **C** crb 84.2 [V02] | — | **C** hle-s 46.4±2.0 — top of board [R01] | **C**ᵖ vb2 $3,774 [H03] |
| `gemini3.5-flash` (L2~) | **B** epoch .793 (high) · dswe .374 (medium — effort mismatch flagged) [C12,C16,C34] | — | — | — | — | **D** [B05] | — | — |
| `gemini3.5-flash-low` (L1~) | — (no external row distinguishes the low tier) | — | — | — | — | — | — | — |
| `gemini3.6-flash` (L2~) | **C** dswe .486 [C10] (vendor 49% agrees — rare convergence) | — | — | — | — | **D** oswv 83.0 [B05] | — | — |
| `gemini3.6-flash-low` (L1~) | — | — | — | — | — | — | — | — |
| `deepseek-v4-pro` (L3 measured) | **B** epoch .776 · fswe .31 · fcode .176 — **spread = harness sensitivity, mirrors host history** [C17,C32,C33] | — | — | **C** orchb .547 [P01] | — | — | **D** bcomp 83.4 [R04] | — |
| `deepseek-v4-flash` (L2~) | — | — | — | — | — | — | **D** bcomp 73.2/53.5 [R04] | — |
| `deepseek-chat` (L1~) | — (identity rot: alias re-points; historical rows measured V3.2) | — | — | — | — | — | — | — |
| `kimi-k2.6` (L2~) | **B** epoch .766 · fswe .28 · curb .476 [C18,C33,C34] | — | — | — | — | **C** osw2 4.6/22.1 (mapping flagged) [B04] | **D** [R04] | — |
| `kimi-k2.7-code` (L2~) | **B** dswe .305 · fcode .301 [C13,C32] | — | **D** MCP Atlas 76.0 [U11] | — | — | — | — | — |
| `kimi-k3` (L2 measured) | **B** dswe .685 · fcode .442 [C05,C32,C36ᵛ] | **D** vendor tb21 88.3 — no official row [T12] | **C** τ³ 37.1/17.5 [U06] | — | — | — | **D** [R03,R04] | **C**ᵖ VB Arena 3rd [H04] |
| `glm-5.2` (L2 measured) | **A** epoch .787±.019 · dswe .438 · curb .546 — **spread = harness sensitivity** [C11,C15,C33,C34,C37ᵛ] | **C** fb 5.1 [T10] (vendor tb21 81.0 conflict — refresh queue) | **C** τ³ 37.1 → 13.4 — steepest pass^k decay in cohort, **externally corroborates the host's measured "cannot converge"** [U07] | — (orchb ran glm-5.1) | — (host-only evidence: strong) | — | **D** hle-v 54.7 [R03] | — |
| cascades (X3/X4) | derived — no external identity; constituents' rows apply with §2.1 discounts | | | | | | | |

## 5. Binding-layer evidence (harness-matched rows)

Terminal-Bench and Frontier-Bench score agent+model pairs, so these rows attach
directly to fleet bindings (transfer factor 1.0):

| binding (nick) | direct external evidence |
|---|---|
| `claude:fable5` (Sable) | TB2.1 Claude Code 83.8±1.2 [T01]; FB v0.1 33.8 [T10] |
| `claude:opus4.8` (Beatrix) | TB2.1 Claude Code 78.9±1.3 [T04]; FB 21.1 [T10] |
| `claude:sonnet5` (Solvei) | TB2.1 Claude Code 74.6±1.6 [T07]; FB 14.6 [T10] |
| `codex:gpt-5.5` (Arlo) | TB2.1 Codex 83.1±1.1 [T02]; TB2.0 Codex CLI 82.2±2.2 [T11] |
| `codex:gpt5.6-sol` (Omar) | FB Codex 34.4 [T10] (aa-tb 89.5 is Terminus-2 → model-layer) |
| `codex:gpt5.6-terra` (Rufus) | TB2.1 Codex 78.4±1.3 [T05]; FB 20.8 [T10] |
| `codex:gpt5.6-luna` (Ursula) | TB2.1 Codex 75.7±1.3 [T06]; FB 14.3 [T10] |

**No external benchmark runs the ycode, opencode, or agy harnesses.** Every
binding on those tools inherits model-layer rows at the default transfer
discount (0.8; browser 0.6). The harness-transfer factor is a declared constant,
not a measurement — the same-model/two-harness experiment `band-ladder.md`
prescribes for gemini3.1 would begin to measure it.

## 6. What the external priors say against the host ladder (informative only)

- **Corroborations:** glm-5.2's τ³ pass^k collapse [U07] and DeepSWE/Epoch split
  echo the host's `measured` L2 ("strong coder, cannot converge"). fable5's
  arena+TB2.1 leadership is consistent with its L4~ peg. deepseek-v4-pro's wild
  harness spread [C17 vs C32] matches the host's harness-bug history with that
  model.
- **Tensions worth a bake-off:** `gemini3.1` holds grade-A coding evidence
  (Epoch .756, Pro 46.1±3.6, LCB-Pro 2887 — top-of-board HLE too) against an
  operator L2 demotion made under a disclosed harness confound — the external
  data strengthens the case for the second-harness re-run. `kimi-k3` and
  `opus4.8` sit closer together on independent coding boards than their L2/L4
  pegs suggest (though k3's τ³ decay supports the convergence concern that
  drove its L2). `opus4.6` (L3~, agy-only) carries some of the best independent
  evidence in the whole fleet (grade-A coding, METR ~14.5 h, VB2 top) — its
  declared band may be underselling it, per §7 a candidate for a gated run.
- **No external instrument measures** what the host's conductor metric measures
  (repeat ratio, delegation, knowing-when-to-stop). The absence is structural:
  planning/orchestration and code review are the two axes where host evidence
  is not just senior but essentially alone.

## 7. Benchmark gaps (explicit)

1. **Code review**: no reputable benchmark covers any 2026 frontier model
   (CodeReviewBench: 2 fleet models, LLM-judged; Martian scores tools).
2. **Planning/orchestration**: no maintained instrument; OrchBench is one-shot,
   CI-less, non-discriminating.
3. **Tool use**: BFCL V4 and MCP-Universe are stale (newest = Dec-2025 models);
   τ² classic domains not run for the mid-2026 generation; τ³ n is inferred,
   not stated.
4. **Long-horizon**: METR's newest values are graph-only (unretrievable as
   text); no METR row exists for fable5/opus5/opus4.8/sonnet5 or any GPT-5.5/5.6;
   >16 h is above METR's own reliability ceiling; Andon Labs blocks fetches.
5. **SWE-bench Verified**: official board stale (2026-02-27) and top-compressed;
   no independent reproduction of any vendor 5-series claim (95–96%) exists;
   Epoch stops at gpt-5.5-pre-release / has no opus4.8.
6. **OSWorld**: -Verified is now vendor-scored in practice; OSWorld 2.0 vendor
   numbers use an undisclosed metric irreconcilable with Snorkel's binary scale.
7. **Effort-tier blindness**: external sources rarely state reasoning-effort
   tiers, leaving `gemini3.5-flash-low`/`3.6-flash-low` (and partially
   `gpt-oss-120b` medium) unmeasurable externally.
8. **Zero-evidence models**: `gpt5.3-spark` (nothing under any Spark identity),
   `deepseek-chat` (identity rot — registry should migrate to explicit
   `deepseek-v4-flash`), `deepseek-v4-flash` (vendor BrowseComp only),
   `gpt5.4-mini` (one vendor OSWorld row).
9. **Aider polyglot frozen** (2025-11-20) and **LiveCodeBench (orig) stale**;
   LCB-Pro active but missing all 2026 Anthropic/OpenAI frontier models.
10. **Harness-transfer factors are declared, not measured**; no external source
    runs ycode/opencode/agy.

## 8. Refresh triggers (instantiating method §11)

Standing triggers 1–6 of the method apply. Specific watch items queued now:

| watch | trigger fires when | affects |
|---|---|---|
| Epoch SWE-V adds Anthropic-5-series / gpt-5.6 / opus4.8 | new rows in their data zip | coding cells for 6 models; tests vendor 95–96% claims |
| Scale SWE-bench Pro runs any 5-series model | leaderboard update | first scaffold-controlled Pro number vs the contested vendor 80.3/79.2 |
| METR publishes text/API values for the 2026 tracker adds | tracker or blog update | long-horizon for opus4.6→opus5/fable5; replaces every [Iᵖ] row |
| tbench.ai 2.1 adds Sol / K3 / GLM-5.2 official rows | leaderboard update | resolves the T12-vs-T10 vendor conflicts |
| Sierra re-runs τ² retail/airline/telecom on mid-2026 models | S3 nulls fill in | tool-use grade B for the whole frontier cohort |
| BFCL V5 or a V4 refresh with 2026 models | gorilla CSV update | tool-use for everything but haiku4.5 |
| Frontier-Bench v0.2 / TB Science 1.0 ships | announcement | terminal frontier spread |
| Andon Labs / openai.com become fetchable | direct retrieval succeeds | upgrades [Iᵖ]→[I] on VB2, confirms/kills the Opus 5 $11,182 snippet |
| fleet registry migrates `deepseek-chat` | baseline/models diff | closes the identity-rot gap |
| any host bake-off contradicts a grade-A cell by >0.2 | leaderboard/ladder run | §11 trigger 5 investigation |

## 9. Machine-readable instance (excerpt)

Schema `bashy-external-prior-v1` (defined in the method doc §10). The tables in
§2–§3 are the canonical raw store for v0; this excerpt shows the encoding, and a
full YAML export is the natural next mechanization step. Composites are omitted
per the v0 application note (per-instrument cells only).

```yaml
schema: bashy-external-prior-v1
algo: ebc-v1.0.0
retrieved: "2026-08-06"
matrix_doc: docs/external-benchmark-capability-matrix.md
rows:
  - id: T01
    source: tb21
    model_external: "Claude Code / Fable 5"
    model: fable5
    binding: claude-fable5          # harness-matched → binding layer
    capability: terminal
    value: 83.8
    normalized: 0.838
    ci: {half_width: 1.2}
    result_date: "2026-06-07"
    retrieved: "2026-08-06"
    provenance: independent
    harness: "Claude Code (Harbor, team-verified)"
    n: 89
    cluster: tb
  - id: U07
    source: tau3
    model_external: "GLM-5.2 (xhigh)"
    model: glm-5.2
    binding: null                   # Sierra scaffold ≠ any fleet tool → model layer
    capability: tool_use
    value: {pass1: 37.11, pass4: 13.40}
    normalized: {pass1: 0.3711, pass4: 0.1340}
    result_date: "2026-07-24"
    retrieved: "2026-08-06"
    provenance: independent
    harness: "tau2-bench, gpt-5.2 user-sim, 4 trials"
    n: 97          # inferred from denominators — flagged
    n_inferred: true
    cluster: si
  - id: C36a
    source: vendor-anthropic
    model_external: "Claude Fable 5"
    model: fable5
    capability: coding
    value: 95.0
    normalized: 0.950
    result_date: "2026-06-09"
    retrieved: "2026-08-06"
    provenance: vendor
    harness: "unstated vendor scaffold"
    cluster: ven-c
    note: "no independent reproduction exists; grade D; never averaged with independent rows"
unmapped_rows:
  - {source: swbpro, model_external: "Muse Spark 1.1", note: "no fleet identity"}
  - {source: metr, model_external: "Claude Mythos Preview (early)", note: "related to fable5 but not the same public model; not mapped"}
unreachable_sources:
  - {url: "https://openai.com/index/gpt-5-6/", status: 403}
  - {url: "https://andonlabs.com/evals/vending-bench-2", status: 403}
  - {url: "https://huggingface.co/spaces/gaia-benchmark/leaderboard", status: dynamic-no-render}
coverage:
  models: 26
  capabilities: 8
  cells_total: 208
  cells_with_independent_evidence: 65
  cells_vendor_only: 23
  cells_missing: 120
  models_without_any_evidence: [gpt5.3-spark, gemini3.5-flash-low, gemini3.6-flash-low, deepseek-chat]
```

## 10. Changelog

| date | trigger | algo | change |
|---|---|---|---|
| 2026-08-06 | initial publication (weave issue-42) | ebc-v1.0.0 | first matrix; 5-lane primary-source sweep; per-instrument cells, pooled composites deferred |
