# tuwunel v1.9.4 Bashy build: license choices and source

This file accompanies a Bashy-built tuwunel release. It records the selected
license for the pinned RocksDB source and the named Rust dependencies. The
CycloneDX SBOM shipped beside the executable identifies the remaining crates;
this notice is not a substitute for reviewing their license obligations.

| Component | Pinned source | License used for this distribution |
| --- | --- | --- |
| tuwunel 1.9.4 | `matrix-construct/tuwunel@f473d9a4a8342514302884041e0ccfea1ad503fc` | Apache-2.0 |
| rust-rocksdb 0.52.0 | `matrix-construct/rust-rocksdb@24dad4d15b47d932b9f060c1b9520c0d1e6b1fe5` | Apache-2.0 |
| rust-librocksdb-sys 0.48.0+11.8.1 | same Rust binding commit | Apache-2.0, elected from MIT OR Apache-2.0 OR BSD-3-Clause |
| RocksDB 11.8.1 source | `facebook/rocksdb@d8a89161c6a53e79d61694289d2b78be36e6ca12` | Apache-2.0, elected from Apache-2.0 OR GPL-2.0 |
| as_variant 1.3.0 | crates.io checksum `9dbc3a507a82b17ba0d98f6ce8fd6954ea0c8152e98009d36a40d8dcc8ce078a` | MPL-2.0; no permissive election |

`LICENSE.Apache` is the Apache text from that exact RocksDB commit;
`LICENSE.MPL-2.0` is the license text included in the exact as_variant crate.
The tuwunel and Rust binding source license files are available at the pinned
source commits above. The as_variant source is available as the exact
`as_variant-1.3.0.crate` from crates.io. The MPL dependency is a policy blocker
for Bashy's permissive-only long-lived-service rule until reviewed and resolved;
publishing this notice does not resolve that policy conflict.

Source URLs:

- https://github.com/matrix-construct/tuwunel/tree/f473d9a4a8342514302884041e0ccfea1ad503fc
- https://github.com/matrix-construct/rust-rocksdb/tree/24dad4d15b47d932b9f060c1b9520c0d1e6b1fe5
- https://github.com/facebook/rocksdb/tree/d8a89161c6a53e79d61694289d2b78be36e6ca12
- https://crates.io/crates/as_variant/1.3.0
