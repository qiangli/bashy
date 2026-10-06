# `bashy mcp serve`: bashy as an MCP server

`bashy mcp serve` exposes bashy's command atlas to agents over the Model
Context Protocol, so non-Go agents can drive the same pure-Go userland the
shell runs in-process. Protocol behavior follows MCP 2026-07-28 (empty
capability set; tools inferred from the registered tool set), built on the
official MCP go-sdk. The server keeps no stream state: the HTTP transport is
stateless Streamable HTTP, and a stdio client closing stdin is a clean
shutdown. Durable state exists only where the tools say so (shell sessions,
below).

Full synopsis:

    usage: bashy mcp serve [--transport stdio|http] [--listen ADDR] [--allow EFFECTS]
                           [--tools default|all|NAME,...] [--max-output BYTES]

## Compatibility floor: `list_tools`, `run_tool`, `server_info`

Every profile exposes these three tools:

- `list_tools` — every command this build ships (name, synopsis, usage, plus
  the atlas group/caps where known). Takes no arguments.
- `run_tool` — run one command: `name` plus `args` (and optional `stdin`,
  `dir`, `env`). Returns `stdout`, `stderr`, `exit_code`. Execution is
  in-process and pure Go. Unknown commands fail with exit 2.
- `server_info` — identity and policy: `name`, `version`,
  `protocol_version`, `tools_sha256`, `allowed_effects`. The hash covers the
  canonical JSON of the live tools/list result, so it includes tools
  registered after startup.

## Direct per-command tools: `--tools`

Beyond the compat pair, registry commands can appear as individual typed
tools (per-flag schemas plus `stdin`/`dir`/`env`):

- `--tools default` — the available core commands: canonical registry
  commands for this OS, aliases excluded.
- `--tools all` — all canonical registry commands for this OS.
- `--tools NAME,...` — exactly the named commands; unknown names are an
  error and the server does not start.

Every profile additionally carries the registered commands and the `bashy`
script tool below.

## The `bashy` script tool

`bashy` runs literal script bytes through the bashy interpreter — the same
session runner as the shell front door, parsed once — with `script`, `stdin`,
and `dir` inputs. It carries the `exec` effect, so the policy below applies.

## Registered commands as typed tools

Commands added with `bashy commands add` appear as typed tools with their
declared schema (positionals, flags, types, enums, defaults). Structured
arguments become argv; `stdin`/`dir` pass through unless the schema declares
them. The server re-reads the command ring and emits
`notifications/tools/list_changed`, so clients see adds and edits live.

## Shell sessions: `shell_open`, `shell_exec`, `shell_close`

Stateful shells for multi-step work (each carries the `exec` effect):

- `shell_open` — `dir`/`env` inputs, returns a `session_id`.
- `shell_exec` — runs a script in that shell; returns `stdout`, `stderr`,
  `exit_code`, and the shell's current directory as `cwd`.
- `shell_close` — closes the shell and removes its output files.

`--max-output BYTES` (default 65536) caps inline streams. A stream past the
cap spills to a file and returns `{path, bytes, preview}` (preview: the first
2048 bytes) instead of text. Spilled files live until `shell_close`.

## Effect policy: `--allow`

Every command carries atlas effects. The privileged effects `destroy`,
`spend`, `cred`, and `priv` are denied unless granted:

    bashy mcp serve --allow destroy,spend

A denial fails with exit 126 (`denied: effect <name> requires --allow
<name>`); policy never guesses. Every tools/call — allowed or denied — is
recorded in the same hash-chained audit as shell dispatch.

## Transports

- stdio (default) speaks MCP on stdin/stdout.
- `--transport http` serves the same server at `/mcp` on a loopback address:
  `--listen ADDR` defaults to `127.0.0.1:0` (`localhost` maps to
  `127.0.0.1`). Anything not loopback is refused:

      refusing non-loopback bind without an authorization issuer, resource URI and audience (post-1.0)

- Remote use is stdio over SSH: `ssh HOST bashy mcp serve` runs the stdio
  server on the far end while the local client speaks MCP to that pipe.

## Client setup

Each snippet below was verified against the client it targets (its built-in
help, the config that client itself writes, or its published schema).

Claude Code:

    claude mcp add bashy -- bashy mcp serve

Codex (`~/.codex/config.toml`):

    [mcp_servers.bashy]
    command = "bashy"
    args = ["mcp", "serve"]

OpenCode (`opencode.json`):

    "mcp": {"bashy": {"type": "local", "command": ["bashy", "mcp", "serve"]}}

To write one of these entries non-interactively, see the install-agent usage.
