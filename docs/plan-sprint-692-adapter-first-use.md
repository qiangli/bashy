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

## Delivery evidence and remaining automatic-launch path

The bounded bootstrap tests complete in four shell process launches. The Git-hook
fixture reproduces `error obtaining VCS status: exit status 128` before disabling
VCS stamping, and succeeds afterward. Targeted bootstrap, embedded-parity and
install-agent shim tests pass; cmd/bashy builds.

The install-agent fix resolves the launcher via bashySelfPath and rejects explicit,
implicit, and symlinked adapter-cache targets. However, inspection of the pinned
sibling yoke module (be872cc5a395) found a separate automatic path:
`pkg/chat/chat.go` in the forceAgentShell branch calls `os.Executable()` followed
by `ensureShims`. A foreman adapter launching an agent reaches that path without
calling install-agent. The dependency needs its own launcher-identity fix and
cache-target guard before the original foreman symptom can be declared closed.
Conductor 692 was notified; this worker does not push sibling code or move pins.
