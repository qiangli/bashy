# Agent adoption matrix — B0 verification results

Status of `bin/bash` (the pure Bash 5.3 drop-in) as the shell that coding
agents spawn. Each row records the agent's shell-selection surface, the exact
invocation shape it uses, and the verification level reached. Companion to
`recipes.md` (how to wire each agent) and the adoption strategy doc in the
umbrella.

Verification levels: **E2E** = a real headless agent session executed commands
through bashy; **shape** = the agent's exact argv shape verified against
`bin/bash` side-by-side with GNU bash; **unverified** = recipe designed, not
yet exercised.

| Agent | Shell selection | Invocation shape | Status (2026-07-07) |
|---|---|---|---|
| Claude Code | `CLAUDE_CODE_SHELL` env (settings.json `env` block); unix only | `bash -c env` (probe), `bash -c -l '<snapshot script>'` (rc snapshot), then per-command | **E2E PASS** — headless `claude -p` session ran its Bash tool through bashy end-to-end, snapshot generation included |
| Aider | `$SHELL` | `pexpect.spawn(shell, ["-i","-c",cmd])` under a PTY | **shape PASS** (`-i -c` parses + runs); PTY-session E2E pending |
| Codex CLI | none (PATH shim: `bash` resolved via execvp) | `bash -lc 'cmd'` | **shape PASS**; live PATH-shim run pending |
| OpenCode | `opencode.json` `"shell": {"path", "args"}` | configured shell `-c` | **unverified** (config exists upstream; recipe pending live run) |
| Gemini CLI / Copilot CLI | none (PATH shim on unix) | `bash -c 'cmd'` | **shape PASS** (plain `-c` is the conformance-suite baseline) |
| Cline / Cursor | VS Code terminal profile | interactive + shell-integration escape injection | **deferred** — requires VS Code shell-integration script compat |

## Findings

### F1 — `bash -c -l 'cmd'`: options after `-c` (FIXED)

Claude Code generates its rc snapshot with `bash -c -l '<script>'` — options
*after* `-c`. GNU bash keeps parsing options until the first non-option
argument (bash 5.3 `shell.c`: `want_pending_command`;
`command_execution_string = argv[arg_index]` only after the option loop).
bashy's Go flag parsing bound `-c` to the immediately following token, so
`-l` became the command string → `line 1: -l: command not found`, exit 127,
on **every** Bash tool call.

Fixed by `relocatePendingCommandFlag()` in `internal/cli/main.go` (argv
rewrite before flag parsing; unit-tested, side-by-side matrix vs GNU bash,
suite 86/86). This shape is now part of the CLI unit tests — it is the
single highest-value agent-compat fixture found so far.

### F2 — `CLAUDE_CODE_SHELL` is honored (unix)

Verified with an argv-logging shim: every shell invocation of a headless
Claude Code session (probe, snapshot generation, per-command) went through
the configured binary. The one-line recipe works:

```json
{ "env": { "CLAUDE_CODE_SHELL": "/path/to/bashy/bin/bash" } }
```

Windows: the variable is ignored (upstream issue #25558); the Windows path
is the Git-Bash-replacement experiment (`CLAUDE_CODE_GIT_BASH_PATH`), still
to run.

### F3 — rc snapshot round-trip

An existing Claude-Code-generated snapshot (93 lines: `unalias -a`,
`declare -f` function dumps, shopt/alias replay) sources cleanly in bashy,
and after F1 the *generation* path (running the snapshot-builder script via
`-c -l`) works too, proven by the E2E run.

## Next verifications

1. Live aider session with `SHELL=` pointing at bashy (PTY + pexpect).
2. Live OpenCode run with `shell.path` config.
3. Live Codex CLI run behind a PATH-shim dir.
4. Windows: `CLAUDE_CODE_GIT_BASH_PATH` → `bash.exe` experiment on a
   Windows fleet host.
5. VS Code shell-integration injection script tolerance (gates Cline/Cursor).
