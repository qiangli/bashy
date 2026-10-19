---
id: 20aa3b1da3e9
kind: bug
title: Resolve Prima compiled-input license audit findings without weakening policy
seq: 1915
status: done
priority: p0
created: 2026-10-13T15:35:52.555099Z
weave: 93
assignee: codex-gpt6-astra
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
closed: 2026-10-13T16:20:17.166293Z
closed_by: s436-codex-manager
---

Hosted candidate 0.1.1-2 run 38420930156: all four native/no-egress lanes PASS; audit failed ScanCode with 604 NNG files `mit AND cpal-1.0`, 4 first-party common/zmq_nng_shim.hpp `mit AND mpl-2.0` (comment says instead of MPL), 4 NNG src/supplemental/sha1/sha1.c `fpl` (real Freeware Public License). Pinned NNG v1.9.0 @29aff2 has canonical MIT headers/Licence; source clone /tmp/s436-nng-v1.9.0. Resolve safely: disable unneeded NNG WebSocket/WSS so SHA1/FPL is absent from compiled-input closure, preserve TCP ring. For CPAL/MPL scanner false positives, inspect exact evidence and implement narrow reviewed-file logic tied to pinned NNG MIT license/hash and source-text absence of those terms, or another fail-closed precise method; do not globally allow CPAL/MPL/FPL. Add focused positive/negative tests, generated fork parity, retain full four-platform compiler-input audit, OSV/grype/SBOM/no-egress. Commit worker story and code with trailers; no push, no dev-host native build/container/full suite. Manager merges then immutable -3 hosted run.


## Implementation plan

Inspect hosted failure expressions and pinned source/license bytes. Disable
unused NNG WebSocket/WSS and SHA1 while preserving TCP. Adjudicate only exact
reviewed path/hash/expression tuples with pinned root license evidence;
retain all raw findings and reject changed or unreviewed inputs. Add focused
positive/negative tests, regenerate the fork, run the bounded worker gate,
and commit this story plus code for manager integration.

## Delivery evidence

- Implemented WS/WSS/SHA1 OFF and TCP ON; an independent compiler-input guard
  rejects the entire NNG SHA1 directory even if ScanCode misses its license.
- Reviewed 152 NNG paths at 29aff2 and the exact 0.1.1 first-party shim at
  542d0193. Runtime checks enforce file hashes, source/dependency pins,
  platform-local root license hashes and exact scanner expressions.
- CPAL/MPL/FPL remain absent from the global permissive allowlist. Raw
  ScanCode findings stay intact; corrections are annotated in the existing
  signed license report. OSV/Grype/SBOM/no-egress stages remain required.
- Four downloaded hosted input archives: 880 inputs hash-verified; replay of
  logged failures yields 608 precise corrections and 12 SHA1 rejections.
  No new native build or ScanCode scan was performed locally.
- `bashy gate --command 'bash builds/check.sh prima.cpp && python3 -B builds/prima.cpp/check_test.py' --json`:
  verdict `passed`, check `passed: true`, exit 0. Inspected underlying output:
  48 Python tests, 11 shared egress cases, 19 toolchain cases and generated
  fork reproduction/structural checks pass. Optional mutation probe not run.
- Local CMake executable absent; optional script-mode option probe could not
  run. Pinned CMake source was inspected; actual four-platform closure proof
  remains the hosted -3 run. No native build/container/full suite/push.
- Manager owns merge and immutable -3 candidate; hosted audit/signing and
  admission remain pending. Detailed evidence: builds/prima.cpp/README.md.
