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
| OpenCode | `opencode.json` `"shell": "<path>"` — a plain **string** (an object `{path,args}` form fails config validation; verified v1.17.10) | configured shell `-c` | **E2E PASS** — `opencode run` executed through bashy (`$0=bash`, `$BASH_VERSION` set) |
| Aider | `$SHELL` | `pexpect.spawn(shell, ["-i","-c",cmd])` under a PTY | **PTY-shape PASS** — exact spawn replayed via aider's own pexpect (PTY, `-i -c`), correct output + exit 0; live LLM session not exercised |
| Codex CLI | **login shell** — reads `/etc/passwd` `pw_shell` (getpwuid_r), NOT `$SHELL`/PATH/config, and derives the shell TYPE from the filename stem, then runs `<shell> -lc` (verified from `codex-rs/shell-command/src/shell_detect.rs`) | `<login-shell> -lc 'cmd'` | **REACHABLE via `chsh`** (was "blocked") — set the login shell to a bash/zsh-named bashy shim; `bashy install-agent codex` writes the shim + prints the (invasive, global) `chsh` recipe. DYLD interposition blocked (SIP + hardened runtime) |
| Gemini CLI / Copilot CLI / Antigravity (`agy`) | none (PATH shim on unix) | bare **`bash -c 'cmd'`** via PATH (`shell:false`; `gemini-cli/.../shell-utils.ts:698`, never reads `$SHELL`); interactive-PTY path → login shell | **shape PASS** + shim mechanism verified (`PATH=~/.bashy/shims` resolves bashy); `install-agent {gemini,copilot,agy}` |
| Cline / Cursor | VS Code terminal profile | interactive + shell-integration escape injection | **deferred** — requires VS Code shell-integration script compat |

**Two layers now force bashy:**

1. **The launcher (automatic).** `coreutils/pkg/chat` `execRunner` — the shared
   spawn path for `bashy chat`/`meet`/`weave`/`sdlc` — injects a bashy-shell env
   into every agent it launches: `PATH=~/.bashy/shims:…` (bare-name `bash`/`sh`/
   `zsh` → bashy; catches agy + opencode-unconfigured), `SHELL=<bashy>` (aider),
   `CLAUDE_CODE_SHELL=<bashy>` (claude). On by default; `BASHY_FORCE_AGENT_SHELL=0`
   disables. codex is the exception (reads `/etc/passwd`, not env) — see below.
   `bashy meet start --dry-run` reports each participant's routing status.
2. **`bashy install-agent <agent> [--project] [--check] [--probe] [--uninstall]
   [--yes]`** (`internal/agentos/installagent.go`) — makes the wiring durable for
   direct/interactive use: claude/opencode get config-file writes (JSON-merge,
   atomic, reversible), gemini/copilot/**agy** get `~/.bashy/shims/{bash,sh,zsh}`,
   aider gets `SHELL=` guidance, **codex** gets a bash-named shim + the `chsh`
   recipe (`--yes` attempts `chsh`; never sudo-edits `/etc/shells`). `--check`
   probes the exact invocation shape (free); **`--probe`** runs the agent LIVE and
   asserts bashy handled its shell (one LLM call).

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

### F4 — OpenCode config is a string, live-verified

Research suggested `"shell": {"path", "args"}`; v1.17.10 validates `shell`
as a plain string. With `"shell": "<bashy bash>"` a live `opencode run`
executed its command through bashy. The first attempt timed out on model
latency — not a shell hang; the retry completed normally.

### F5 — Codex CLI is reachable via the LOGIN shell (corrected 2026-07-08)

Reading the codex-rs source (`shell-command/src/shell_detect.rs`,
`core/src/shell.rs`) corrects the earlier "blocked" conclusion. codex resolves
its shell from the **`/etc/passwd` login shell** (`getpwuid_r` `pw_shell`) — it
does **not** read `$SHELL` (a `SHELL=… codex exec …` probe still ran `/bin/zsh
-lc`, verified). It derives the shell TYPE from the filename stem
(`detect_shell_type`: a path ending in `bash`→Bash, `zsh`→Zsh) and runs
`<shell> -lc`. So the lever is `chsh` to a **bash/zsh-named** bashy shim
(`~/.bashy/shims/bash → bashy`), added to `/etc/shells`. `bashy install-agent
codex` writes the shim and prints the (invasive — global login shell) recipe.
DYLD interposition stays unusable (codex has hardened runtime + no
`allow-dyld-environment-variables`, and SIP strips `DYLD_*` for `/bin/*`). The
launcher can't fix codex per-process (it ignores env), so it's the one agent that
needs this durable step; for text-only turns (e.g. `bashy meet`) codex's shell is
moot.

## Next verifications

1. Live aider LLM session with `SHELL=` pointing at bashy (shape already
   replayed under aider's own pexpect).
2. Live Gemini/Copilot CLI runs behind the shim dir.
3. Windows: `CLAUDE_CODE_GIT_BASH_PATH` → `bash.exe` experiment on a
   Windows fleet host.
4. Codex CLI on Linux (does it use bash there?).
5. VS Code shell-integration injection script tolerance (gates Cline/Cursor).
