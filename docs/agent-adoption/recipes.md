# Agent adoption recipes — use bashy as your agent's shell

**Automatic for bashy-launched agents.** `bashy meet`/`chat`/`weave`/`sdlc` spawn
agents through the `coreutils/pkg/chat` launcher, which force-injects the shell
env (`PATH=~/.bashy/shims:…`, `SHELL=<bashy>`, `CLAUDE_CODE_SHELL=<bashy>`) into
every child — so agents you launch *through bashy* already run their shell under
bashy (on by default; `BASHY_FORCE_AGENT_SHELL=0` disables). `install-agent` below
is for making it durable when you launch the agent **directly**.

One command wires a coding agent to run its shell commands through bashy:

```sh
bashy install-agent            # status of every known agent
bashy install-agent <agent>    # wire it (claude | opencode | aider | gemini | copilot | agy | codex)
bashy install-agent <agent> --check      # verify the wiring + the agent's exact invocation shape (free)
bashy install-agent <agent> --probe      # verify LIVE: run the agent once, confirm bashy handled its shell
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

## Gemini CLI / Copilot CLI / Antigravity `agy` (shim mechanism verified)

All spawn a bare `bash -c` resolved via PATH on unix (gemini-family
`run_shell_command`, `shell:false`, never reads `$SHELL`).
`bashy install-agent gemini` (or `copilot`, or `agy`) writes
`~/.bashy/shims/{bash,sh,zsh}` symlinks; launch with the shim dir prepended:

```sh
PATH="$HOME/.bashy/shims:$PATH" agy
```

(The launcher does this automatically for `bashy meet`/`chat`/`weave`.)

## Codex CLI (reachable via the login shell — invasive)

codex reads the **`/etc/passwd` login shell** (`getpwuid_r` `pw_shell`), not
`$SHELL`/PATH/config, and keys the shell type on the filename stem, then runs
`<shell> -lc` (verified from `codex-rs/shell-command/src/shell_detect.rs`). So the
lever is `chsh` to a bash/zsh-named bashy shim:

```sh
bashy install-agent codex          # writes ~/.bashy/shims/bash -> bashy, prints the recipe
echo "$HOME/.bashy/shims/bash" | sudo tee -a /etc/shells   # once
chsh -s "$HOME/.bashy/shims/bash"  # or: bashy install-agent codex --yes (after the /etc/shells line)
```

This changes the login shell for **all** sessions (Terminal, ssh) — it is the
one invasive recipe. For text-only agent turns (e.g. `bashy meet`) codex's shell
is moot, so this is only needed when codex will actually run shell commands.
DYLD interposition is blocked (SIP + hardened runtime).

## Codex CLI on Linux

Same login-shell mechanism; the fallback chain differs (`get_shell_path` tries
`which(bash/zsh)` before the hardcoded `/bin/*`), so a PATH shim can also win when
the passwd shell type doesn't match. `chsh` remains the robust lever.

## Cline / Cursor (deferred)

VS Code terminal profiles can point at bashy, but the shell-integration
escape-sequence injection script is unverified against bashy — deferred
until that compat item lands.
