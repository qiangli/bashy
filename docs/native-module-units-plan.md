# Native module units — Sprint 200, story 421

Compiled Go module recipes need original package identities: flattening an
imported type into main changes linker field-tracking names. The existing
Go-source native-unit lowering already preserves declarations, imports and
initialization; expose it through transpile-only `--go-native-unit`, requiring
`--source=go --go-import-path PATH`. Package maps supply checker dependencies.
The flag is incompatible with library-export and synthetic test-main modes.

The harness emits dependency units in go-list order and builds their original
module layout with unchanged recipe flags/environment. This does not change
ordinary flattened transpilation or Classic shell execution.

The independent regression compares native and emitted three-package programs,
including initialization order, original reflect package identity, per-unit
source maps, and field tracking with `GOEXPERIMENT=fieldtrack` plus
`-ldflags=-k=main.fieldTrackInfo`. Invalid option combinations are rejected.
The authenticated upstream leaf remains the final acceptance evidence.
