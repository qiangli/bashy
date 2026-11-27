###########################################################################
##  GENERATED FILE — DO NOT EDIT.                                        ##
##  Produced from dag.md by `bashy dag --makefile`. Edit dag.md instead, ##
##  then regenerate; manual changes are overwritten and fail --check.    ##
###########################################################################
# Tasks live in dag.md (a bashy dag file); this Makefile only forwards.
#   bashy dag <target>           run a target (its dependencies first)
#   bashy dag --list             list targets
#   bashy dag --dry-run <target> print the plan, run nothing
#   bashy dag --skill DIR        export these tasks as an Agent Skill
# Regenerate with: bashy dag --makefile  (verify: bashy dag --makefile --check)

# List the dag targets, then how to run them natively.
help: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag --list; echo "Run: bashy dag <target> | skill: bashy dag --skill DIR | more: bashy help"

# Build both independent binaries into bin/ (bash = pure drop-in from cmd/bash; bashy = AgentOS shell from cmd/bashy).
build: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag build

# Build the full **unix host** bashy: the container/LLM engines (`bashy podman`/`ollama`, `-tags bashy_engines`, cgo + btrfs/MLX) and the observability stack (`b…
build-host: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag build-host

# Install the built pair into the shared dhnt user bin (`$DHNT_BIN_DIR`, default `~/.local/bin`).
install: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag install

# Run all test prerequisites matching the Makefile test contract, then all Go tests.
test: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test

# Hermetic regression for the freshness gate — proves fresh accepted, stale rejected and left unchanged, and missing toolchain fails closed.
test-meet-spa-fresh-regression: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-meet-spa-fresh-regression

# REQUIRED non-mutating gate — build a fresh meet SPA and compare it against the tracked artifact WITHOUT promoting.
test-meet-spa-fresh: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-meet-spa-fresh

# Prove a failed Go build cannot fall through to the native launcher compiler or the installer and reuse stale binaries.
test-build-fail-closed: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-build-fail-closed

# Verify concurrent test-lane naming and ownership wiring.
test-isolated-lanes: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-isolated-lanes

# Focused test for the release QA lane (xfail-card rule + known-failing/skipped summary counts).
test-release-qa-lane: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-release-qa-lane

# Type-check every combination of the two host build layers.
test-build-tag-matrix: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-build-tag-matrix

# Prove requested container modes survive a helper shell that consumes BASHY_BASHPP (fake OCI/build commands; no container).
test-bash-container-mode: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash-container-mode

# Rebuild and fail closed unless the Cloudbox artifact is static Linux amd64, purego-free, command-smoked, and scratch-runnable when an OCI runtime is available.
verify-bashy-scratch: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag verify-bashy-scratch

# Gate certified bsh package initialization and 100 background jobs on Linux.
test-startup-linux: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-startup-linux

# Release gate: Hello World shaping lifecycle on the BUILT binary, model-free.
test-shaping-smoke: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-shaping-smoke

# Proves test-shaping-smoke can fail: the app's smoke check is deliberately made to expect the old greeting, so verification must fail and the smoke must exit no…
test-shaping-smoke-regression: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-shaping-smoke-regression

# Two disposable hosts exercise real org/upstream Git transport and gated shaping pull, local verification, approval and undo.
test-shaping-promotion: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-shaping-promotion

# Exercise the owner and member site MCP paths inside the locked Ubuntu base.
test-group-shaping-e2e: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-group-shaping-e2e

# Exercise the field loop (incident heal, owner feed, scheduled feed with the off switch) with fakes inside a throwaway HOME.
test-field-loop-e2e: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-field-loop-e2e

# Exercise `bashy contain --net deny --dir` driving real `bashy genie` through `bashy llm session forward` against a fake cligw unix-socket door in the locked of…
test-contained-genie-e2e: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-contained-genie-e2e

# Opt-in pre-release Hello World lifecycle through the real shaping delegate.
test-shaping-real-agent: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-shaping-real-agent

# The formatting gate.
fmtcheck: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag fmtcheck

# Build a platform-appropriate bashy Podman engine binary and smoke-test the container path.
test-podman: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-podman

