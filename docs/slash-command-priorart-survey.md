# Prior-art slash-command survey → bashy coreutils / verb candidates

Survey date: **2026-06-30.** Inventoried the built-in `/slash` commands of five
agentic coding CLIs and evaluated which concepts warrant a new bashy
**coreutils tool** or **front-door verb**, vs. which are already covered or out
of scope.

| tool | version inspected | ~commands |
|------|-------------------|-----------|
| Claude Code | source extraction | ~70 |
| aider | `5dc9490` 2026-05-22 | ~40 |
| codex | `cfead68e` 2026-06-29 | ~45 |
| gemini-cli | `ae0a3aa` 2026-06-26 | ~30 (+ subcommands) |
| opencode | `797cb530` (dev) 2026-06-30 | ~20 |

## Framing: bashy is the shell, not the agent

A slash command is an **agentic-REPL** affordance. bashy is the **shell +
userland the agent runs *in***, not the agent itself — so the large majority of
slash commands are **N/A by design**, owned by whatever tool drives bashy:

- **session/context:** clear, compact, new, resume, fork, branch, rename,
  timeline, restore, export, share, /context, /copy-context
- **model/inference:** model, effort, fast, reasoning-effort, think-tokens,
  personality, chat-mode, architect/ask/code
- **UI/TUI:** theme, color, vim, keymap, statusline, shortcuts, voice, editor,
  corgi/pets, multiline-mode
- **account/limits:** login, logout, upgrade, usage, cost, stats, privacy,
  feedback, doctor-of-account
- **agent config:** memory, hooks, permissions, approvals, sandbox, agents,
  mcp, plugins, extensions, settings
- **LLM-driven actions:** init (generate AGENTS.md/CLAUDE.md), review,
  security-review, ultrareview, plan — these need a model, not a deterministic
  shell tool; they belong to the **conductor skill**, not a bashy binary.

## What bashy already covers

The handful of slash commands that *do* wrap a reusable, scriptable capability
mostly map onto existing bashy surface:

| slash concept | seen in | bashy equivalent |
|---|---|---|
| run shell + capture output | aider `/run` `/test` | native shell + **`bashy run`** (result envelope + advisor hints) |
| repo map | aider `/map` `/map-refresh` | **`yc repomap`** (`--budget`) |
| code search / references | (implicit in all) | **`yc symbols/search-symbols/refs/query`** |
| git commit / diff / undo | aider, claude, codex, opencode | PATH `git` + **`gh`** passthrough; `/undo` = `git revert/reset` |
| lint dirty files | aider `/lint` | project's own linter via the shell |
| skills list/run | claude, gemini, opencode, codex | **`bashy skills`** |
| background jobs / ps / stop | codex `/ps` `/stop`, claude `/tasks` | **`bashy jobs/fg/bg/kill`** (real-PID registry) |
| API keys / auth | (via env) | **`bashy secrets`** |
| schedule / cron | (none have it) | **`bashy schedule`** |
| structured machine output | (none have it as a flag) | **`--json` / `BASHY_AGENTIC`** |
| command discovery | help | **`bashy commands`** |

The cross-tool takeaway: bashy's existing verbs + `yc` + coreutils already
absorb every *deterministic* capability the five tools expose. The genuine gaps
are few.

## Genuine gaps → candidates

### 1. `fetch` — URL → clean markdown — **RECOMMEND** (coreutils tool)
- Prior art: aider **`/web`** (scrape → markdown → context), Claude web fetch,
  gemini browser.
- Gap: `curl`/`wget` exist but dump raw HTML; an agent wants readable markdown
  (docs, issues, RFCs) without a browser. Already flagged in
  `agentic-tooling-modernization.md` (curl/wget → httpie-class, structured).
- Shape: `fetch URL` → markdown on stdout; `--raw` (bytes), `--json` (status +
  meta + body), follows redirects with a timeout. Pure-Go (`net/http` + an
  MIT/BSD html→markdown lib); cross-platform.
- Value: **HIGH** for agents; composes in pipelines (`fetch … | grep`).

### 2. `tokens` — token accounting — **RECOMMEND** (coreutils tool)
- Prior art: aider **`/tokens`**, Claude **`/context`**, gemini **`/stats`**.
- Gap: agents budget context before reading; bashy has `yc repomap --budget`
  but no standalone counter. `tokens FILE…`/stdin → estimate (a `wc` for
  tokens), `--json`, per-file table, dir total.
- Shape: pure-Go BPE estimate (tiktoken-class, permissive port or a documented
  heuristic — fail-loud on the model not being a known encoder).
- Value: **MEDIUM-HIGH**; pairs with the `--budget` flows; cheap to add.

### 3. `doctor` — environment self-diagnostic — **RECOMMEND** (verb, modest)
- Prior art: Claude **`/doctor`**, gemini **`/bug`** (context dump), codex
  **`/status` `/debug-config`**.
- Gap: bashy has real, documented footguns — a PATH shim shadowing `sh`, a
  missing `external/bash-5.3` fixture symlink, engine build-tag availability,
  sibling-pin drift. `bashy doctor` checks these and prints a health table
  (+ `--json`), turning tribal knowledge in the docs into a command.
- Value: **MEDIUM**; reuses knowledge already written down.

### 4. `clip` — cross-platform clipboard — **OPTIONAL** (coreutils tool)
- Prior art: aider/claude/gemini **`/copy`**, aider **`/paste`**.
- Gap: `pbcopy`/`xclip`/`clip.exe` differ per-OS; one `clip` / `clip -o`
  normalizes. Minor; not agent-critical (agents pipe, not clip).
- Value: **LOW-MEDIUM** convenience.

## Explicitly NOT recommended

- **review / security-review / ultrareview / init / memory / plan** — LLM-driven;
  the **conductor skill** + `weave` are the home, not a deterministic binary.
- **commit / diff / undo as new verbs** — PATH `git` + `gh` already serve;
  agents run `git` directly.
- **session / model / UI / auth / mcp / hooks / plugins / extensions** —
  agentic-REPL state owned by the driving tool, out of bashy's scope by design.

## Recommendation

Add **`fetch`** and **`tokens`** as coreutils tools and **`doctor`** as a verb
(all `--json`/`BASHY_AGENTIC`-aware, brand-neutral, pure-Go, permissive deps).
`clip` is an optional convenience. Everything else the five tools expose is
either already covered by bashy's verbs/`yc`/coreutils or correctly out of scope
— confirming the design line: **bashy is the shell the agent runs in, so it
absorbs reusable capabilities and leaves session/model/UI/LLM affordances to the
agent.**
