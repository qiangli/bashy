# Group shaping end-to-end gate

Phase 1 exercises the owner-only site MCP in one disposable container. The
runner checks the official Ubuntu 24.04 base image against the repository's
`base_digest` and runs its image ID through `bashy oci` with networking disabled.
The fake satellite serves the host grant copy and a static page on loopback.
The outpost daemon fetches the grants and signs each request to the mounted
website and MCP. The test client supplies the synthetic front's user and mount
headers. Outpost generates its own app signing key.

## Agent binding

The site MCP passes the wish's `agent` field to `shape.BashyDelegate`.
It invokes `bashy delegate --agent codex:gpt6-sol` in the candidate checkout.
The built-in Codex recipe normally selects a managed binary. In the container's
empty home, the harness sets just `cli.binary` to the `codex` fixture found
on `PATH`. A delegate dry run checks the resolved executable. The stub answers
the recipe's login-status probe and edits the website title on the one expected
task. The real delegate, verifier and approval path run with synthetic accounts.

## Run on a claimed gate lane

Claim the gate lane with `bashy claim` before running. Use a Linux candidate
build of Bashy, outpost and Bashy's delegate adapter. From matching Bashy and
outpost sibling checkouts on the lane, for example:

```sh
export GROUP_SHAPING_BIN="$(mktemp -d)"
GOWORK=off CGO_ENABLED=0 go -C bashy build -o "$GROUP_SHAPING_BIN/bashy" ./cmd/bashy
GOWORK=off CGO_ENABLED=0 go -C outpost build -o "$GROUP_SHAPING_BIN/outpost" ./cmd/outpost
GOWORK=off CGO_ENABLED=0 go -C bashy/yoke build -o "$GROUP_SHAPING_BIN/delegate" ./delegate
```

Set `GROUP_SHAPING_PYTHON` to a relocatable Linux Python installation with
`bin/python3` and its standard library. The gate host needs Python 3.11+, the
matching Go toolchain on PATH, Git compatible with the Ubuntu base, and
`bashy oci` with the locked base image cached. These tools and the website
template are mounted read-only. The image's unprivileged user owns the
disposable home. Each attempt creates a new container.

```sh
cd bashy
export GROUP_SHAPING_GATE=1
export GROUP_SHAPING_EVIDENCE=/path/to/private-evidence/phase1-run1.log
bashy dag test-group-shaping-e2e
export GROUP_SHAPING_EVIDENCE=/path/to/private-evidence/phase1-run2.log
bashy dag test-group-shaping-e2e
```

Each log includes the image identity, executable hashes, service output, MCP
replies, the candidate diff, sealed verifier evidence and the active revision
check. The MCP preview assertion checks the candidate and URL returned by
`shape_preview` before approval. Member wishes, verification, preview and
approval must return the member-tier refusal. The out-of-group member,
unrelated account and anonymous caller must receive the mount's authentication
challenge. Keep the raw logs on the gate lane. The runner refuses to overwrite
a previous attempt.