# Build independent source-owned release products in the locked previous-release image, with warm caches, bounded target parallelism and measured stages.
dist: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag dist

# Inspect the immutable-source/platform/budget inputs without building or publishing.
dist-plan: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag dist-plan

# Ensure the optional GNU Bash 5.3 fixture data is present.
test-bash-data: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash-data

# **GNU Bash 5.3 baremetal compatibility test** — the canonical native host conformance gate.
test-bash: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash

# Prepare the native Bash 5.3 baremetal lane once per host for chunked fanout.
test-bash-prepare: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash-prepare

# Run the native Bash 5.3 harness against an already prepared checkout.
test-bash-run: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash-run

# List the GNU Bash 5.3 fixtures known to the bashy-native harness.
test-bash-list: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash-list

# Run one GNU Bash 5.3 distributed chunk.
test-bash-chunk: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash-chunk

# Run any DAG target as deterministic chunks and aggregate result lines.
dag-fanout: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag dag-fanout

# Run a generic fanout target repeatedly until chunking settles or `MAX_ROUNDS` is reached.
dag-fanout-tune: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag dag-fanout-tune

# Run the GNU Bash 5.3 suite through the generic DAG fanout target on the native host OS.
test-bash-chunks: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash-chunks

# Check that the standard distributed Bash 5.3 test fleet is reachable and that each remote checkout exposes a usable bashy binary.
test-bash-fleet-check: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash-fleet-check

# Prepare the standard distributed Bash 5.3 test fleet.
test-bash-fleet-prepare: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash-fleet-prepare

# Run the GNU Bash 5.3 chunked suite through the standard development fleet using the container-normalized lane.
test-bash-chunks-fleet: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash-chunks-fleet

# Tune GNU Bash 5.3 chunk assignments with bounded repeated fanout runs.
test-bash-chunks-tune: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash-chunks-tune

# Run the authoritative 86-fixture Bash 5.3 gate in a self-contained Linux image through bashy podman (mode 0, BASH53_BASHPP=0).
test-bash-container: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash-container

# Run the same hermetic 86-fixture gate with Bash++ selected explicitly for each top-level testee process (mode 1, BASH53_BASHPP=1).
test-bash-container-bashpp: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash-container-bashpp

# Release gate, sealed 86/86 in both modes (off then on).
test-bash-gate: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash-gate

# Run the licensed POSIX Profile B **shell-only** release gate, explicitly and only from a clean, pushed Bashy checkout at the supervisor-approved commit.
test-posix-shell: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-posix-shell

# The offline bashy image from this checkout (Sprint 227, "Bashy is all you need"): build the static `bashy_scratch` linux artifact for `BASHY_IMAGE_ARCH` (defau…
build-image: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag build-image

# The airgap gate (Sprint 227): build the offline image (`build-image`) and prove every row of `docs/airgap-image.md` — shell modes, every builtin, every coreuti…
smoke-airgap: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag smoke-airgap

# "Bashy is all you need" (Sprint 227), as one gate: from the PUBLISHED release archive of `SELF_TAG` (e.g.
smoke-self-contained-image: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag smoke-self-contained-image

# Prepare the GNU Bash 5.3 container-normalized lane once per host.
test-bash-container-prepare: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash-container-prepare

# Run the GNU Bash 5.3 container harness against an already prepared checkout.
test-bash-container-run: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash-container-run

# Run GNU Bash 5.3 compatibility chunks in the container-normalized lane.
test-bash-chunks-container: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash-chunks-container

# Tune GNU Bash 5.3 container-normalized chunk assignments.
test-bash-chunks-container-tune: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash-chunks-container-tune

# **yash POSIX (`-p`) conformance test** — the POSIX-conformance frontier metric (an INFO probe, not a 0/1 gate).
yash: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag yash

# List chunkable yash POSIX suite files.
yash-list: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag yash-list

# Run one chunk of the yash POSIX INFO suite.
yash-chunk: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag yash-chunk

# Run the yash POSIX INFO suite through generic DAG fanout.
yash-chunks: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag yash-chunks

