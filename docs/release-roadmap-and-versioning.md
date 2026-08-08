# Bashy release roadmap and versioning policy

Status: **plan of record, 2026-08-08**. This document defines release order,
public promises, upstream compatibility coordinates, and version-number rules.
Detailed gates remain in `bashy-v1.0.0-readiness.md` and the component plans.

## Principles

1. **One product version, several compatibility coordinates.** `vX.Y.Z` is the
   Bashy product/API version. GNU Bash, POSIX, Go, GNU Coreutils, and Tessaro
   versions are recorded independently in release metadata and the support
   matrix. One SemVer tuple cannot losslessly encode five independent streams.
2. **SemVer describes user impact.** Major means an incompatible Bashy public
   API/language/default-behavior change; minor means backward-compatible public
   functionality; patch means backward-compatible fixes. This follows
   [Semantic Versioning 2.0.0](https://semver.org/).
3. **Compatibility is a tested profile, not an implication.** Every release
   names exact upstream sources, patches, test profiles, host platforms,
   denominators, and known exceptions.
4. **Profiles accumulate when practical.** Adding a newer GNU/POSIX/Go profile
   is normally a minor release if the previous profile remains supported.
   Replacing a default or removing a supported profile may require a major.
5. **No aspirational claims.** “Compatible,” “conformant,” “certified,” signed,
   and packaged each have independent evidence gates.

## Roadmap

### Foundation milestones before v1.0

1. **GNU Bash 5.3 compatibility — achieved; continuously gated.**
   - Keep the serial GNU Bash 5.3 fixture gate and clean-room differentials.
   - Track the exact upstream tarball and applied Bash 5.3 patch level. GNU
     announced Bash 5.3 in July 2025; the release and patch series are separate
     provenance inputs.
2. **IEEE Std 1003.1 Shell and Utilities — shell milestone achieved; utilities are next.**
   - Correct standard family name: **IEEE Std 1003.1**, not 1002.1.
   - Current licensed campaign profile: VSC-PCTS2016/POSIX08, as already
     recorded by the harness and license documents.
   - Lane A: shell language/builtins. **Achieved 2026-08-08:** the complete
     493-TP `POSIX.shell` scenario placed all 493 TPs in the certification
     pass group with zero blockers, zero manual resolutions, and zero caps.
     The measured shell-isolation PATH used GNU Coreutils 9.11 and excluded
     Bashy's Go applets. Evidence tag: `vsc-pcts-posix-shell-2026-08-08`.
     The dedicated shell-closure sprint (#45) is ended; reopen this lane only
     for a demonstrated regression.
   - Lane B, the **next milestone**: complete all 116 Commands and Utilities
     sets (8,844 configured TPs). Bashy's Go utilities are the SUT; GNU
     Coreutils 9.11 is the apples-to-apples diagnostic control, not a claim
     that GNU behavior defines POSIX. Both arms start from the same immutable
     image and provisioning revision and differ only in their declared SUT.
   - Re-running the 493 shell TPs with the assembled Bashy utility providers is
     a regression/integration gate inside Lane B. It is not a separate roadmap
     milestone and does not precede starting utility-set work.
   - Lane B exit: all 116 sets and 8,844 TPs measured, no unexplained
     Bashy-only failures, `UNRESOLVED` results, or caps in the declared scope,
     repeatable matched-host evidence, and finalized provider limitations.
   - After Lane B: execute one uninterrupted 117-set/9,337-TP assembled formal
     profile, generate the suite's official report (`vrpt`), finalize the
     conformance statement, and prepare the human submission.
   - Formal Open Group submission/certification remains a separately named
     human/legal milestone; engineering completion must not be worded as a
     certification award.
3. **Bash++ language foundation.**
   - Stable target for v1.0: Go 1.26-shaped constructs supported by the declared
     Bash++ grammar/profile, including the race and lifecycle gate in
     `bash-plus-plus-design.md`.
   - Go 1.26 is an upstream toolchain/language coordinate, not a promise to
     accept arbitrary Go source. Every supported construct must be enumerated.
   - Python, TypeScript, and Rust embedding starts experimental, opt-in, and
     capability/effect-gated. It becomes stable only after runtime discovery,
     version negotiation, sandbox/effect policy, packaging, cancellation,
     serialization, and cross-platform gates exist. Recommendation: do not make
     all three stable embeddings blockers for v1.0.

### v1.0.0 — official foundation release

Public promise: stable Bash 5.3-compatible shell foundation, declared POSIX
profile and limitations, stable Bash++ Go-construct profile, and reproducible
official packages.

- Unix/Linux archives and packages, with checksums and provenance.
- macOS universal/per-architecture artifacts, notarization/signing and package.
- Windows artifacts, signing and package/installer.
- Cross-platform install, upgrade, uninstall, smoke, signature, and checksum
  tests. A cross-build alone does not satisfy this milestone.

### v1.1.0 — expanded GNU Coreutils compatibility

- The POSIX Go-utility profile is pre-v1.0 certification work, not a v1.1
  introduction: Bashy's Go utilities already exist and are the Lane B SUT.
- Expand the declared command and option/behavior matrix beyond the POSIX
  profile toward GNU Coreutils compatibility.
- Gate GNU Coreutils 9.11 differentials and uutils-derived tests by provenance
  and relevance; do not require foreign-suite 100% where it tests extensions.
- Preserve external-command fallback and provider reporting where commands are
  not promoted.

### v1.2.0 — stable agentic surface

- Promote the documented AgentOS/agentic commands and schemas to supported API.
- Require capability routing, safety/effect policy, structured output/schema
  versioning, observability, and benchmark evidence.
- Experimental agentic verbs may exist earlier, but v1.2.0 is their stability
  commitment.

### v1.3.x — Tessaro integration track

- **v1.3.0:** Sphere/P2P pairing and Ollama execution, with identity, trust,
  discovery, transport, upgrade, and offline/degraded-mode gates.
- **Recommended v1.4.0, or v1.3.1 only if no new public API:** cluster/DKS
  integration. DKS is a materially larger control-plane surface than Sphere;
  it should not inherit readiness merely because Sphere is green.

Minor milestone numbers are planning coordinates, not permission to bypass
SemVer. If a milestone requires incompatible public changes, it becomes the
next major release.

## Product version rules

| Change | Normal Bashy bump | Reason |
|---|---:|---|
| Bashy-only backward-compatible bug/security fix | patch | Corrects promised behavior |
| Upstream patch/toolchain update with no public semantic change | patch | Dependency/provenance refresh |
| Add compatible GNU Bash/POSIX/Go/Coreutils profile or feature | minor | New supported functionality |
| Expose additional Go-version constructs in Bash++ compatibly | minor | Language surface grows |
| Change default semantics, remove a profile, or break public CLI/language/schema | major | User migration required |
| Documentation/evidence-only correction | normally no release, or patch if republishing corrected metadata | No new API |

Consequences:

- GNU Bash 5.3 patch 006 does **not automatically** dictate Bashy `x.y.6`.
  It produces a compatibility-profile revision and a Bashy patch/minor/major
  chosen by user-visible impact.
- GNU Bash 6.x does **not automatically** force Bashy 2.0.0. Adding a Bash 6
  profile while retaining Bash 5.3 can be a minor; changing defaults
  incompatibly or dropping 5.3 requires a major.
- Go 1.26.x compiler/security updates are normally Bashy patches. Adding Go
  1.27 language constructs to Bash++ is normally a minor. A hypothetical Go 2
  only forces a Bashy major when Bashy’s stable language/API becomes
  incompatible.
- A new POSIX edition is a named profile. Adding it is normally minor; making
  it the default when behavior changes incompatibly is major.
- Patch releases remain reserved for backward-compatible fixes and upstream
  refreshes that do not add a promised public feature.

## Compatibility coordinates and release metadata

Every release publishes a machine-readable manifest and human support matrix:

```text
bashy:       1.0.0
gnu_bash:    5.3 + patch-level/hash
posix:       VSC-PCTS2016 / POSIX08 / IEEE 1003.1 campaign profile
go_build:    1.26.x
bashpp_go:   go1.26-profile-v1
coreutils:   GNU 9.11 reference; Bashy command-set revision/hash
tessaro:     none | sphere-v1 | dks-v1
platforms:   exact signed/package artifact matrix
evidence:    run IDs, totals, checksums, limitations
```

The binary should expose the same coordinates through a stable `--version` or
`version --json` schema. Annotated Git tags and release notes repeat them, but
the product tag remains simply `vX.Y.Z`.

## Maintenance cadence

- Watch stable GNU Bash releases/patches, POSIX editions and certification
  profiles, supported Go releases, GNU Coreutils, and packaging/signing policy.
- Open one compatibility campaign per upstream change; never silently float.
- Re-run the smallest relevant gate during development and the complete
  single-source release gate before tagging.
- Maintain at least the currently declared profile until a documented major
  migration, and state end-of-support dates before removal.
- Apply critical dependency/security fixes promptly even when they do not
  change Bashy’s feature roadmap.

References: [GNU Bash project](https://www.gnu.org/software/bash/),
[GNU Bash 5.3 announcement](https://lists.gnu.org/archive/html/bash-announce/2025-07/msg00000.html),
[Go release policy](https://go.dev/doc/devel/release),
[Go 1.26 release notes](https://go.dev/doc/go1.26), and
[The Open Group Issue 8 online publication](https://pubs.opengroup.org/onlinepubs/9799919799/).
