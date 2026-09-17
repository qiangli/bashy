# mini-swe-agent — bounded CLI profile + offline harness

A bounded, self-contained example of the [mini-swe-agent](https://github.com/SWE-agent/mini-swe-agent)
`mini` CLI expressed as a ycode YAML CLI profile, a thin Bash++ adapter, and a
stdlib-only offline rewrite of the agent lifecycle. It is independently runnable
from this directory. **No benchmark results are claimed.**

## Upstream source pin

- Repository: `https://github.com/SWE-agent/mini-swe-agent`
- Source commit pin: **`04d809ceab9df28f9adaed044884180159172930`**
- Upstream package version at that pin: **`2.4.6`**
  (`harness/minisweagent_bounded/__init__.py:UPSTREAM_VERSION`)

The read-only reference bundle used to author this port is **not shipped**; only
the files below are part of the example.

## Three surfaces

| Surface | File(s) | What it is | What it is NOT |
|---|---|---|---|
| CLI contract | `profile.yaml` | ycode strictly validates and renders the declared `mini` CLI (help/version/validate/completion + the bounded lifecycle verbs). | A running agent. ycode's frozen dispatch cannot launch a mini loop, so `start`/`run` exit unsupported (4) under ycode. |
| Adapter | `main.bpp` | A thin Bash++ adapter that maps the bounded verbs onto an installed upstream `mini` and forwards argv/streams/exit status verbatim. | A reimplementation of any mini behavior. It runs the real upstream (online) — nothing bounded happens in the adapter. |
| Offline harness | `harness/` | A stdlib-only, deterministic, **replay-only** rewrite of the agent step loop that runs end-to-end with real local shell actions and a scripted model, emitting a normalized result envelope. | Live inference. No network and no model provider is contacted. |

### Replay-only vs. live inference — stated plainly

The `harness/` lifecycle is **replay-only**: a scripted `ReplayModel` supplies a
pre-recorded sequence of assistant steps, so the loop is fully deterministic and
offline. Substantive **live** inference happens **only** in the installed
upstream `mini`, reached through `main.bpp`. The fixtures never call a paid
model.

## CLI surface

The pinned `mini` is a single [typer](https://typer.tiangolo.com/) command with
flags — not a subcommand tree. This profile projects the common session
lifecycle onto that flags-first surface:

- Root flags: `-t/--task`, `-m/--model`, `-c/--config`, `-o/--output`,
  `-l/--cost-limit`, `-y/--yolo`, `--exit-immediately`, `--model-class`,
  `--agent-class`, `--environment-class`.
- `start` (alias `run`): the bounded convenience verb for the default bare
  invocation; the adapter drops the token and forwards the remaining flags.
- `version`, `validate`, `schema`, `completion <shell>`, `help` — the shared
  common CLI surface, dispatched by ycode.
- `init`, `model`, `plan`, `resume`, `exit`: **declared unsupported (exit 4)**
  because there is no such verb in the pinned source (mini configures on first
  run; model selection is the `-m` flag; there is no plan/resume/exit verb).

## Run it

From this directory (`examples/cli/mini-swe-agent/`):

```sh
YCODE_BIN=/path/to/ycode

# 1. CLI contract (strict validation + rendering, no agent runs):
"$YCODE_BIN" --file profile.yaml validate
"$YCODE_BIN" --file profile.yaml --help
"$YCODE_BIN" --file profile.yaml completion bash

# 2. Adapter → installed upstream `mini` (online; needs a real mini + credentials):
MINI_BIN=/path/to/mini bashy --bashpp main.bpp start -t "fix the failing test"

# 3. Offline lifecycle harness (deterministic, no network):
cd harness
python3 -m minisweagent_bounded.runner ../fixtures/scenarios/submit-success.json
python3 -m unittest discover -s tests
```

Set `MINI_BIN` to an absolute path to the installed `mini`; with no override the
adapter resolves `mini` from `PATH`.

## Offline fixtures

`fixtures/run.sh` is the shared, deterministic, offline gate over all three
surfaces (requires an absolute `YCODE_BIN`, a `BASHY_BIN` or `bashy` on `PATH`,
and `python3`):

```sh
YCODE_BIN=/path/to/ycode BASHY_BIN="$(command -v bashy)" /bin/sh fixtures/run.sh
```

- `fixtures/cli/` — golden stdout for the ycode-rendered help surface.
- `fixtures/scenarios/` — 5 self-contained lifecycle scenarios (JSON).
- `fixtures/goldens/` — the normalized result envelope for each scenario.

The five scenarios exercise the full exit taxonomy with real actions and a real
submission (the "parity" the harness demonstrates — not merely matching
unsupported ops):

| Scenario | Exit status | Demonstrates |
|---|---|---|
| `submit-success` | `Submitted` | stateless shell actions + explicit submit marker + submission payload |
| `cost-limit` | `LimitsExceeded` | cost budget stops a non-submitting loop |
| `step-limit` | `LimitsExceeded` | step budget stops a non-submitting loop |
| `repeated-format-error` | `RepeatedFormatError` | N consecutive malformed responses terminate |
| `format-error-recovers` | `Submitted` | a clean step resets the consecutive-error counter |

The normalized envelope (`mini-swe-agent-bounded-result-v1`) is intentionally
free of timestamps, absolute paths, and host identity, so a future benchmark
harness can compare runs byte-for-byte.

## Source / adaptation manifest

Every file under `harness/`, with its upstream origin at the source pin:

| This file | Upstream origin | Relationship |
|---|---|---|
| `harness/minisweagent_bounded/exceptions.py` | `src/minisweagent/exceptions.py` | **Materially derived.** Trimmed to the subset the bounded loop uses (dropped `UserInterruption`). |
| `harness/minisweagent_bounded/environment.py` | `src/minisweagent/environments/local.py` | **Materially derived.** Rewritten stdlib-only: pydantic `LocalEnvironmentConfig` replaced by plain `__init__` args; the `_run` process-group semantics and the `COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT` submit protocol are preserved. `get_template_vars` (platform/env injection) is omitted as out of scope. |
| `harness/minisweagent_bounded/agent.py` | `src/minisweagent/agents/default.py` | **Materially derived.** The `DefaultAgent` step loop, the exit-status taxonomy, and the trajectory shape are preserved. jinja2 → a minimal `{{ var }}` substitution; pydantic `AgentConfig` → a `dataclass`; `recursive_merge` reimplemented. |
| `harness/minisweagent_bounded/model.py` | *(none — original)* | **Original.** The offline `ReplayModel` stands in for the upstream network model providers (litellm), implementing the small model interface the agent depends on. |
| `harness/minisweagent_bounded/runner.py` | *(none — original; cf. `src/minisweagent/run/mini.py`)* | **Original.** Offline scenario driver + normalized envelope. Replaces the typer/interactive/live-provider `mini` entrypoint. |
| `harness/minisweagent_bounded/__init__.py` | cf. `src/minisweagent/__init__.py` | Mostly original package doc; carries the upstream version and trajectory-format constants. |
| `harness/tests/test_harness.py` | *(none — original)* | **Original.** Deterministic offline regression tests. |
| `harness/LICENSE` | `LICENSE` (upstream, MIT) | Verbatim upstream MIT license + copyright, with a note scoping the derived portions. |

## License handling

Upstream mini-swe-agent is **MIT**, © 2025 Kilian A. Lieret and Carlos E.
Jimenez. The full MIT text and copyright are preserved verbatim in
`harness/LICENSE`, and every materially derived file carries a header pointing
back to its upstream origin and this manifest. Original files (`model.py`,
`runner.py`, `test_harness.py`) say so explicitly. This is a permissive-only
dependency; nothing is compiled into the bashy binaries.

## Excluded upstream subsystems & compatibility limits

Deliberately **not** ported (out of scope for a bounded, offline, dependency-free
example):

- **Model providers** — litellm and all hosted/local backends. Replaced by the
  replay model; the harness performs no inference.
- **Non-local environments** — docker / singularity / swerex. Only the local
  shell environment is ported.
- **The interactive UX** — the `InteractiveAgent`, confirm/human modes, the
  `/c /y /u /h /m` controls, and Ctrl-C interruption live only in the upstream
  `mini` reached through the adapter; they are not reimplemented offline.
- **jinja2 templating** — replaced by minimal `{{ var }}` substitution;
  conditionals/loops (e.g. the OS-conditional blocks in upstream `mini.yaml`)
  are unsupported.
- **pydantic config models, config discovery/merge, and trajectory storage
  integrations.**

Compatibility limits:

- ycode strictly validates and renders the CLI contract, but its frozen dispatch
  **cannot launch a mini loop**; `start`/`run` therefore exit unsupported (4)
  under ycode. Running an actual session requires the adapter + an installed
  upstream `mini`. This is a documented example-local bridge, not CLI parity.
- The offline harness demonstrates the *lifecycle and exit outcomes* with real
  actions and a real submission, under a **replay** model — it is not a
  reproduction of upstream's live-inference behavior, and **no comparative
  benchmark is claimed.**
