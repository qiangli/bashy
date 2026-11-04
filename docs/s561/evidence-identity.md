# Certification evidence identity

Certification evidence may be reused across source commits **only while both
certification closure digests and the hashes of the exact staged certification
artifacts are unchanged**. A `CHANGED` result requires the affected lanes to
rerun before their evidence is attributed to the new candidate. A `SAME` result
does not excuse a changed artifact hash. Keep the original evidence, candidate
commit, build commands, and artifact hashes together; a source commit alone is
not an artifact identity.

The `cert_closure` in each [baseline manifest](baseline-manifest.json) and
[candidate manifest](candidate-manifest.json) contains one digest for each
certification build: `./cmd/bash` and `-tags=bashy_cert_base ./cmd/bashy`.
`scripts/cert-closure.py` runs `go list -deps -json` in `GOOS=linux`,
`GOARCH=amd64`, `CGO_ENABLED=0`, `GOWORK=off`. It records every linked package
and SHA-256 of its selected compilation inputs: Go, cgo, C, C++, Objective-C,
Fortran, assembly, SWIG, object and embedded files. Test files are excluded.
The product wrapper's effective `bashy_runtime_snapshot` tag and patched Go
runtime file are included. The digest covers the toolchain version, effective
build tags and flags, product wrapper inputs, package names, file names and
file bytes. It contains no module version or source commit; a pin change whose
linked package bytes stay identical therefore preserves the digest. The
commit-dependent `buildID` linker stamp is checked through the artifact hash,
not the closure digest.

Generate a manifest at a revision with
`GOWORK=off sh scripts/s561-baseline-manifest.sh REV > manifest.json` after
building any artifacts whose hashes the manifest should record. Compare the
closure with `sh scripts/cert-identity-check.sh manifest.json [REV]` (default
`HEAD`). The checker archives `REV`, recomputes both digests, prints `SAME` or
`CHANGED` for each build and lists changed packages/files, and exits 0 for both
same or 1 for either changed. A calculation error exits 2. Compare the
certification artifact SHA-256 values separately against the hashes recorded
with the evidence. The existing manifest `build.artifacts_sha256` values are
development-host outputs; they are **not** Linux certification artifact hashes.

Validation on 2026-11-04 with Go 1.27.2 and the fixed certification environment:

| Recorded revision → checked revision | `cmd/bash` | cert-base `cmd/bashy` | Changed linked inputs |
| --- | --- | --- | --- |
| `5a4bd03f` → `f1bcd711` | CHANGED (exit 1) | CHANGED | `github.com/qiangli/bashy/internal/cli/main.go`, `mvdan.cc/sh/v3/interp/builtin.go` |
| `f1bcd711` → `c6649c52` (then-current main; docs-only after candidate) | SAME (exit 0) | SAME | None |
| `9adb8acc^` → `9adb8acc` (only `internal/agentos/inbox.go` and its test changed) | SAME (exit 0) | SAME | None |

The historical `9adb8acc` comparison directly demonstrates that a change
outside `cmd/bash`'s linked graph leaves its digest unchanged. The
`5a4bd03f` closure matches the inventory's `c3dd9e38` baseline closure.
