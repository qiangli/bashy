# mini-swe-agent — bounded self-contained example (shell + YAML)

A minimal, **self-contained** reconstruction of the [mini-swe-agent](https://github.com/SWE-agent/mini-swe-agent)
`mini` lifecycle, runnable from this directory through two equivalent
entrypoints. It is **not a wrapper around an installed upstream `mini`** — the
agent loop, the model interface, and the execution boundary are the local
implementation under `harness/`. **No benchmark results are claimed.**

## Upstream source pin

- Repository: `https://github.com/SWE-agent/mini-swe-agent`
- Source commit pin: **`04d809ceab9df28f9adaed044884180159172930`** (package version `2.4.6`)

The read-only reference bundle used to author this port is **not shipped**.

## Two entrypoints, one local loop

Both entrypoints run the SAME loop (`harness/minisweagent_bounded/cli.py`):

| Entrypoint | File | Parsing | Runs the loop? |
|---|---|---|---|
| Shell | `main.bsh` | the local CLI (flags-first, argparse) | **yes, by default** |
| YAML | `yaml-run.sh` + `profile.yaml` | **declaratively owned by ycode** (strict validation of the compiled contract) | yes, via the documented bridge |

### The example-local bridge (documented limitation)

The frozen generic ycode compiler strictly **validates and renders**
`profile.yaml` and owns the CLI parsing. Its frozen dispatch, however, **cannot
launch a mini loop** — so the run itself dispatches `unsupported` (exit 4).
`yaml-run.sh` uses ycode as the **strict parse gate**: it runs the invocation
through ycode, propagates a real usage error (exit 2) verbatim, and on the exit-4
"parsed-OK-but-not-runnable" signal it execs the same local `cli.py`. This is an
example-local bridge only: **no shared Go/schema change and no second
product-specific dispatch branch.** Matching the unsupported exit alone would not
be parity; the shared fixtures prove both paths execute real actions, persist
trajectories, and submit successfully.

## Run it

From this directory (`examples/cli/mini-swe-agent/`):

```sh
YCODE_BIN=/path/to/ycode
BASHY_BIN=/path/to/bashy      # or `bashy` on PATH

# Shell entrypoint (runs the local loop; here with an offline replay scenario):
BASHY_BIN=$BASHY_BIN bashy --bashsharp main.bsh \
    --scenario fixtures/scenarios/submit-success.json -y --emit-envelope

# YAML entrypoint (ycode validates/parses, then the same local loop runs):
YCODE_BIN=$YCODE_BIN BASHY_BIN=$BASHY_BIN /bin/sh yaml-run.sh \
    --scenario fixtures/scenarios/submit-success.json -y --emit-envelope

# Declarative surface rendered by the frozen ycode binary:
"$YCODE_BIN" --file profile.yaml validate
"$YCODE_BIN" --file profile.yaml --help
"$YCODE_BIN" --file profile.yaml completion bash

# A live run against a real OpenAI-compatible endpoint (needs a provider + key):
BASHY_BIN=$BASHY_BIN bashy --bashsharp main.bsh \
    -t "fix the failing test" --model-class openai -m gpt-4o-mini \
    --base-url "$OPENAI_BASE_URL" --api-key "$OPENAI_API_KEY"
```

### CLI surface (flags-first, mirroring upstream `mini`)

`-t/--task`, `-c/--config` (repeatable), `-m/--model`, `-y/--yolo`,
`-l/--cost-limit`, `-o/--output`, `--model-class`, `--agent-class`,
`--environment-class`, `--exit-immediately`. Helper verbs
(`version`/`validate`/`completion`/`schema`/`help`) render through ycode.
Bounded example extensions (declared in `profile.yaml` so both entrypoints parse
symmetrically): `--scenario`, `--replay`, `--emit-envelope`, `--step-limit`,
`--mode`, `--base-url`, `--api-key`. Unsupported `--model-class` /
`--agent-class` / `--environment-class` values **fail explicitly** (exit 2).

## Behavior

- **Interaction modes** — `confirm` (default), `yolo`, `human`; the `/c /y /u`
  mode switches, `/h` help, and `/m` multiline comment; confirm-on-exit.
- **Fail-closed** — confirm/human decisions and task prompting require a real
  terminal; with no TTY the run refuses to proceed (exit 4). Piped newlines are
  never treated as approval.
- **Ctrl-C** — a real SIGINT is caught; interactively it prompts for a
  comment/continue, non-interactively it stops with `UserInterruption`. The
  child process group is always reaped (no orphan).
- **Stateless actions** — every action runs as a fresh `bashy -c` process
  (explicit argv, `shell=False`, **no host `/bin/sh` fallback**); a `cd` or env
  change in one action never leaks into the next.
- **Limits & truncation** — cost, step, and wall-time limits; long command
  output is head/tail-elided.
- **Trajectory** — the linear `mini-swe-agent-1.1` trajectory is persisted to
  `-o/--output` on **both** successful and failed exits.
- **Submission** — the explicit `COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT` marker
  (first output line, return code 0) submits; a successful submission exits 0.

### Models: replay + live (honest cost)

- `ReplayModel` — deterministic offline replay of scripted assistant steps.
- `LiveModel` — a minimal **stdlib** OpenAI-compatible chat/completions transport
  (`urllib`, no SDK). The action is the last fenced bash block; a reply with none
  raises a format error. Tested against a **loopback** fake provider
  (`fixtures/fake_provider.py`) — no paid/network calls during verification.
- **Cost accounting is honest**: cost is `cost_per_call × n_calls`. When a
  provider's price is unknown the operator supplies `cost_per_call` (default 0)
  or disables the budget with `--cost-limit 0`; the harness never invents a
  price. NaN/infinite/negative budgets and malformed replay/config data are
  rejected.

### Normalized result contract

`--emit-envelope` prints `mini-swe-agent-bounded-result-v1` on stdout —
deterministic (no timestamps, paths, or host identity):

```json
{"schema":"mini-swe-agent-bounded-result-v1","scenario":"…","upstream_version":"2.4.6",
 "bounded_version":"0.1.0","trajectory_format":"mini-swe-agent-1.1","mode":"yolo",
 "exit_status":"Submitted","submission":"…","model_name":"…","api_calls":N,"cost":N,
 "actions":[{"command":"…","returncode":0,"submitted":true}]}
```

## Requirements

- **python3** (standard library only — no third-party packages).
- A **Bashy** executable (`BASHY_BIN` or `bashy` on `PATH`) as the action executor.
- The frozen **ycode** binary (`YCODE_BIN`) for the declarative surface and the
  YAML entrypoint. The shell entrypoint's run path does not require ycode.

## Fixtures

`fixtures/run.sh` is the shared, deterministic, offline gate (26 cases):

```sh
YCODE_BIN=/path/to/ycode BASHY_BIN="$(command -v bashy)" /bin/sh fixtures/run.sh
```

It covers: the ycode CLI contract; **shell/YAML/golden parity** across five
lifecycle scenarios; **real-PTY** confirm and human interaction and a **real
SIGINT** interruption (`fixtures/interactive_check.py`); the **live loopback**
transport (`fixtures/live_check.py`); fail-closed and validation cases; and the
python unit suite (`harness/tests/`). Scenarios live in `fixtures/scenarios/`,
result goldens in `fixtures/goldens/`, help golden in `fixtures/cli/`.

## Source / adaptation manifest

| This file | Upstream origin | Relationship |
|---|---|---|
| `harness/minisweagent_bounded/exceptions.py` | `src/minisweagent/exceptions.py` | **Materially derived.** Full agent-flow hierarchy incl. `UserInterruption`; `NonInteractiveApproval` is original. |
| `harness/minisweagent_bounded/environment.py` | `src/minisweagent/environments/local.py` | **Materially derived.** Stdlib-only; runs every action as `bashy -c` (explicit argv, no `/bin/sh`); process-group reap on timeout/interrupt; submit protocol preserved. |
| `harness/minisweagent_bounded/agent.py` | `src/minisweagent/agents/default.py` + `agents/interactive.py` | **Materially derived.** Default step loop + the interactive subset (modes, `/c /y /u /h /m`, Ctrl-C, confirm-exit). jinja2→`{{var}}`; pydantic→dataclasses. |
| `harness/minisweagent_bounded/model.py` | *(original; cf. litellm model)* | **Original.** `ReplayModel` + stdlib OpenAI-compatible `LiveModel`; last-fenced-block action extraction; output truncation. |
| `harness/minisweagent_bounded/prompter.py` | *(original; replaces rich/prompt_toolkit)* | **Original.** Stdlib TTY-aware prompter + scripted test prompter. |
| `harness/minisweagent_bounded/config.py` | *(original; replaces jinja/pydantic config merge)* | **Original.** JSON + `key=value` config merge; strict budget/replay validation. |
| `harness/minisweagent_bounded/runner.py` | *(original)* | **Original.** Build/run/normalize core shared by both entrypoints and tests. |
| `harness/minisweagent_bounded/cli.py` | *(original; cf. `run/mini.py`)* | **Original.** Flags-first entrypoint both paths exec. |
| `harness/minisweagent_bounded/__init__.py` | cf. `src/minisweagent/__init__.py` | Package doc + version/trajectory-format constants. |
| `harness/tests/test_harness.py` | *(original)* | **Original.** Deterministic offline unit tests. |
| `harness/LICENSE` | `LICENSE` (upstream MIT) | Verbatim MIT + copyright, scoped to the derived portions. |

## License handling

Upstream is **MIT**, © 2025 Kilian A. Lieret and Carlos E. Jimenez. The full MIT
text and copyright are preserved verbatim in `harness/LICENSE`; every materially
derived file carries a header pointing back to its origin; original files say so
explicitly. Permissive-only; nothing is compiled into the bashy binaries.

## Excluded upstream subsystems & compatibility limits

Deliberately **not** ported (documented as unsupported):

- **litellm / provider SDKs** — replaced by the stdlib `LiveModel` + `ReplayModel`.
- **docker / singularity / swerex environments** — only the local Bashy
  environment is ported; other `--environment-class` values fail explicitly.
- **jinja2 templating** — replaced by minimal `{{ var }}` substitution;
  conditionals/loops (e.g. upstream `mini.yaml`'s OS-conditional blocks) are
  unsupported.
- **rich / prompt_toolkit** console decoration, **pydantic** config models, YAML
  config-document discovery/merge, and trajectory storage integrations.
- **Advanced Python classes / plugins / vendor behavior** — any unsupported
  `--model-class` / `--agent-class` / `--environment-class` value fails
  explicitly rather than silently degrading.

Compatibility limits: ycode's frozen dispatch cannot launch a mini loop, so the
YAML entrypoint runs the loop through the documented example-local bridge above.
The comparison is of **normalized events and exit outcomes**, not byte-identical
UI decoration, and **no comparative benchmark is claimed** in this story.
