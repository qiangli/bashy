---
id: f115bd99ba9c
kind: task
title: bashy release — embed GoReleaser core, binmgr the tail
seq: 8
status: todo
priority: p2
created: 2026-08-12T17:38:59.548639Z
---

Plan of record (committed, umbrella repo):
  dhnt/docs/bashy-release-pipeline-design.md  (commit 6187df6, 271 lines)
Indexed in dhnt/docs/README.md (full abstract) and dhnt/CLAUDE.md (one-liner).
NOTE: that commit is NOT PUSHED yet — it exists only on the dev box. Push the
umbrella, or work in that checkout, before starting.

WHY NOW: ycode's Makefile is slated for retirement in favour of bashy dag
(ycode/DAG.md already covers every Makefile target). That exposed the release
path — not the build path — as the part with no bashy answer: release.yml
hand-rolls a build matrix, a package job, SHA256SUMS and a Homebrew formula
generator, and the umbrella's fleet-upgrade contract consumes those artifacts
by name. The Makefile stays until this lands.

DECIDED BY MEASUREMENT (do not re-litigate from intuition — see doc §4,
goreleaser v2.17.1, darwin/arm64, stripped):
  - whole CLI via cmd.Execute : 77.3 MB standalone (+75.7 vs baseline), 324
    linked modules (277 new to bashy), adds 6 HashiCorp MPL-2.0 deps + 1 with
    NO license file at all (ipfs/bbloom). bashy today is 78.0 MB, so this
    roughly DOUBLES it — resident in every shell on every host. FAILS both gates.
  - core only (pkg/config + pkg/archive + pkg/context) : 8.3 MB (+6.7), 63
    modules (39 new), and ZERO new license exposure — its single MPL-2.0 dep
    (cyphar/filepath-securejoin) is one bashy already ships. PASSES both gates.
  - No GPL/LGPL/AGPL anywhere in either set. GoReleaser itself is MIT.

=> T0 = embed the core, leave the tail as binmgr-managed external binaries.
This is NOT a compromise: it is what GoReleaser itself does — it execs docker
(20 call sites), snapcraft (14), gpg/gpg2 (14), cosign (11), flatpak (8),
syft (7), npm (7), makeself (6), upx (4), plus zig/rust/nix/ko. The 77 MB is
the cloud SDKs behind the blob-storage and publisher stages, which the plan
refuses anyway (Azure SDK 538 MB of source, google.golang.org/api 368 MB,
gocloud.dev, moby/moby/api, envoy, MCP registry).

T0 SCOPE:
  embedded    pkg/config (full .goreleaser.yaml schema, so an existing config
              works unchanged), pkg/archive, pkg/context; build/archive/checksum
              in-process over Go archive primitives + sha256sum + bashy git.
              `coreutils/pkg/pax` is currently only a manifest/safe-extraction
              planning kernel and must not be described as a shipped pax
              command or archive writer.
  binmgr'd    goreleaser itself for unembedded stages, plus cosign, syft, gpg,
              docker, upx. pkg/binmgr already fetches/sha256-verifies/caches/runs
              GitHub-released binaries (gitea, ollama, zot), and
              docs/external-binary-builtins.md is already the policy.
  exit        bashy release --snapshot reproduces today's ycode release.yml
              assets for the same tag, byte-for-byte where the build is
              deterministic.

BEYOND T0 — the three things GoReleaser structurally CANNOT do (doc §3):
  1. per-OS RUNTIME gate over dag --mesh. docs/per-os-release-gate.md is already
     policy: cross-compilation is NOT runtime testing. GoReleaser can build the
     darwin artifact but never RUN it. Exit criterion: a knowingly-broken windows
     binary must FAIL the release.
  2. Ensure:-backed attestation instead of an exit code (fleet-evidence-invariant).
  3. emit the entity.OutpostRelease fleet-upgrade envelope directly, preserving
     the prerelease split (registering != notifying).

OPEN BEFORE CODE (doc §7):
  - verb budget: docs/orchestration-verb-consolidation-audit.md requires a Why
    field before any new verb. 'release' must answer it, and the stages must be
    SUBCOMMANDS of one verb, never a new top-level family.
  - code home: coreutils/pkg/release (OSS, mounted via bashy internal/agentos),
    following the pkg/{weave,craft,secrets} precedent. Confirm before writing.
  - ycode's release.yml during the transition: its embed step is ALREADY DEAD
    (references internal/container/podman_embed/*, a directory that no longer
    exists). Delete it, do not port it.

TRAP, BANKED: MPL-2.0 §1.12 names the GPL/LGPL/AGPL when defining 'Secondary
License', so any license scanner testing for 'GNU Affero' BEFORE 'Mozilla
Public License' false-positives every MPL dep in the tree as AGPL. The first
audit pass here did exactly that and reported 6 phantom AGPL deps. Match MPL
FIRST. Reproduce the audit over LINKED modules (go list -deps), not go list -m
all, or the count reflects the module graph rather than what ships.

REFUSED (doc §6 — do not re-propose): announce (Slack/Twitter/Mastodon/Discord/
Telegram/webhook), AUR/scoop/chocolatey/snap/flatpak/krew/winget, blob-storage
upload (this IS the size/license failure), monorepo mode (submodules + warp
already answer it), and the whole Pro list (dmg/msi/nsis/app-bundles/nightlies)
until the v1.0.0 Windows/macOS installer milestones need it.
