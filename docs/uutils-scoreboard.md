# Contained uutils Scoreboard

`make test-uutils` is the only supported uutils-suite entry point. It resolves
Docker first, then `bashy podman`, and never executes cargo or upstream tests on
the host. `UUTILS_OCI=/path/to/runtime` can select another Docker-compatible
CLI.

The runner creates a `git archive` of the tracked uutils revision, builds or
accepts the pure-Go `SUT`, and exposes both to the container read-only. It does
not mount the repository, host root, home directory, Cargo cache, SSH material,
or cloud credentials. The container runs as a non-root UID with:

- `--network=none`, a read-only root filesystem, all capabilities dropped, and
  `no-new-privileges`;
- hard memory and PID limits (defaults: `3g` and `512`);
- tmpfs-only build space and a host-enforced wall timeout (default: one hour);
- a permanent quarantine for infinite-device and root-recursion landmines.

Contained attempt 6 identified one exact output-FIFO landmine:
`test_dd::test_seek_output_fifo`. The SUT runs
`dd count=0 seek=1 of=fifo status=noxfer`, while the test writes one 512-byte
block to the same FIFO. Both sides currently open write-only, so neither
provides a reader and both block forever. Only this exact case is quarantined
while nonseekable-output seek semantics are implemented and verified; other
`dd` tests remain measured.

Two public uutils FIFO cases discovered in the contained run at commit
`a7551d77574266075f085d7db9add85e15dec7d6` are now resolved:

- `test_cp::test_cp_fifo` runs
  `cp --preserve=mode -r fifo fifo2`. Recursive `cp` must recreate the FIFO;
  opening it for input blocks forever because the test has no producer.
- `test_cp::test_dir_perm_race_with_preserve_mode_and_ownership` runs
  `cp --preserve=<mode|ownership> -R --copy-contents --parents src dest`.
  The destination hierarchy must exist before `cp` blocks on `src/fifo`, or
  the producer's directory handshake times out and leaves the child blocked.

Coreutils commit `40eb4b634b5530cbabe2075da43bb0194fd588de` adds
deadline-bounded regressions. Its Linux ARM64 SUT
(`sha256:7460116a9c73407ab06e2c830f14b9245b26c8d478c83c33d16c6bf277db638e`)
passed each exact upstream case separately in the isolated `bashy-cert` VM:
one passed, zero failed, no timeout. Their two skips are therefore retired.
They must still never be run directly on a steward host.

Contained attempt 4 later stalled with an idle harness and defunct SUT
children. Process arguments suggested `test_cat::test_fifo_symlink`, whose
producer can block indefinitely if no FIFO reader arrives, so the exact case
was quarantined pending proof. The audit found that coreutils already uses
`os.Open`, follows the symlink, and rendezvous correctly. Coreutils
`0e421f3b60b21bd7cca4d325b2f3497c69ba78f2` adds a bounded 128 KiB
subprocess regression with producer deadline and kill/wait cleanup. Its Linux
ARM64 SUT
(`sha256:e57d48e56ada1e5641c8fd76c6c798cb0735774739aa137af7fbe2e028301093`)
passed the exact public case in `bashy-cert`: one passed, zero failed, 5,365
filtered, 0.03 seconds, no timeout. The process snapshot had not identified
the stalled test; the temporary cat skip is retired.

Contained attempt 5 then identified
`test_dd::test_random_73k_test_lazy_fullblock`: the former SUT rejected
`iflag=fullblock` before opening the FIFO, while the test opened its writer
without a deadline. Coreutils `c12313da0da7407974176e70618a08ceb2f16397`
now accumulates short reads to `ibs` or EOF and adds a bounded 73,728-byte FIFO
regression with nonblocking writer deadlines and kill/wait cleanup. Its Linux
ARM64 SUT
(`sha256:e7b720a7d30937c6118cf96dca9587331cac1193fbce7e41b56da4619e33e041`)
passed the exact public case in `bashy-cert`: one passed, zero failed, 5,365
filtered, 3.88 seconds, no timeout. The temporary dd skip is retired.

Prepare the dependency-only image once with `make prepare-uutils-image`. This
networked preparation step archives the selected uutils revision and runs only
`cargo fetch --locked` during an OCI image build; it never builds or executes
the foreign tests. The scored run uses
`localhost/bashy-uutils-cert:local`, never pulls implicitly, copies the baked
cache into its tmpfs `CARGO_HOME`, and runs Cargo offline. Set
`UUTILS_OCI_IMAGE` for a different pre-provisioned image. For stronger
reproducibility, use an image digest.

Result publication is fail closed. The parser requires exactly one terminal
Cargo summary, a preflight `cargo test -- --list` denominator, exactly one exit
marker after it, `observed + filtered == listed`, and an exit status consistent
with the summary. Ordinary test failures produce a valid informational
scoreboard; timeout, OOM, signal, build abort, truncation, or inconsistent logs
do not.

Configuration examples:

```sh
make prepare-uutils-image
UUTILS_MEMORY=4g UUTILS_PIDS=768 UUTILS_TIMEOUT=5400 make test-uutils
UUTILS_OCI_IMAGE=localhost/bashy-uutils@sha256:... make test-uutils
make test-uutils-safety  # synthetic logs and stub OCI only
```

The tmpfs build-space ceiling is currently fixed at 8 GiB. OCI implementations
may also impose VM-level limits; those should be at least as strict as the
per-container settings.