# Tune yash POSIX INFO suite chunk assignments with bounded repeated fanout runs.
yash-chunks-tune: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag yash-chunks-tune

# go mod tidy + gofmt -s -w .
tidy: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag tidy

# Remove built binaries.
clean: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag clean

# Build ONLY `bin/bash` — the pure Bash 5.3 drop-in from cmd/bash (import graph never includes coreutils; ~5.7 MB).
build-bash: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag build-bash

# Build ONLY `bin/sh` — the pure strict POSIX sh for the opt-in shell conformance gate.
build-sh: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag build-sh

# Build ONLY `bin/bashy` — the AgentOS shell from cmd/bashy.
build-bashy: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag build-bashy

# Build the lean, static linux Bashy profile used by Cloudbox and by the offline image (`bashy self image`).
build-bashy-scratch: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag build-bashy-scratch

# Ensure the pinned GNU Bash 5.3 fixtures are present.
test-bash-fixtures: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash-fixtures

# Build both binaries against the Go FIPS 140-3 Cryptographic Module (GOFIPS140=v1.0.0).
build-fips: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag build-fips

# Compile the recho/zecho/xcase C helpers the bash-5.3 suite needs (the only place `cc` is invoked — for test fixtures, not for bashy).
test-bash-helpers: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash-helpers

# The **canonical 86/86 gate** — the bash-5.3 fixture suite fanned across cores via the single `tools/bash53suite` runner.
test-bash-parallel: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bash-parallel

# Contained uutils/coreutils suite vs the pure-Go multicall (INFO metric).
test-uutils: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-uutils

# Prepare the dependency-only OCI image.
prepare-uutils-image: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag prepare-uutils-image

# Bounded containment/parser regression tests using synthetic logs and a stub OCI runtime only.
test-uutils-safety: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-uutils-safety

# zsh-own-suite Tier-0 scoreboard (tools/ztst runner; INFO metric, not a gate).
test-zsh: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-zsh

# Install the git pre-push hook (pin-drift guard).
hooks: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag hooks

# Print the current uutils failure list (INFO; the -list companion to test-uutils).
test-uutils-list: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-uutils-list

# Print the current bashy-specific zsh failure list (INFO; companion to test-zsh).
test-zsh-list: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-zsh-list

# Submit the self-contained GNU Bash or yash conformance image as an Indexed DKS Job.
dks-conformance-apply: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag dks-conformance-apply

# Aggregate one completed DKS conformance Job.
dks-conformance-result: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag dks-conformance-result

# Submit one native-platform task to a real `vk-native` host.
dks-native-apply: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag dks-native-apply

# Validate one native-platform DKS Job from its bounded terminal RunRecord.
dks-native-result: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag dks-native-result

# Fail-closed pre-tag release gate over explicit completed DKS Jobs.
dks-release-gate: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag dks-release-gate

# Per-OS release smoke (the release gate): download the released `$BASHY_TEST_VERSION` asset for the host OS/arch, verify its sha256 against the published `check…
qa: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag qa

# Build the reusable Ubuntu/glibc Bashy base image.
build-bashy-oci: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag build-bashy-oci

# Smoke the already-built Bashy base with no network, a read-only root, dropped capabilities, and temporary work directories.
smoke-bashy-oci: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag smoke-bashy-oci

# Offline structural checks for the OCI base contract.
test-bashy-oci-policy: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-bashy-oci-policy

# Apply the seed freshness rule to VERSION (for release builds).
check-seed-bands: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag check-seed-bands

# Release gate.
govulncheck: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag govulncheck

# Shipped Go dependency license gate (bashy, yoke, sh, outpost + vendored/third_party) Ported from the Makefile in Sprint 660 R3; the body is the old recipe's co…
license-gate: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag license-gate

# Stress repeat (25x by default, AWD_STRESS_RUNS) of the installed-binary awd concurrency e2e against a throwaway install dir — the repetition gate for the inter…
test-awd-installed-stress: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-awd-installed-stress

# yash POSIX (-p) conformance scoreboard — the yash analogue of test-bash.
test-yash: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-yash

