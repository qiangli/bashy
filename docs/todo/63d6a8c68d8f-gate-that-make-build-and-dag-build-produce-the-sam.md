---
id: 63d6a8c68d8f
kind: task
title: Gate that make build and dag build produce the same artifacts
seq: 244
status: todo
priority: p1
created: 2026-09-06T11:33:26.160232Z
sprint: 130
---

There are TWO build systems and NOTHING tests that they agree. Verified: no
script under scripts/ compares them (the dag-* scripts are argo/k8s emitters,
the posix-parity-* scripts are shell conformance).

That gap already shipped a defect. `dag build` omitted the native pre-Go signal
launcher that `make build` compiles from native/siglaunch.c.in on linux/darwin,
so `bashy dag install` installed the Go payload AS ~/.local/bin/bashy and
~/.local/bin/bash — 94644306 and 8117522 bytes where make leaves a 34232-byte
launcher beside its payload. Fixed in dag.md (story 5eacb678ffdb, sprint 115),
but only because someone happened to diff the byte sizes against a prior
install. Nothing would have reported it.

WHY A GATE AND NOT CARE: the two recipes are maintained by hand in two files
(Makefile build-bash/build-bashy, dag.md ### build) and neither references the
other. Any future edit to one re-opens the same hole, and the failure mode is
silent — see the companion story on installbashy.

SHAPE: build through both paths into separate output dirs and compare the
ARTIFACT SET, not the bytes. Go builds are not reproducible across two
invocations here (the ldflags carry a BUILD_ID that includes a -dirty suffix),
so compare: the set of files produced, which are native vs Go objects (`file`
or a size-class check), and that every installed name has its .real sidecar on
linux/darwin. A byte-identical comparison will flap and get disabled.

SCOPE NOTE: the honest fix may be to delete one of the two recipes rather than
gate both. dag is the favored runner and the Makefile is the incumbent; if they
converge on one, this story becomes "make the other a thin delegator" and the
gate shrinks to asserting the delegation. Decide that before writing the
comparison.
