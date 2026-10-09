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

## rc.2 follow-up

Measure the published v1.0.0-rc.2-dev assets, with both ZIPs and outpost
verified against checksums.txt. Use the approved bashy main harness,
outpost 62a152f, bashsharp 00e6e0b and the recorded bashsharp-tests HEAD.
If printf reports a same-second diagnostic, retain up to three isolated
TESTS=printf reruns verbatim before deciding whether it is deterministic.

The approved S1 source driver builds products by default. Preserve its source
and generate a release-only adaptation with
`scripts/prepare-windows-s1-release.py SOURCE DESTINATION`: copy the verified
pair from ROOT/product, retain source-unit gates as a separate evidence class,
and exclude the four Ruby-backed gates under D11 without reporting skips.
The adapter refuses unrecognized build steps or Ruby gate inventories.

D15 authorizes temporarily replacing the existing managed-service task. Save
its XML, principal, binary version and hash first; restore in a finally block,
then check Running, the binary hash and principal SID against the saved XML.
Do not serialize CIM objects and assume their convenience properties survive
as JSON fields. Test user mode and system mode when elevated. Record failures
before SSH/SFTP as coverage gaps, even if installation copied the binaries.

Run the Y7 local isolated workflow without a BASHY override, putting the
release directory first on PATH. Snapshot real-home session state before and
after; rc.2 does not include the forthcoming genie state-isolation repair.

Do not launch other Windows lane processes while O4 temporarily changes User
registry environment variables: a new SSH process can retain those values
after restoration. Serialize O4 first, then explicitly reload the original
User values for XDG_CONFIG_HOME, XDG_CACHE_HOME, BASHY_HOME and
OUTPOST_ADMIN_ADDR into the next lane's Process environment. Record the
effective paths and reject any remaining O4 scratch path before testing.
