# pause — bashy verified build

The outpost runtime image gives k3s its pod sandbox ("pause") process only
from a **bashy-CI-built, keylessly Sigstore-signed build** checked against the
official Kubernetes release tag. This directory is the declaration for that
build. `fork/` is installed as the `bashy-build` branch of
`github.com/qiangli/pause`; it carries no upstream source — the workflow
checks out the upstream tag fresh on every run.

| | |
|---|---|
| Upstream | `https://github.com/kubernetes/kubernetes` (`build/pause/linux/pause.c`) |
| License at the tag | **Apache-2.0** (`LICENSE`, verified at `v1.37.1`); the port is Apache-2.0 too |
| Pinned release | `v1.37.1` (annotated tag `d2b770f4…`) → commit `f78e722310e50bcaca9276be22276d9e91d91308` |
| Pause version | `3.10.2` (`build/pause/Makefile` `TAG` at the tag) |
| Toolchain | Go `1.26.0` (the tag's `go.mod`), built with the in-minor security override `1.26.9` |
| Trigger | push of tag `bashy-v1.37.1-N` on `bashy-build` (first: `bashy-v1.37.1-1`) |
| Signing identity | `https://github.com/qiangli/pause/.github/workflows/bashy-build.yml@refs/tags/bashy-v1.37.1-1` |
| Issuer | `https://token.actions.githubusercontent.com` |

## Why a port

Upstream's pause is 60 lines of C, built `-static` against glibc inside a
container image. A static glibc link is not a permissive artifact, and bashy
ships no C it does not have to. `files/pause/` is a pure-Go port, standard
library only, built `CGO_ENABLED=0`:

| pause.c | port (`files/pause/main.go`) |
|---|---|
| `-v` (case-insensitive) prints `pause.c <VERSION>`, exit 0 | prints `pause.go v3.10.2-v1.37.1-bashy`, exit 0 |
| warning on stderr when not PID 1 | same text |
| SIGINT/SIGTERM: `psignal` "Shutting down, got signal", exit 0 | same text and status |
| SIGCHLD: `waitpid(-1, NULL, WNOHANG)` until none left | `syscall.Wait4(-1, …, WNOHANG)` until none left |
| `for (;;) pause();` | blocks on the signal channel |

The kubernetes tag is the **specification**, not the compiled source:
`build.yml` refuses to build unless the tag's `pause.c` hashes to
`PAUSE_C_SHA256` (the file the port was reviewed against) and its `Makefile`
`TAG` is `PAUSE_VERSION`. A pin bump that changes `pause.c` therefore fails
until the port is re-reviewed. The build also fails if the binary has an ELF
interpreter, is not `CGO_ENABLED=0`, links any non-stdlib module, fails the
port's unit test (child reaping), or does not report the pinned version.

## Layout

```
tool.env                  the declaration (shipped as PIN)
build.yml                 spec gate (pause.c hash, pause version) + build of the port
audit.yml                 govulncheck + SBOM of the port (the shipped module)
files/pause/              the pure-Go port (go.mod, main.go, reap_test.go)
files/no-egress-test.sh   runtime gate: no socket at all, plus pause.c's SIGTERM contract
files/otel.env            OpenTelemetry environment (disabled; local collector only)
files/no-tracking.env     proxy clearing
files/vuln-ignore         govulncheck waivers (empty)
```

## Audit

govulncheck v1.9.0 of the port under go1.26.9: standard library only, **0
reachable, nothing waived**.

## How the runtime image serves it to k3s

This pipeline produces static binaries only; the image is built elsewhere.

1. Wrap `pause-linux-<arch>` in a `FROM scratch` OCI image (the binary at
   `/pause`, `ENTRYPOINT ["/pause"]`), tagged with a bashy-owned name, e.g.
   `bashy.local/pause:3.10.2-bashy1`.
2. Deliver it without a registry pull: write it as an OCI/docker archive into
   k3s's airgap directory (`/var/lib/rancher/k3s/agent/images/*.tar`), which
   k3s imports into containerd at agent start. A local registry declared in
   `/etc/rancher/k3s/registries.yaml` is the alternative.
3. Start the agent with `--pause-image bashy.local/pause:3.10.2-bashy1`
   (k3s's default is `rancher/mirrored-pause:3.6`), which becomes containerd's
   sandbox image and the image k3s pins against garbage collection.

## Release assets

`pause-linux-{amd64,arm64}`, `pause-<tag>.cdx.json`,
`pause-<tag>.govulncheck.json`, `pause-pin.txt`, `pause-otel.env`,
`pause-no-tracking.env`, `vuln-ignore`, each with a `.sigstore.json` bundle,
plus `SHA256SUMS`.

## Operator steps (not in bashy CI)

```sh
# in a new repository github.com/qiangli/pause
git switch --orphan bashy-build
cp -R <bashy>/builds/pause/fork/. .
git add .github/workflows/bashy-build.yml bashy-build
git commit -m "bashy-build: pause 3.10.2 (kubernetes v1.37.1) verified build"
git push -u origin bashy-build
git tag bashy-v1.37.1-1 && git push origin bashy-v1.37.1-1
```

Then pin the release in binmgr against the signing identity and issuer above.
