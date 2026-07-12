# AGENTS.md

This is a pointer file for tools that look for `AGENTS.md` (Codex, OpenCode,
etc.). All guidance lives in [`CLAUDE.md`](CLAUDE.md) — read that.

<!-- BEGIN bashy lexicon (generated — do not edit by hand) -->

## Project lexicon

**In this workspace the words below are bashy verbs and agent bindings — NOT their
English senses, and not vendors' products.** When you see one, it names a thing on
this machine. Resolve it; do not assume.

    bashy lexicon resolve <term> --json     # what does this word mean HERE?

The sentence this vocabulary exists for, and its exact form:

    "handoff this to codex"   →   bashy handoff --to codex -m "<why, for the successor>"
    "resume it"                →   bashy resume

**Agent tools** (host-specific — each names a CLI *with the model bound to it here*;
the SAME word denotes a different binding on another machine):

- **agy** — the agy CLI on this host; bound as: agy-gemini3.1 (gemini3.1)
- **aider** — the aider CLI on this host; bound as: aider-deepseek-v4 (deepseek-v4), aider-kimi-k2.7-code (kimi…
- **claude** — the claude CLI on this host; bound as: claude-fable (fable), claude-opus (opus)
- **codex** — the codex CLI on this host; bound as: codex-gpt-5.5 (gpt-5.5)
- **opencode** — the opencode CLI on this host; bound as: opencode-deepseek-v4 (deepseek-v4), opencode-kimi-k2.7-c…

**Verbs:**

- **dag** — agent-first markdown DAG task runner (cross stage)
- **gate** — does this project pass? (the one command that decides) (test stage)
- **handoff** — pause this session and hand the work to another agent, a scheduler, or tomorrow (cross stage)
- **invoke** — invoke ONE agent, ONCE, on one instruction (the primitive every orchestrator is built on) (code s…
- **kb** — host-shared knowledge base: search before a task, add/retro after (all agents, all repos) (cross …
- **meet** — multi-participant deliberation session with a notes-only secretary (plan stage)
- **resume** — pick up a handed-off session — any tool, any machine (cross stage)
- **sdlc** — route intake issues through agentic implementation and deployment gates (deploy stage)
- **skills** — tier-2 workspace skills, env-gated: list applicable here, probe the coordinate, show one (cross s…
- **sprint** — cross-repo plan/continuity board (peer to weave) (plan stage)
- **weave** — per-repo multi-agent workspace orchestrator (code stage)

This is a selection, not the whole vocabulary — the rest is reached by lookup, above.
In written artifacts a term may be marked `[[handoff]]` so it is unmistakable; in
conversation it is used plainly, like any jargon. The marker is optional emphasis,
never required syntax.

<!-- END bashy lexicon -->
