# Sprint 436 / Story 1920 — official runner native gate

Build on 0686f21f and preserve the official v0.21.2 source pin and security
admission hold. Implement a hosted-only runtime exercise of the final executable
on Linux amd64/arm64, Darwin amd64/arm64 and Windows amd64. Fetch the small MIT
TinyStories 260K GGUF by immutable revision and exact digest before isolation.
Require launched health, explicit load commit, ready health, streamed tokens and
terminal completion, finite nonzero embeddings, cancellation with single-slot
reuse, process reap and closed listener. Bind evidence to executable and fixture
hashes. Use Linux network namespace plus syscall verdict, Darwin sandbox with
denial controls, and Windows dynamic WFP filters plus a single-process job.
Missing dependencies, evidence, fixture, bind, runtime or enforcement fail.

Run only offline harness/recipe regression gates on Dragon. Do not build or run
the native runner here. The conductor reviews, installs the generated fork and
runs the five hosted lanes; those results, audit, signing and release remain
separate from this worker's local validation. Commit exact worker paths with
required trailers; no push and no edits to audit.yml or the security gate.

## Worker delivery evidence

`bash builds/check.sh ollama` passed: 11 shared egress cases, 19 toolchain
cases, 3 recipe tests (including all five fake compiler targets), 14 HTTP/control/
fixture/evidence harness tests, shell syntax, shellcheck, actionlint, and exact
fork regeneration. `git diff --check` passed. The pinned 1,185,376-byte GGUF was
fetched and SHA-256 verified; metadata read without model execution reports llama,
64 embedding dimensions and five blocks. No native build/inference ran on Dragon.

Hosted status: **unmeasured on all five targets**, including Windows guard
compilation. The conductor must collect each `-native.json` PASS plus its hash-bound
`-egress.txt` and log after installing the reviewed fork. Audit/security admission
remains untouched and blocked. The complete runtime/evidence contract and limits
are in `builds/ollama/README.md`.
