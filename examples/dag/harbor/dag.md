---
name: harbor
description: bashy dag front door for Harbor — sync the CLI from a checkout and run the built-in hello-world task to a real PASS verdict under podman, as one dependency graph
type: dag
default: smoke
---

# Harbor — task graph

The repo's own commands (`uv`, `harbor run`), wired as a dependency graph so
one front door replaces the ad-hoc `uv tool install harbor && harbor run …`
incantations: `bashy dag --list` shows the targets, `bashy dag smoke` syncs
the venv then runs the built-in hello-world task with the oracle agent. Lives
at the repo root of [harbor-framework/harbor](https://github.com/harbor-framework/harbor)
— formerly `laude-institute/harbor`; that URL redirects — (Apache-2.0); the
pinned example is measured at release tag `v0.24.0`
(`b53b8134e1241686dca7759af188f987ecc48e8b`, 2026-10-05). Needs `bashy`
(github.com/qiangli/bashy), `uv`, and `podman` with a compose frontend
(`podman compose`, else `podman-compose`) on PATH; harbor requires
Python >= 3.12 and commits `uv.lock` + `.python-version`.

## Tasks

### sync
Create or refresh `.venv` from the committed `uv.lock` — `--frozen` so a
newer `uv` never rewrites the lock, `--no-dev` because the `dev` group pulls
the full cloud-extra matrix (twenty-plus sandbox SDKs, tinker, dspy) the
hello-world smoke does not need; contributors drop `--no-dev` to get pytest,
ruff and the extras. The installed-product alternative upstream's README
fronts is `uv tool install harbor`; this graph runs the checkout's own CLI
(`uv run --no-sync harbor …`).
Sources: pyproject.toml uv.lock .python-version
Generates: .venv/pyvenv.cfg
Effects: read, write, net
Timeout: 30m

```bash
uv sync --frozen --no-dev
```

### smoke
Run the built-in hello-world task (`examples/tasks/hello-world`, family
`harbor/hello-world`: the agent must write `hello.txt`, the verifier greps
it) with the `oracle` agent — the keyless agent that executes the task's own
`solution/solve.sh` — so the verdict is a real PASS (reward 1.0) with no
model credentials; upstream's own e2e pins exactly this (oracle → 1.0, nop →
0.0), which is why `nop`, the other keyless agent, is not used here: it never
acts, so its verdict is a FAIL. Sandbox venue: **podman on Linux** —
`-e podman` swaps harbor's orchestrator to the `podman` CLI and shells out to
`podman compose ls`, which podman-compose does NOT support — the venue needs
docker-compose v2 as the podman compose provider plus a podman API socket in
`DOCKER_HOST`; never Docker on a dev box:

    export PODMAN_COMPOSE_PROVIDER=docker-compose
    podman system service -t 0 unix:///tmp/podman.sock &
    export DOCKER_HOST=unix:///tmp/podman.sock

The body refuses to run off Linux (the sandbox venue), parks job output in a
cache dir outside the checkout, then reads the trial-level `result.json`
(`<jobs>/<job>/<task>__<id>/result.json`, whose
`verifier_result.rewards.reward` is 1.0 on PASS — the job-level
`<jobs>/<job>/result.json` carries `stats` only) and asserts the verifier's
reward is 1. The task's verifier installs uv inside the sandbox, so the
venue needs network.
Requires: sync
Effects: read, write, net, exec
Timeout: 30m

```bash
[ "$(uname -s)" = Linux ] || { echo "smoke: harbor's sandbox venue is podman on Linux (never Docker on a dev box)" >&2; exit 1; }
jobs="${DAG_CACHE_DIR:-${TMPDIR:-/tmp}}/harbor-jobs"
mkdir -p "$jobs"
uv run --no-sync harbor run -p examples/tasks/hello-world -a oracle -e podman -o "$jobs"
job=$(ls -1t "$jobs" | head -1)
results=$(find "$jobs/$job" -mindepth 2 -name result.json | head -1)
[ -n "$results" ] || { echo "smoke: no trial result.json under $jobs/$job" >&2; exit 1; }
ok=$(uv run --no-sync python -c "import json,sys; r=json.load(open(sys.argv[1])); vr=r.get('verifier_result') or {}; rw=vr.get('rewards') or {}; print(1 if float(rw.get('reward', 0))==1.0 else 0)" "$results")
[ "$ok" = 1 ] || { echo "smoke: hello-world reward not 1.0 ($results)" >&2; exit 1; }
echo "smoke: harbor/hello-world oracle PASS ($results)"
```
