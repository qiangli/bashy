# Hermes Agent YAML CLI adapter

A **bounded** projection of the Hermes Agent CLI, focused on the common agent
**session lifecycle**: start/continue a chat, pick the model, and resume a
previous session — plus `help`, `version`, `validate` and `completion`.

Two artifacts describe the same bounded surface:

- `profile.yaml` — the declarative `spec.interfaces.cli` contract. It is compiled
  and rendered by ycode (the generic bootstrap); no product-specific Go is added.
- `main.bsh` — a thin native Bash++ adapter that forwards substantive behavior to
  an **installed upstream** Hermes selected by `HERMES_BIN`.

Behavioral source pin: `ycode/priorart/hermes-agent` at
`3ba5602b6273f9bb0d2a0d52c77bbe0f50f6227b` (top-level parser
`hermes_cli/_parser.py`; subcommands `hermes_cli/subcommands/*.py`;
console script `hermes = hermes_cli.main:main`).

## Compatibility statement

This is a bounded subset, not a mirror of the (very large) `hermes` CLI.

### Supported (handled by the candidate from `profile.yaml`)

| Surface | Notes |
|---|---|
| `--help`, `help [command]` | Root, nested and alias help render from the contract. |
| `version` | Prints the candidate build identity. |
| `validate` | Strictly compiles this profile. |
| `schema` | Prints the profile JSON Schema. |
| `completion <shell>` | `bash`, `zsh`, `fish`, `powershell`. Upstream Hermes supports `bash/zsh/fish`; `powershell` is required by the generic contract and is emitted by the candidate. |
| usage errors → exit `2` | Unknown flag, or an out-of-enum `completion` shell. |

### Projected (declared here, executed only by forwarding to the upstream)

Source-backed session-lifecycle commands and flags are declared so help,
parsing, aliases and dispatch are exercised, but the agent loop is **not** run by
ycode — those operations dispatch `unsupported` and exit `4` under the candidate.
`main.bsh` forwards them (and the bare-prompt form) to the installed upstream.

| Surface | Upstream source |
|---|---|
| bare `hermes [PROMPT]` | default chat/start (`_parser.py` top-level + `_build_chat_parser`) |
| `chat` (alias `start`) with `-q/--query`, `--oneshot` | `_build_chat_parser` |
| `model` with `--refresh` | `subcommands/model.py` |
| top-level `-m/--model`, `--provider`, `--reasoning`, `-r/--resume`, `--yolo`, `--tui` | `_add_top_level_flags` |

`start` is a **bounded convenience alias** defined by this profile/adapter (it is
mapped onto upstream `chat`); it is not a native upstream alias. The alias rewrite
in `main.bsh` applies only when `start` is the leading verb.

### Unsupported / out of scope

Represented as an explicit `unsupported` exit `4` (when reached through a declared
command) or simply not projected:

- Interactive-only session controls exposed by Hermes as slash commands
  (`/init`, `/plan`/goal, `/exit`) — **not** batch CLI subcommands upstream, so
  they are intentionally not projected here.
- Hosted auth, gateway/dashboard/web, plugins, MCP, cron/kanban, `pause`/`resume`
  emergency-stop, and every vendor-, provider- and platform-specific command.
- The optional-value form of `-c/--continue` (bare `-c`) is not modeled; use
  `-r/--resume` for session resume.

No supported-looking option is silently ignored: anything ycode cannot itself run
fails `unsupported` (4), and the adapter forwards the invocation verbatim to the
real upstream instead of approximating it.

## Running the adapter

Set `HERMES_BIN` to an absolute path to the installed `hermes`; without it the
adapter resolves `hermes` from `PATH`. Argv, stdin/stdout/stderr and the exit
status are forwarded verbatim.

```sh
HERMES_BIN=/abs/path/to/hermes bashy --bashsharp examples/cli/hermes-agent/main.bsh chat -q "Explain this repo"
HERMES_BIN=/abs/path/to/hermes bashy --bashsharp examples/cli/hermes-agent/main.bsh start --oneshot -q "one and done"
```

Validate / inspect the declarative contract through ycode directly:

```sh
YCODE_BIN=/abs/path/to/ycode "$YCODE_BIN" --file examples/cli/hermes-agent/profile.yaml validate
"$YCODE_BIN" --file examples/cli/hermes-agent/profile.yaml --help
"$YCODE_BIN" --file examples/cli/hermes-agent/profile.yaml completion bash
```

## Offline fixture

`fixtures/run.sh` is the offline conformance runner. It makes **no** model or
network calls and uses a scratch runtime directory.

```sh
env YCODE_BIN=/abs/ycode BASHY_BIN=/abs/bashy /bin/sh examples/cli/hermes-agent/fixtures/run.sh
```

It performs **17 checks**:

1. strict `validate` of `profile.yaml`;
2. **11 candidate goldens** in `fixtures/goldens/` (stdout, stderr and exit
   class) covering root/nested/alias help, `completion --help`, `version`,
   declared `unsupported` dispatch (`chat`, `model`, bare prompt → exit `4`),
   usage errors (`completion telnet`, `chat --nope` → exit `2`) and
   `completion bash`;
3. **5 adapter transport checks** through installed Bashy against a **fake**
   upstream selected via `HERMES_BIN`: argv passthrough, the `start`→`chat`
   alias rewrite, stdin passthrough, stdout/stderr passthrough and exit-status
   passthrough.

The transport checks prove **only** argv/stream/status wiring of the Bash++
adapter; they are labeled `(fake upstream)` and are not evidence of real Hermes
behavior. Goldens are generated with `LC_ALL=C` against the frozen candidate.

`main.bsh` demonstrates **native Bash++** (typed variable declaration, control
flow, verbatim `exec` forwarding) with no fenced-Go helper, since none is used;
the fenced-Go polyglot path is demonstrated by the sibling `ycode` profile.
