# Hermetic Bash 5.3 gate

Goal: make the complete 86-fixture Bash 5.3 verdict independent of host `/tmp`,
TTY, locale, filesystem permissions, and previously built fixture helpers.
Ubuntu 24.04 is the common base for this gate and future self-contained POSIX
Profiles A/B/C/D; profile images vary the shell/provider payload, not the OS.

- [x] Reuse `tools/bash53suite`; do not introduce a second runner.
- [x] Reuse the SHA-256-pinned fixture bootstrap and bake its output into the
  image with the Linux testee and runner.
- [x] Run all fixtures serially, not a default chunk.
- [x] Run non-root with no network, a read-only image, fresh tmpfs mounts, and
  a PTY.
- [x] Expose the lane through both `make test-bash-container` and
  `bashy dag test-bash-container`.
- [x] Build the image and obtain an 86/86, zero-skip result.
- [ ] Run Go/unit/static gates, commit and push Bashy, bump and push the
  umbrella pin, then rebuild/install/smoke the installed Bashy binary.

The native targets remain useful host-integration diagnostics. They are not a
release verdict when the host cannot supply the expected fixture environment.
