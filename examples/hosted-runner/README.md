# Hosted Runner Pilot (`bashy/examples/hosted-runner/`)

Containerized `bashy mcp serve --transport http --hosted` pilot runner and fake
master signer helper for Sprint 691 H4.

## Files

- `hosted-runner.bsh` — Bash# skill-script (`build|start|stop|status`, `--json`, `--dry-run`)
- `Containerfile` — derived from `tools/bashy-build/Containerfile.base` with `bashy` built from `main`, `google` + `trove` registered, and pre-warmed `BASHY_ADAPTER_CACHE` (`secret`, `jq`, `lens`, `sha256sum`)
- `register.sh` — registers `hosted-runner` in `bashy commands` and `bashy skill`
- `cmd/signer/main.go` — fake master signer test helper (test keys only) implementing H1 `X-Bashy-Hosted-Identity` (`v1.<b64url>.<hmac>`)

## Usage (on a Linux build host, never the dev box)

```sh
bashy --bashsharp examples/hosted-runner/hosted-runner.bsh build --json
bashy --bashsharp examples/hosted-runner/hosted-runner.bsh start --port 22751 --json
GOWORK=off go -C examples/hosted-runner run ./cmd/signer \
  --url http://127.0.0.1:22751/mcp \
  --key-file "${XDG_STATE_HOME:-$HOME/.local/state}/bashy/hosted-runner.key" \
  --verify --json
bashy --bashsharp examples/hosted-runner/hosted-runner.bsh status --port 22751 --json
bashy --bashsharp examples/hosted-runner/hosted-runner.bsh stop --json
```

## Hermetic tests

```sh
GOWORK=off go -C examples/hosted-runner test -v ./...
```

## H5 load mode and pre-warmed fakes

- `hosted-runner.bsh build --with-fakes --json` also pre-warms `custom/gh`, `UV_PYTHON_INSTALL_DIR` (`/var/cache/bashy/python`), and static fake `gws` / `gh` binaries (`cmd/fakecli`).
- `hosted-runner.bsh start --cloudbox-url URL --pids-limit 8192 --json` configures the fake vault endpoint and container PIDs ceiling for concurrent load.
- `cmd/signer --load [--rate N --duration 60s [--open-loop --queue-cap M] | --burst 1000] --concurrency C --mix echo,gmail,trove,gh --fake-vault-listen 127.0.0.1:22752 --json` drives concurrent fresh-identity MCP calls (including H1b digest-bound `cred` approval for `gh` and H2 vault projection for `google`/`trove`).
