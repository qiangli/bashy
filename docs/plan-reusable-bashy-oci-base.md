# Plan: reusable Bashy OCI base

Status: implemented initial base; runtime-provider expansion remains a derived
image concern.

## Objective

Provide one public, reusable Linux OCI base that carries the real Bashy runtime
shape and can be extended by development, CI, or POSIX Profile D images. It must
not depend on or copy the proprietary certification harness.

## Product horizon

This base is the first layer of a longer-term alternative to the popular
Ubuntu and Alpine application bases: Bash 5.3 and POSIX shell compatibility,
the one canonical Bashy Go-coreutils inventory, O3 facilities (OpenTelemetry,
Ollama, and OCI), and automatically provisioned toolchains delivered through
verified, pinned `binmgr` caches. The aim is a powerful agent-operable base,
not merely a smaller distro image.

That is a roadmap, not a current replacement or parity claim. Today the image
is Ubuntu/glibc-based, the Bash-only and Go-utility certification profiles are
measured separately, O3 services remain optional/derived layers, and managed
providers must be explicitly pinned, prefetched, and evidenced. Promotion of
each layer requires its own measured profile and offline smoke gate.

## Decisions

1. Build from the flat `bashy`, `coreutils`, `sh`, and `readline` sibling
   checkouts. The helper stages only those four open-source trees so an umbrella
   build never sends unrelated or proprietary siblings to an OCI daemon.
2. Build through `make build-bashy`, never `go build`. Linux Bashy is a native
   pre-Go signal launcher plus an adjacent `bashy.real` Go payload; losing that
   pair loses inherited-signal semantics.
3. Use a minimal pinned Ubuntu 24.04 runtime, not `scratch`. The launcher and
   Bashy's locale bridge require glibc, and useful shell operation requires CA,
   timezone, locale, and terminal databases. Apt package resolution uses a
   dated Ubuntu snapshot. A future scratch-like build first needs a separately
   designed portable-locale mode.
4. Install the pair as `/bashy` and `/bashy.real`, with
   `/usr/local/bin/bashy -> /bashy`. Keep Ubuntu's `/bin/sh` unchanged for apt
   maintainer scripts and ordinary derived-image administration. Put
   `/opt/bashy/bin/sh -> /bashy` first on `PATH` as the explicit Bashy POSIX-mode
   compatibility name. A Profile D derivative may wire its own SUT path such as
   `/vsc/sut/sh` without mutating the generic base.
5. Default to an unprivileged account, a declared managed-binary cache, and
   `OTEL_TRACES_EXPORTER=none`. Downloads are not part of image startup or
   certification; provider caches must be populated and evidenced during a
   derived-image build.
6. Separate offline policy tests from the daemon smoke. Policy tests validate
   the recipe and staging contract without network. The smoke runs with no
   network, a read-only root, dropped capabilities, and small tmpfs mounts.

## Deliverables and gates

- `tools/bashy-oci/Containerfile`: multi-stage builder and Ubuntu/glibc runtime.
- `tools/bashy-oci/build-smoke.sh`: minimal-context build and isolated smoke.
- `tools/bashy-oci/test-policy.sh`: offline structural policy gate.
- `docs/bashy-oci-base.md`: operator-facing use and extension contract.
- Make targets: `build-bashy-oci`, `smoke-bashy-oci`, and
  `test-bashy-oci-policy`.

The initial gate proves the launcher pair, Bashy execution, the separate
POSIX-mode alias, system `/bin/sh` preservation, and disabled default tracing.
Provider-specific commands, services, locales, accounts, terminals, and
certification evidence are explicitly outside this generic layer.

## Remaining reproducibility gap

The Ubuntu runtime is digest-pinned and apt uses a dated snapshot. The official
Go builder image is version-pinned but not yet digest-pinned because the
multi-architecture manifest digest has not been recorded in this repository.
`GO_IMAGE` is an overridable build argument; release automation must supply and
record a reviewed digest-pinned builder until a canonical pin is committed.
