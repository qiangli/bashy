# Pinned Yoke runner release integration

Sprint 436 Story #2028 (`732cbb22d353`) restores the embedded llama.cpp
runner without accepting a pre-existing gzip or mutating a shared module
cache.

## Release shape

- `go.mod` pins Yoke `98dd834f10a8`, the first main revision carrying the
  source-pinned runner recipe.
- Linux amd64/arm64 run on their matching hosted architectures. Each lane
  clones the exact Yoke pin into runner-temporary storage, hydrates only its
  pinned Ollama gitlink, builds with cgo, and uploads a deterministic gzip plus
  a source/digest manifest.
- The Linux gzip inputs receive GitHub build-provenance attestations. The
  GoReleaser job verifies those attestations before using one isolated Yoke
  checkout, module cache, build cache, and Go workspace per target.
- Darwin amd64/arm64 perform the same native source build inside the existing
  native archive jobs. The final archives retain the existing provenance
  attestations and publication checks.
- Release artifact verification reads Go build information and requires the
  exact Yoke pseudo-version, target OS/architecture, and `embed_runner` tag.
  The signed checksum manifest continues to bind the complete release set.
- The GoReleaser archive and nFPM definitions remain in place. Existing meet
  SPA, signal, SBOM, package-content, candidate, promotion, and cleanup gates
  remain enforcing.

## Fail-closed gap

Yoke `98dd834f10a8` has no native Windows runner build. The Windows Bashy build
definitions remain present, but their post-build runner check reports
`INCOMPLETE` and exits non-zero. Therefore no release candidate can be
packaged or published until Windows gains a genuinely native pinned runner.
There is no override.

No release is published as part of this story. A manager must review the
Yoke/Coreutils/GFY pin bundle and the hosted-CI evidence before any tag is
created.
