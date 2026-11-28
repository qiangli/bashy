# Contained genie end-to-end gate

The gate exercises `bashy contain --net deny --dir DIR --socket SOCK` driving
real `bashy genie` through the in-sandbox launcher (`genie --session-launcher`, which forwards in process) against a fake cligw
unix-socket backend (`fake_door.py`) in the official `bashy-base` image locked
by `tools/bashy-build/image.lock`.

## How it works

1. `run.py` stages a synthetic host `HOME` with credential-shaped files and
   environment variables, plus an outside canary file (`CANARY_OUTSIDE_DIR_SECRET_738`)
   that must remain unreadable inside the sandbox.
2. `fake_door.py` listens on an owner-only (`0600` in a `0700` directory) unix
   domain socket, validates bearer tokens (valid, wrong, expired, and
   model-scoped), and answers `/v1/chat/completions` with a deterministic
   sequence of `bashy` tool calls:
   - write `expected.txt` inside `DIR`
   - attempt to `cat` the outside canary and synthetic credential files
   - print the environment and `$HOME` listing
   - return a final answer ending with `DONE`
3. `in_sandbox.sh` runs inside `bashy contain --net deny --dir DIR --socket SOCK`,
   captures the container-level environment, then hands over to the product launcher (`genie --session-launcher`) on the
   one permitted loopback port.
4. Four legs run and are asserted against the fake door's request log and
   on-disk artifacts:
   - **Valid token:** `expected.txt` written in `DIR`; outside canary and host
     credential files denied; no credential-shaped variable or host secret in
     either the `contain` environment or genie's tool environment; `DONE` emitted.
   - **Wrong token:** refused with HTTP `401` (`invalid_api_key`), non-zero exit.
   - **Expired token:** refused with HTTP `401` (`token_expired`), non-zero exit.
   - **Out-of-scope model:** refused with HTTP `403` (`model_out_of_scope`),
     non-zero exit.

## Run on a claimed gate lane

Claim the gate lane with `bashy claim` before running. Build a Linux candidate
binary of `bashy`, ensure the locked `base_image` in `tools/bashy-build/image.lock`
is present in `bashy oci`, and run twice keeping fresh raw logs:

```sh
cd bashy
GOWORK=off CGO_ENABLED=0 go build -o bin/bashy ./cmd/bashy
export CONTAINED_GENIE_GATE=1
export CONTAINED_GENIE_EVIDENCE=/path/to/private-evidence/contained-genie-run1.log
bashy dag test-contained-genie-e2e
export CONTAINED_GENIE_EVIDENCE=/path/to/private-evidence/contained-genie-run2.log
bashy dag test-contained-genie-e2e
```

## Configuration

| Variable | Meaning | Default |
|---|---|---|
| `CONTAINED_GENIE_GATE` | must be `1` (fail-closed otherwise) | — |
| `CONTAINED_GENIE_BASHY_BIN` | candidate `bashy` binary under test | `bin/bashy` |
| `CONTAINED_GENIE_SANDBOX_BASHY` | static linux `bashy` mounted inside the image (`bashy dag build-bashy-scratch` output) | the candidate binary |
| `CONTAINED_GENIE_IMAGE` | container image override (skips `image.lock` digest check) | `base_image` in `image.lock` |
| `CONTAINED_GENIE_GENIE_BAR` | prebuilt `genie.bar` override | built via `bashy genie build` |
| `CONTAINED_GENIE_EVIDENCE` | fresh raw log path (must not exist) | — |
| `CONTAINED_GENIE_WAIT_SECONDS` | per-leg timeout in seconds | `120` |
| `CONTAINED_GENIE_FORWARD_READY_SECONDS` | unused: the launcher forwards in process | `20` |
| `CONTAINED_GENIE_SOCKET_READY_SECONDS` | timeout waiting for fake door socket | `15` |
| `CONTAINED_GENIE_REQUEST_TIMEOUT_MS` | per-request genie timeout in milliseconds | `30000` |

## Static and unit checks (dev host)

```sh
bash -n tests/contained-genie-e2e/in_sandbox.sh
python3 -m py_compile tests/contained-genie-e2e/fake_door.py tests/contained-genie-e2e/run.py tests/contained-genie-e2e/test_fake_door.py
python3 -m unittest discover -s tests/contained-genie-e2e -v
```
