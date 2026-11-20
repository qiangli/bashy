---
id: 96ff25a316fb
kind: feature
title: 'bashy <tool> shortcut: one command for a human to launch a steerable session (bashy codex, bashy agy, bashy claude, ...)'
seq: 3248
status: todo
priority: p2
labels:
    - agents
created: 2026-11-19T22:00:59.676054Z
---

Operator request 2026-11-19. Today a human needs: bashy chat --agent codex-gpt6.1-sol -i --yolo --cwd DIR, plus BASHY_CHAT_INBOX=off to avoid the mb backlog flood (yoke todo 5e01de0e), plus knowing the session id mangles dots (codex-gpt6.1-sol -> codex-gpt6-1-sol). 'bashy codex' / 'bashy agy' are currently 'command not found', so the names are free.

Shape (KISS):
- bashy <tool> [--model M] [--yolo] [prompt...] for every registered tool with a steer launch (claude, codex, agy, muse, opencode, ycode, zcode, dsh, kimi).
- Defaults: cwd = $PWD; model = the tool's configured default (operator pref, else the highest-band LIVE-VERIFIED binding for that tool); interactive + steerable; inbox prepend at the default cap; the tool's own permission prompts kept unless --yolo.
- Session id = the tool name (codex, agy, ...; suffix -2 if one is already live), printed with the exact steer/attach commands: 'steer it: bashy chat steer codex "..."'.
- Needs a real TTY: if stdin is not a terminal (e.g. run from an agent's ! shell) refuse with a clear message instead of starting a TUI that exits at once (observed today with codex).
- bashy <tool> -- <args> stays a raw passthrough to the vendor CLI.
- bashy <tool> --help shows the resolved model, binding and steer id (--explain/--dry-run supported).
Acceptance: 'cd repo && bashy codex' opens Codex's own TUI as a steerable session named codex; 'bashy chat steer codex "hi"' reaches it; non-TTY launch refuses with guidance; tests for name resolution, id, non-TTY refusal.

Operator decision 2026-11-19 (supersedes the defaults above): 'bashy <tool>' should behave like the user running the vendor CLI themselves in the repo, PLUS yolo — i.e. cwd = $PWD, the vendor's OWN default model and config (e.g. ~/.codex/config.toml model/effort; no bashy model override unless --model is given), the vendor's normal home/sign-in, and permission bypass ON by default (--yolo semantics; --no-yolo to keep the vendor's prompts). Still steerable with the tool-name session id, capped inbox, non-TTY refusal, and '--' raw passthrough.

Operator addition 2026-11-19: a ZERO-TOKEN e2e for every tool shortcut — a test-owned fake vendor CLI (argv recorder + line echo, modelled on agent-bench scripts/benchbot.py) bound via BASHY_TOOL_BINARY_<TOOL>; asserts launch through the bashy chat steerable PTY path, session id (tool / tool-2), steer receipt, yolo default and --no-yolo, non-TTY refusal, and '--' passthrough. Real-vendor checks stay manual (operator runs bashy <tool>, conductor steers).

Operator test 2026-11-19 (bashy f151dfb9 + yoke f4a1c2f2, one tool at a time, conductor steering): claude PASS (Opus 5.5), codex PASS on gpt-6-astra (gpt-6.2-* blocked by the 0.162.0 pin, todo caabac24), agy PASS (agy default Gemini 4 Argon; interrupt needed to skip its bashy session-open routine), muse PASS (Muse Spark; its picker lists muse-wonder-full-1.0 / muse-ember-1.0 / muse-spark-1.3), opencode PASS (deepseek-v4-pro). ycode NOT covered: 'bashy ycode' is the pre-existing built-in ycode verb (embedded engine), so the shortcut correctly yields to it — but that verb opens no steerable bashy chat session (no 'ycode' in chat sessions) and rejects --dry-run ('invalid command flags'). Follow-up: make the interactive 'bashy ycode' join as a steerable chat session named ycode (same id/inbox/yolo conventions), so all six tools are one-command steerable. zcode/dsh/kimi have no steer_exec (exit 127).
