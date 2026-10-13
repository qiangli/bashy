---
id: 1a4b45bfe7b3
kind: bug
title: Align Prima preparation source guard with reviewed 0.1.1 pin
seq: 1914
status: done
priority: p0
created: 2026-10-13T15:14:30.769445Z
weave: 92
assignee: codex-gpt6-astra
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
closed: 2026-10-13T15:19:01.470715Z
closed_by: s436-codex-manager
---

Hosted candidate bashy-bashy-prima-0.1.1-1 run 38420049892 failed all four prepare steps before compilation: fork/bashy-build/prepare.py SOURCE still f4ea9845 despite reviewed PIN 542d01932a88. Update builds/prima.cpp/files/prepare.py and generated fork to enforce exact 0.1.1 source via PIN or same SHA, add focused mismatch regression to check_test.py. Preserve fail-closed source/license/nng guards. Commit worker story file and code with Sprint/Story trailers; no push, no Dragon native build/container/full suite. Manager will merge, gate, cut immutable -2 candidate.

## Implementation plan

Align the prepare source constant with the reviewed 0.1.1 SHA, update the
dependency source/SBOM metadata, regenerate the fork, and test both prepare
copies against the reviewed and rejected inputs. Preserve all source, license
and nng guards. Run the focused producer gate and commit for manager review.

## Delivery evidence

- Both prepare scripts enforce reviewed source 0.1.1 at
  `542d01932a88372a184037c5939048f355dcbd57`. Source cleanliness, source
  license, nng commit and nng license assertions remain unchanged.
- Updated dependency manifest source tag/commit and inference-core version/purl
  in authoring files and generated fork, as requested in sprint continuity.
- Regression exercises both prepare copies: reviewed input succeeds; old SHA,
  unrelated SHA, dirty source, wrong source license, moved nng tag and wrong
  nng license fail before overlay writes. Metadata tests tie both manifests
  and preparation constants to the reviewed declaration/PIN.
- Focused gate: `bashy gate --command 'bash builds/check.sh prima.cpp && python3 -B builds/prima.cpp/check_test.py' --json`.
  Actual verdict `passed`, check `passed: true`, exit 0. Fork reproduction,
  structural/shell checks, 11 shared egress cases, 19 toolchain cases, and
  38 Python tests passed. Optional mutation probe was not run.
- `git diff --check` passed. No native build, container, full suite or push.
  Manager owns merge, immutable -2 candidate and hosted build/signing proof.
