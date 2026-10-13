---
id: ca7bd88a349f
kind: bug
title: Allow reviewed Prima 0.1.1 shim test loopback ports
seq: 1913
status: done
priority: p1
created: 2026-10-13T15:07:30.869338Z
weave: 91
assignee: codex-gpt6-sol
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
closed: 2026-10-13T15:16:39.846016Z
closed_by: s436-codex-manager
---

Reviewed first-party source tag bashy-prima-0.1.1 @542d0193 adds nng shim tests on fixed loopback ports 5602,5603,5604,5605. Bashy no-egress currently allows 5599-5601 plus engine data 19000/19001 and signal 19100/19101. Update only builds/prima.cpp/files/no-egress-test.sh, generated fork counterpart, focused check_test.py tests to allow exact ring 5599-5605 and four engine ports on Linux verdict and Darwin sandbox; preserve deny-all non-loopback and positive control. Do not edit shared egress verdict or worker #89 exercise.py; avoid README/tool.env because #90 owns repin. Commit worker story file with trailers; no push/native Dragon build/container/full suite. Manager merges sequentially before hosted candidate.
