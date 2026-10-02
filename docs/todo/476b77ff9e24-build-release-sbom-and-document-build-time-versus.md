---
id: 476b77ff9e24
kind: task
title: Build release SBOM and document build-time versus runtime license boundary
seq: 385
status: todo
priority: p1
labels:
    - licensing
    - release
created: 2026-10-01T17:23:26.533927Z
sprint: 350
sprint_id: 406ae7fe-78aa-5ddc-9508-6e4dbf105b2a
sprint_title: Bashy release SBOM and license boundary
---

Goal
Produce an auditable SBOM for the exact Bashy release binaries and a separate inventory of programs fetched or built only after installation. Establish the claim "the shipped Bashy binary contains only permissively licensed open-source projects" from evidence, and block that claim if the evidence disagrees.

Deliverables
1. Generate a machine-readable SPDX 2.3 (or equivalent standard) SBOM for each supported release OS/architecture and build-tag variant, tied to the binary digest, Bashy commit, Go version, build flags, and release version. Include the full direct/transitive Go module graph, vendored code, embedded assets, native/C objects or launcher, generated code, and first-party submodules actually present in the artifact. Distinguish shipped components from build tools, test-only inputs, and source fetched during a build.
2. Publish a human-readable license inventory and THIRD_PARTY_LICENSES/attribution for shipped components: component, exact version/commit, source URL, SPDX expression or custom license, evidence link, inclusion mechanism, and license obligations. Identify unknown/mixed licenses explicitly. Record the policy's precise definition of "permissive"; do not infer a whole archive's license from its top-level repository license.
3. Publish a separate runtime-acquisition inventory for every shipped binmgr path, provider source build, engine/helper download, and indirect toolchain acquisition. For each: Bashy trigger/command, fetched program(s), platform, pinned or dynamic version, source, digest/check mechanism, cache destination, license of the actual asset and bundled helpers, and whether Bashy itself redistributes any bytes. Include Node/npm, Bun/WebKit, Linux podman-static, POSIX providers, and the otel/Victoria stack; reconcile stale atlas/docs against live callsites. Operator-registered commands and optional cloud overlays are explicitly dynamic, not silently counted as fixed Bashy dependencies.
4. Add a reproducible generation/check command and release gate. The gate fails on missing license evidence, non-permissive or proprietary code in the shipped binary, or SBOM/binary digest drift; runtime-only programs must remain separately disclosed and must not be presented as linked dependencies. Resolve or record any blocker before asserting the permissive-only claim in release documentation.

Acceptance
- From a clean checkout, the documented command regenerates the release SBOMs and inventories for the release build matrix; a reviewer can map every listed artifact to the exact binary or runtime trigger.
- An independent comparison of Go build info, embed/native inputs, binmgr callsites, POSIX manifest, and generated SBOM finds no unclassified shipped component or runtime download.
- The release report plainly separates build-time inputs, bytes inside the distributed Bashy artifact, and post-install downloads; each category has a license summary grouped as permissive, non-permissive, or proprietary.
- The permissive-only statement is made only for the verified shipped Bashy artifact. Any non-permissive, proprietary, unknown, or mixed component inside it remains a visible failing finding until resolved; platform-specific runtime bundles retain their own license notices and conditions.