# print the current bashy-specific yash failure list (suite line desc).
test-yash-list: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-yash-list

# drive `bashy chat` interactive under a real pty against an installed agent — asserts the governed-launcher contract (native launch · registry · steer · capture…
smoke-chat: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag smoke-chat

# bashy local-model e2e (Sprint 379 Y2) — pull a tiny model, `llm up`, chat through the OpenAI + Anthropic paths, assert non-empty answers.
smoke-llm-local: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag smoke-llm-local

# Explicit installed-product smoke for Sprint 183's unchanged nanochat and mini-SWE-agent checkouts (NANOCHAT_ROOT and MINISWEAGENT_ROOT name them).
smoke-python-imports: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag smoke-python-imports

# Installed-product smoke for the examples/dag Python front doors (Sprint 185): `bashy awd ROOT -- bashy dag -f examples/dag/<repo>/dag.md` against pinned cache…
smoke-dag-python: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag smoke-dag-python

# Installed-product smoke for the examples/dag TypeScript front doors (Sprint 186): `bashy awd ROOT -- bashy dag -f examples/dag/<repo>/dag.md` against pinned ca…
smoke-dag-typescript: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag smoke-dag-typescript

# Installed-product smoke for the examples/dag Rust front doors (Sprint 194): `bashy awd ROOT -- bashy dag -f examples/dag/<repo>/dag.md` against pinned cache cl…
smoke-dag-rust: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag smoke-dag-rust

# Installed-product smoke for the examples/dag C and C++ front doors (Sprint 190): `bashy awd ROOT -- bashy dag -f examples/dag/<repo>/dag.md` against unchanged…
smoke-dag-c: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag smoke-dag-c

# Installed-product smoke for gh, Hugo, and Caddy Go front doors (Sprint 192).
smoke-dag-go: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag smoke-dag-go

# Installed-product smoke for the examples/dag PowerShell front door (Sprint 358): `bashy awd DIR -- bashy dag -f examples/dag/powershell/dag.md` exercising Bash…
smoke-dag-powershell: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag smoke-dag-powershell

# Installed-product smoke for the examples/dag C# front door (Sprint 358): `bashy awd DIR -- bashy dag -f examples/dag/csharp/dag.md` exercising Bash#-owned cont…
smoke-dag-csharp: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag smoke-dag-csharp

# Installed-product smoke for the Sprint 234 text fences (B30): the Caddy graph's `image` target (a ```bashpp body holding a ~~~dockerfile fence, built with the…
smoke-dag-text: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag smoke-dag-text

# Installed-product smoke for the Sprint 238 manifest fences: examples/manifests/<tool>/build.bsh (cargo, pyproject, gomod, cmake, makefile, package) each run th…
smoke-dag-manifests: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag smoke-dag-manifests

# Installed-product smoke for the rod (Sprint 238): a language or toolchain bashy has never heard of added with a runner and nothing else — examples/runners/{awk…
smoke-runners: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag smoke-runners

# Three-mode process-level example check (Sprint 216, Story 540).
smoke-quickstart: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag smoke-quickstart

# AUTHORITATIVE three-mode FROM-scratch gate (Sprint 216, Story 540).
smoke-quickstart-container: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag smoke-quickstart-container

# The airgap gate (Sprint 227): build the offline bashy image (build-image) and prove every row of docs/airgap-image.md under --network=none --read-only --cap-dr…
smoke-airgap-container: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag smoke-airgap-container

# Run build/unit tests in an agent-owned Ubuntu OCI lane.
test-self-container: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-self-container

# Drift guard for the test-group table in scripts/test-groups.sh: fails if a *_test.go package belongs to no group or a group pattern matches nothing.
test-groups-check: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-groups-check

# Run named test groups as parallel local lanes, e.g.
test-group: FORCE
	@command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag test-group

.DEFAULT: ; @command -v bashy >/dev/null || { echo "requires bashy: https://github.com/qiangli/bashy"; exit 127; }; bashy dag $@

# FORCE names no file and has no recipe: as the one prerequisite it
# keeps every forward running even when a file or directory on disk
# shares a target's name. Dependency order stays in dag, not here.
FORCE:
