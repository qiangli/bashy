# Plan: shared fork-build-sign pipeline (Sprint 434, story 61071172f8ee)

Goal: the verified builds of third-party tools (`builds/openclaw`, `builds/tuwunel`,
then the CLI-tool migration of story 5aa131c7) share one pipeline. Adding a tool
means writing one declaration plus its genuinely tool-specific scripts.

Mechanism (KISS: no new service, no Go): `builds/render.sh` renders each
`builds/<tool>/fork/` from `tool.env`, `files/`, optional step fragments, and
`builds/_shared/` (workflow skeleton, `verify-pin.sh`, `kinds/cargo`).
`builds/check.sh` re-renders every tool and diffs it against the committed fork,
so the committed fork trees always reproduce from their declarations. The usage
guide is `builds/README.md`.

A GitHub reusable workflow was rejected for two reasons. It would add a
cross-repo `uses:` whose identity differs from the per-fork
`bashy-build.yml@refs/tags/T` that binmgr pins. It would also need its own
pinning and release story.

Behaviour (pins, gates, identity, OTel endpoint) is preserved:

- tuwunel: same jobs and steps. The only changes are parameterization through
  `tool.env`.
- openclaw: moved from one job onto the shared pin → build → release shape. It
  keeps the same upstream pin, override/lockfile/audit/SBOM/no-egress gates,
  assets and identity. It gains the shared gates: repository guard,
  `bashy-<tag>-N` tag shape, upstream release not draft/pre-release, tag→commit
  and pristine-tree checks, and per-bundle self-verify. It also moves to the
  newer pinned `cosign-installer` (v4.1.2), `actions/attest` (v4.2.2, over
  `SHA256SUMS`) and `upload-artifact` (v7.0.2). Its PIN keys are renamed to the
  shared `UPSTREAM_*`. The leftover 30-day "keep assets" artifact is dropped,
  because the release carries the assets.
