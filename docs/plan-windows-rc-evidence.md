# Windows release candidate evidence

Sprint 379, stories B1, S1, O4, Y7 and R3. Use the published Windows amd64
archives for v1.0.0-rc.1-dev, verified against the release checksums, with
harness sources from bashy 6cb8b20f and its module pins.

Run the gates in that order, recording exact commands, binary digests, counts,
durations and raw logs in the uncommitted REPORT-379.md. Keep unresolved
failures visible. If the tagged harness cannot select published executables,
add a tested harness option; do not rebuild the product being measured.
The optional BASH53_RELEASE_DIR selects bash.exe and bashy.exe together and
retains builds only for the fixture runner and data preparation helpers.

S1 requires an explicit corpus revision and the gate script revision when they
are absent from the pinned tree. Record missing provenance as blocked rather
than silently selecting a later checkout. O4 must restore host state on every
exit. Y7 uses isolated state. R3 tests the extracted release ZIP and records
whether this prerelease actually published install-channel manifests.

Commit harness changes locally with story trailers, leave the evidence report
uncommitted, and retain logs without leaving services or scratch processes.
