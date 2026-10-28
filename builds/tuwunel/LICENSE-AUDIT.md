# Sprint 513 tuwunel v1.9.4 license audit

Story #2752 (`77b364d18111`), 2026-10-28. Scope: Bashy's pinned,
signed `qiangli/tuwunel` release `bashy-v1.9.4-2`, not an upstream or
proposed Windows OCI image.

## Pinned source and license choices

The build declaration pins `matrix-construct/tuwunel` v1.9.4 to
`f473d9a4a8342514302884041e0ccfea1ad503fc` and builds with Rust
1.95.0. The [release SBOM](https://github.com/qiangli/tuwunel/releases/download/bashy-v1.9.4-2/tuwunel-v1.9.4.cdx.json)
identifies `rust-rocksdb 0.52.0` and `rust-librocksdb-sys
0.48.0+11.8.1` at `matrix-construct/rust-rocksdb@24dad4d15b47d932b9f060c1b9520c0d1e6b1fe5`.
The exact [Rust binding manifest](https://github.com/matrix-construct/rust-rocksdb/blob/24dad4d15b47d932b9f060c1b9520c0d1e6b1fe5/librocksdb-sys/Cargo.toml)
declares MIT/Apache-2.0/BSD-3-Clause; this distribution elects Apache-2.0.
`rust-rocksdb` itself declares Apache-2.0. Its RocksDB submodule is
`facebook/rocksdb@d8a89161c6a53e79d61694289d2b78be36e6ca12`.
The exact [RocksDB README](https://github.com/facebook/rocksdb/blob/d8a89161c6a53e79d61694289d2b78be36e6ca12/README.md)
offers Apache-2.0 OR GPL-2.0; this distribution elects Apache-2.0 and
copies that commit's `LICENSE.Apache` into future release payloads.
No GPL election is needed for those components.

## Unresolved service-policy issue

The shipped SBOM also reports `as_variant 1.3.0` as **MPL-2.0, required**.
Four Ruma packages depend on it, including `ruma-events 0.34.0`.
The exact crate checksum is
`9dbc3a507a82b17ba0d98f6ce8fd6954ea0c8152e98009d36a40d8dcc8ce078a`;
its manifest declares MPL-2.0 and its source defines a declarative macro.
This is a dependency of the service build, not evidence of a build-only
compiler tool. Whether its macro expansion appears in the binary requires
source/build review; the SBOM alone cannot settle that.
Umbrella `docs/bashy-service-supply-chain-policy.md` Rule 1 disallows an
MPL-licensed long-lived server. The broader persistence catalog allowing MPL
does not override that rule. **Do not assert permissive-only service compliance
or promote a new tuwunel build until this is resolved with the operator.**

## Artifact notice gap and scoped source repair

The `bashy-v1.9.4-2` GitHub release lists four raw binaries, config,
CycloneDX SBOM, SHA256SUMS and Sigstore bundles. It has no LICENSE or
NOTICE asset. The sampled `tuwunel-linux-amd64` is a stripped raw ELF;
a `strings` check found none of the standard Apache, MPL or BSD notice
headings. Yoke's `binmgr.Ensure` downloads and verifies the binary only;
`license_url` is metadata and does not install a notice. These checks do not
prove the absence of every embedded attribution, but they establish that
the present release/installer supplies no explicit license accompaniment.

This source change stages signed `LICENSE.Apache`, `LICENSE.MPL-2.0`, and
`THIRD_PARTY_NOTICES.md` assets in the next release template. The generic
release job includes every `RELEASE_ASSETS` entry in SHA256SUMS, signs each
asset, and publishes it. **The existing v1.9.4-2 release is unchanged**;
there is no new build or release from this audit. A full attribution review
of the remaining SBOM components and any compiled native libraries is still
required before claiming complete binary or OCI compliance. No Windows OCI
image was built: the operator stopped that work, so no OCI notice claim exists.
