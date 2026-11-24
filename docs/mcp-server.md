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
- `--tools all` — all canonical registry commands and visible front-door verbs for this OS.
- `--tools NAME,...` — exactly the named commands; unknown names are an
  error and the server does not start.

Every profile additionally carries the registered commands and the `bashy`
script tool below.

`bashy mcp tools --json` includes a `definitions` array containing the actual
MCP `tools/list` objects for that profile, alongside the existing name arrays.
For a front-door verb, `bashy help --format mcp CMD` prints the same object as
`bashy mcp tools --tools CMD --json`. `--format skill` prints its existing help
as a skill document with that object in a closing `json mcp` fence; `--format
classic` prints the original help. `BASHY_HELP_FORMAT` also selects a format.
Root `bashy CMD --help` defaults to skill when fleet detects an agent tool or
`BASHY_AGENTIC=1`, otherwise classic, independently of terminal attachment.


With `--tools all`, visible front-door verbs are also direct tools; each runs
as a bashy subprocess with the policy gate applied before dispatch. They can
also be selected by name with `--tools`. Builtins stay script-only, and
`run_tool` stays registry-only.

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

To write one of these entries non-interactively, see the install-agent usage;
it registers the absolute path of the running bashy instead of the bare name,
so a session whose PATH lacks the install dir still starts the server.

## Outpost tunnel app

The `bashy-mcp` service runs `bashy mcp serve --transport http --outpost`
on loopback port 22750. Outpost publishes it while the service is active,
with login required and trusted identity enabled; it has no mesh exposure.
The local outpost admin proxy and the legacy loopback trust escape hatch
cannot reach this app. `bashy service disable bashy-mcp` withdraws it.

`--outpost` requires HTTP and rejects every request without a fresh outpost
identity HMAC for the `bashy-mcp` app. The key is provisioned by outpost in
its private `service_sso` configuration and re-read for each request, so
provisioning and rotation need no MCP restart. Missing keys, invalid
signatures, empty callers and conflicting caller headers fail closed. The
key is never a command-line argument or a client-visible credential.

The forwarded role grants no effects. The existing MCP policy and audit
still apply: `destroy`, `spend`, `cred` and `priv` require explicit host-side
approval (`--allow` grants); the shipped service grants none. Digest-bound
per-call approval is handled by the gateway/conductor approval workflow.
The non-loopback bind refusal also applies with `--outpost`.

## Hosted runner

`bashy mcp serve --transport http --hosted` serves a caller who has no host
of their own. It is the third profile beside `--outpost` and `--tools site`
and excludes both. It listens on loopback only.

Every HTTP request carries one per-call identity signed by the master:

```
X-Bashy-Hosted-Identity: v1.<payload>.<signature>
```

`payload` is the unpadded base64url JSON below; `signature` is the unpadded
base64url HMAC-SHA256 of `v1.<payload>` under the runner key.

| Field | Meaning |
|---|---|
| `v` | `bashy-hosted-identity-v1` |
| `caller` | the accountable principal of the call |
| `task_id` | the master's task for the call |
| `idempotency_key` | executes at most once on this runner |
| `secrets_token` | optional `secrets:read` token for this call only |
| `iat`, `exp` | Unix seconds; `exp - iat` is at most five minutes |

The runner key is the file named by `BASHY_HOSTED_KEY_FILE`, else
`bashy/hosted-runner.key` under the user configuration directory. It must be
a private regular file (mode 0600) of at least 16 bytes and is re-read for
each request, so provisioning and rotation need no restart. A request with a
missing, unsigned, expired, over-long, future-dated or incomplete identity, a
second identity header, a missing or public key file, or a non-loopback peer
answers 401. The key is never a command-line argument.

Each request gets its own server bound to the verified caller. An executing
`tools/call`:

- claims its `idempotency_key`; a second executing call with the same key
  answers `{state: "replayed"}` and runs nothing. The cache lives in the
  runner process; the master keeps the durable record;
- runs as a child process of the same binary in a scratch tree made for the
  call under `BASHY_HOSTED_SCRATCH_DIR` (default: the temporary directory)
  and removed when the call returns. `HOME`, `BASHY_HOME`, `TMPDIR` and the
  XDG directories point into it; the working directory is its `work`
  directory, and a `dir` argument must stay inside that;
- inherits only `PATH`, locale and time zone from the runner. The call sees
  `BASHY_PRINCIPAL` (the caller), `BASHY_SESSION` and `BASHY_HOSTED_TASK_ID`
  (the task), `BASHY_HOSTED_IDEMPOTENCY_KEY` and `BASHY_HOSTED_SCRATCH`. A
  client `env` argument cannot override these values. The runner's registered
  commands stay callable through a read-only `BASHY_COMMANDS_PATH`, and any
  `BASHY_ADAPTER_CACHE` or `BASHY_BIN_CACHE` set on the runner passes through
  unoverridable;
- with a signed `secrets_token`, lists connected credentials and uses the
  `secret exec --no-cache` projection path. Google Workspace files are
  written with mode 0600 under the call's `GWS_CONFIG_ROOT` and removed
  with the scratch tree. `bashy google auth-status --vault` uses the
  call's vault listing. GitHub, AWS and gcloud environment variables are
  supplied when the vault names identify one account for that provider.
  The signed vault token stays in the runner for general tools. The
  registered `trove` tool receives it in `BASHY_SECRETS_TOKEN` to read
  its layout from the caller's vault;
- stops at `--timeout` (default 1m): the child's process group is killed and
  the call answers exit 124;
- returns at most `--max-output` bytes of each stream; stderr names what was
  dropped.

`shell_open`, `shell_exec` and `shell_close` are not served, and neither are
the tools that read the server's own home (`db_lifecycle`, `kb_memory` and
its retrieval and maintenance tools).

The effect policy is unchanged. A `destroy`, `spend`, `cred` or `priv` call
answers `{state: "gated"}` before it spends its identity or creates a scratch
tree. The audit line of every call, gated ones included, names the caller as
`actor.human` (the field `BASHY_PRINCIPAL` fills for a local shell) and the
task as `actor.session`.
