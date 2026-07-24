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

The quarantine also covers two public uutils FIFO cases discovered in the
contained run at commit `a7551d77574266075f085d7db9add85e15dec7d6`:

- `test_cp::test_cp_fifo` runs
  `cp --preserve=mode -r fifo fifo2`. Recursive `cp` must recreate the FIFO;
  opening it for input blocks forever because the test has no producer.
- `test_cp::test_dir_perm_race_with_preserve_mode_and_ownership` runs
  `cp --preserve=<mode|ownership> -R --copy-contents --parents src dest`.
  The destination hierarchy must exist before `cp` blocks on `src/fifo`, or
  the producer's directory handshake times out and leaves the child blocked.

These skips have no override. Remove them only after deadline-bounded unit
regressions and a successful contained verification.

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
