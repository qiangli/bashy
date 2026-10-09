# examples/fleet — custom model, tool and agent definitions

bashy's built-in fleet lists hold only curated entries. Everything else is a
**custom** definition: you add it yourself with `bashy tool add`,
`bashy model add` or `bashy agent add`, and it lives in your local store. This
folder holds ready-made definitions for those, and doubles as the test corpus
for the add path.

```sh
bashy tool add examples/fleet/tools/qwen-code.yaml
bashy tool list --custom
bashy tool verify qwen-code
```

`--force` is needed for a name that still belongs to a built-in entry. That
applies to aider, cline, gemini, goose, hermes and openclaw until those
built-in entries are retired.

## Layout

| Folder | Holds |
|---|---|
| `tools/` | third-party agent CLIs: how bashy launches each one headless |
| `models/` | models outside the curated list: older generations, local Ollama tags, OpenAI-compatible endpoints |
| `agents/` | `tool:model` bindings outside the curated L3–L5 families |

## File header

Every file starts with a comment block. It records:

- **source** and license
- **install** command
- **add** line: the exact `bashy … add` command
- **models**: how the tool picks a model and an endpoint
- **status**: either `verified: <date> on <tool version>` (with the e2e record it came from) or `unverified`, with the reason

A file is stamped verified only after a live run of the bashy agent e2e lane passed. That run uses bashy delegate, a real model, and a task with a pass/fail check.

## Launch contract fields these examples use

A custom tool definition can carry everything a third-party CLI needs, so
nothing about a specific CLI lives in bashy's code:

- **`key_env`** names the env var that receives the bound model's
  credential, e.g. `OPENAI_API_KEY`, whatever the model's `api_key_ref` is.
- **`env`** holds `KEY=VALUE` pairs. Values may use `{model}`, `{base_url}`,
  `{base_url_origin}`, `{base_url_path}` and `{state_dir}`. A tool that
  reads its model from the environment declares `{model}` here.
- **`setup`** is a Bash# snippet bashy runs before every launch. Use it
  to write the CLI's provider config into `{state_dir}` (bashy's private
  per-binding copy, so your own config of that CLI is never touched).
- **`{model}`** may also sit inside an argument, e.g. `-m bashy/{model}`.

## Tool status (2026-10-09)

**How verified:** live runs with `bashy delegate` on macOS arm64 against
three endpoints: an OpenAI-compatible cloud endpoint (glm-5.3), local
Ollama (qwen3.5:9b), and the bashy model door (qwen3:8b). Each tool got two
tasks: `pong` (the endpoint answers) and `calc` (fix a one-line bug; its test
must pass). Per-tool results are in each file's header.

| Tool | Status |
|---|---|
| mini-swe-agent, openhands, cline | verified, all six runs pass |
| qwen-code, goose, crush, forge | verified; only `calc` via the door's qwen3:8b fails |
| hermes | verified on cloud and Ollama; it needs a 64K context window, more than the door's qwen3:8b offers |
| openclaw, aider | verified for `pong`; partial `calc` (see the file headers) |
| droid, copilot | unverified: they need a vendor account |
| junie, grok, gemini | unverified: they need a vendor account or a protocol shim |

Definitions here must be safe to publish. Never put real hostnames, tokens,
vault values or private endpoints in them; use `127.0.0.1` or the vendor's
public URL, and name credentials only by `api_key_ref`.
