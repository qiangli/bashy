---
name: go-repo-health
description: Verify a Go repository is healthy — the build compiles and the tests pass — with one machine-verified, attested command. Use before starting work in an unfamiliar Go repo, after a merge, or as a convergence gate. Run `bashy skills run go-repo-health` from the repo root; exit 0 iff the contract held.
metadata:
  requires: "has=go"
  check-build: "go build ./..."
  check-tests: "go test ./..."
---

# go-repo-health — attested Go repo health check

The reference **dual-bundle** skill: beside this prose sits `skill.dhnt`,
a canonical machine face carrying a content identity, a success contract
(`build-ok ∧ tests-green`), and a read-only effect cap. Any
Skills-capable tool can follow this file as prose; a dhnt-aware runtime
(bashy) executes and *attests* it.

## Use

From the root of a Go repository:

    bashy skills run go-repo-health

- The contract is machine-verified — `run` exits 0 iff `go build ./...`
  and `go test ./...` both succeed — and every run appends a
  re-checkable attestation to the host-local store ("it held here-now").
- Environment-gated: hosts without a `go` toolchain are never offered
  this skill (`bashy skills list` filters it out; `bashy skills verify
  go-repo-health` explains why).
- Effect cap is read-only (`efefecato reada`): the skill declares no
  write authority, and the pre-flight audit refuses any variant that
  tries to add steps without raising the cap.

## Bindings

The concrete commands live in this file's `metadata` (`check-build`,
`check-tests`) — the executor-side half of the dual bundle. Per-host
overrides learned by `run --adapt` are stored beside the skill store
(`bindings/go-repo-health.json`), never written back into this file.
