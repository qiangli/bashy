# Official runner build recipe (Sprint 436, Story 1917)

Source authority: Yoke qualification at `5e8c4687`, official Ollama v0.21.2
`590109c8352e8d5a6206e8909b518a54a2b0a7b8`.

1. Inspect the existing held runner release draft without importing stale pins.
2. Add an independent builds/ollama declaration and generated fork workflow
   for five native hosts, full Git source and upstream cmd/runner with cgo.
3. Package deterministic gzip and raw/compressed digest metadata, declare the
   exact signed artifact contract and retain explicit runtime/security holds.
4. Exercise renderer and guard tests with fixture sources/fake compiler only;
   commit on the current worker branch, no push.

Implementation and integration contract: [builds/ollama](../builds/ollama/README.md).
The current worker branch starts from main, not the older draft. Preserve the
remote draft and deliver an additive commit for the manager to layer onto it;
do not switch branches or merge the held draft into this workspace.

Validation: focused recipe tests and `bash builds/check.sh ollama` (including
shellcheck/actionlint) are the local gate. These results establish recipe
behavior only. Five native builds, runtime/no-egress proof, vulnerability and
compiled-input license admission, SBOM and signed release verification remain
open. No Dragon heavy build, container, full suite or publication is authorized.

## Local evidence (2026-10-13)

`bash builds/check.sh ollama` passed after inspecting its results: 11 shared
no-egress verdict cases, 19 shared toolchain gate cases, three recipe test
methods (including all five mocked native targets), generated-tree comparison,
Bash syntax, installed shellcheck and actionlint. Guards reject dirty/missing
source, wrong commits, missing tags, module ZIPs, cross-host builds and
unsupported targets. Gzip round-trip, zero timestamp/filename and both digests
are checked. Separately rendered all 11 pre-existing tool declarations and
compared each against its committed fork tree: all unchanged. `git diff
--check` passed. No native/security release gate was run or claimed.
