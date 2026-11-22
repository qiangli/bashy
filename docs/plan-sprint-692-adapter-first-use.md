# Sprint 692 adapter first-use fixes

Deliver in order, one commit per story:

1. #3298 / 42f79daac406: suppress agent-mode provisioner functions at adapter
   shell startup; force the Go adapter's own build through the in-process Go
   provisioner. Test the shipped bootstrap with empty caches and bounded shell
   process launches using a tiny dependency-free payload.
2. #3299 / 3dd8b4ec6f29: disable VCS stamping for adapter bootstrap builds;
   test with inherited foreign GIT_DIR and GIT_INDEX_FILE.
3. #3300 / 3d47facb6d19: resolve install-agent's default shell through launcher
   identity and reject adapter-cache targets before writing shims.

Regenerate embedded scripts after adapter changes. Build cmd/bashy and run only
focused unit tests for the changed packages, including embedded/source parity.
Full integration and release gates belong to candidate story 458628c0767e.
The weave worker contract leaves commits on the current branch for conductor
integration (no push or umbrella pin in this worker).
