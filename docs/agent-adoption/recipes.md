# Agent adoption recipes — use bashy as your agent's shell

One command wires a coding agent to run its shell commands through bashy:

```sh
bashy install-agent            # status of every known agent
bashy install-agent <agent>    # wire it (claude | opencode | aider | gemini | copilot)
bashy install-agent <agent> --check      # verify the wiring + the agent's exact invocation shape
bashy install-agent <agent> --uninstall  # reverse it
```

By default the running bashy binary itself is installed as the shell; pass
`--shell /path/to/bash` to use the pure drop-in instead. `--project` writes
project-level config (claude: `.claude/settings.json`; opencode:
`./opencode.json`) instead of user-level.

Verification status for every recipe: `matrix.md`.

## Claude Code (E2E verified, unix)

`bashy install-agent claude` merges into `~/.claude/settings.json`:

```json
{ "env": { "CLAUDE_CODE_SHELL": "/path/to/bashy" } }
```

Restart claude. Its Bash tool — including rc-snapshot generation, which uses
the `bash -c -l '<script>'` shape — runs through bashy.

Windows: `CLAUDE_CODE_SHELL` is ignored by Claude Code; the experimental
route is `CLAUDE_CODE_GIT_BASH_PATH` pointed at `bash.exe` (untested — see
matrix.md).

## OpenCode (E2E verified)

`bashy install-agent opencode` merges into
`~/.config/opencode/opencode.json`:

```json
{ "shell": "/path/to/bashy" }
```

Note the plain string — an object `{path, args}` form fails config
validation (v1.17.10).

## Aider (shape verified)

Aider takes its shell from `$SHELL` (spawned as `shell -i -c <cmd>` under a
PTY via pexpect):

```sh
SHELL=/path/to/bashy aider
```

`bashy install-agent aider --check` probes the `-i -c` shape.

## Gemini CLI / Copilot CLI (shim mechanism verified)

Both spawn a bare `bash` resolved via PATH on unix.
`bashy install-agent gemini` (or `copilot`) writes `~/.bashy/shims/{bash,sh}`
symlinks; launch the agent with the shim dir prepended:

```sh
PATH="$HOME/.bashy/shims:$PATH" gemini
```

## Codex CLI (blocked on macOS)

codex-cli (verified v0.142.5) executes commands in `/bin/zsh` by absolute
path on macOS — no config key, no PATH shim reaches it. Tracked as upstream
work (a portable-bash backend beside codex's ZshFork). Linux unverified.

## Cline / Cursor (deferred)

VS Code terminal profiles can point at bashy, but the shell-integration
escape-sequence injection script is unverified against bashy — deferred
until that compat item lands.
