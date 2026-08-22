# Bashy OCI base

The reusable Linux base builds Bashy from the four public sibling checkouts and
runs it on a small Ubuntu/glibc runtime. It preserves the Unix launcher contract:
`/bashy` is the native signal launcher and `/bashy.real` is its adjacent Go
payload. The image starts as the unprivileged `bashy` user with `/bashy` as its
entry point.

The product goal is for this line to grow into a viable, powerful alternative
to familiar Ubuntu and Alpine application bases: Bash 5.3/POSIX behavior, the
canonical Bashy Go-coreutils inventory, O3 capabilities (OpenTelemetry, Ollama,
and OCI), and automatically provisioned toolchains from verified `binmgr`
caches. This initial image is the foundation, not a claim of current parity or
a drop-in replacement for those distributions. It remains glibc-based, and its
shell, Go-utility, O3, and external-provider layers retain separate measured
profiles and readiness gates.

```sh
make test-bashy-oci-policy                 # offline, no daemon or network
make build-bashy-oci                       # localhost/bashy-base:ubuntu24.04
make smoke-bashy-oci                       # isolated smoke of that image

# Override the engine, tag, or platform.
BASHY_OCI=docker \
BASHY_OCI_IMAGE=example/bashy-base:test \
BASHY_OCI_PLATFORM=linux/arm64 \
  tools/bashy-oci/build-smoke.sh all
```

The build helper creates a temporary context containing only `bashy`,
`coreutils`, `sh`, and `readline`. Those directories must be flat siblings, as
required by `go.mod`. Use the helper from an umbrella checkout: sending the
whole parent directory as a raw OCI context could disclose unrelated sibling
content to the daemon. A direct `podman build` is appropriate only when its
context has first been restricted to the same four public trees.

The Ubuntu runtime digest and apt snapshot are pinned. The versioned Go builder
currently has no committed multi-architecture digest; for a release build,
override it with a reviewed pin:

```sh
GO_IMAGE='docker.io/library/golang@sha256:<digest>' # pass as a build arg in release automation
```

`/bin/sh` remains Ubuntu's system shell. This is deliberate: package maintainer
scripts in derived images must keep their distro-tested interpreter. Bashy's
POSIX invocation is `/opt/bashy/bin/sh`, which is first on the image `PATH`, so
`sh` resolves to Bashy for ordinary commands while absolute `/bin/sh` remains
available. A certification derivative should point its declared SUT path at
`/bashy` rather than overwrite `/bin/sh` globally.

This image is a base, not a complete POSIX provider environment. Add pinned
external providers, account/session fixtures, locales, terminfo, printer/mail
services, capabilities, and evidence manifests in named derived images. Bake
managed binaries before an offline run; do not rely on first-use downloads.
The default `OTEL_TRACES_EXPORTER=none` also prevents an unconfigured first run
from creating trace spool state or printing an exporter banner.
