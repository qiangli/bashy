---
id: 67007e1c7e32
kind: chore
title: Repin Prima producer to first-party shutdown repair 0.1.1
seq: 1912
status: done
priority: p1
created: 2026-10-13T15:05:35.688918Z
weave: 90
assignee: codex-gpt6-astra
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
closed: 2026-10-13T15:15:51.967047Z
closed_by: s436-codex-manager
---

First-party qiangli/prima.cpp source main merged 542d01932a88372a184037c5939048f355dcbd57; annotated/pushed tag bashy-prima-0.1.1 peels to exact commit. Update Bashy builds/prima.cpp declaration/PIN/tool.env/README and generated fork to source tag 0.1.1 and exact SHA; release version becomes bashy-bashy-prima-0.1.1-1, same raw four-platform asset contract and exact workflow identity. Update focused tests; preserve fail-closed source verification and no-egress. Do not change unrelated shared pipeline or binary consumer pins; no dev-host native build/container/full suite, no push. Commit story file and source diff with trailers. Coordinate independent #89 no-egress/log fix; manager merges sequentially.

## Delivery evidence

- Producer declaration, generated PIN/workflow, README and reference asset verifier
  now select source `bashy-prima-0.1.1` at
  `542d01932a88372a184037c5939048f355dcbd57`, candidate
  `bashy-bashy-prima-0.1.1-1`. Four raw assets and exact workflow identity retained.
- Read-only `git ls-remote` verified main and peeled annotated tag at that SHA;
  tag object `74bf7765b1e415e52edc9c6f2013d30cf24403c5`. The generated source gate
  passed against the pristine local first-party checkout.
- Focused gate: `bashy gate --command 'bash builds/check.sh prima.cpp && python3 -B builds/prima.cpp/check_test.py'`.
  Fork reproduces; 11 shared egress and 19 toolchain cases pass; 33 Python tests
  pass, including exact identity and fail-closed source mismatch/tag/dirty cases.
  Plain gate reports PASS; JSON capture reports check passed=true but overall
  verdict=abstained because the optional mutation probe did not run.
- Coordinated scope with s436-codex-manager for sequential merge with #89.
  No no-egress implementation, shared pipeline or binary consumer pins changed.
- No native build, container, full suite, push or release. Hosted four-platform
  no-egress/signing evidence and independent #89 integration remain manager work.
