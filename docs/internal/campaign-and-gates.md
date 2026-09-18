# Campaign identity, regression gates, and the product sequence

Moved out of the README front page on 2026-09-18 (Sprint 212): these are the
conformance-campaign and product-sequence notes that a contributor needs and a
visitor does not. Unchanged text.

### POSIX conformance campaign identity

Bashy's licensed Open Group campaign uses **VSC-PCTS2016 version 3.1** with the
**VSC 5.4.1** framework/documentation, configured in the suite's **POSIX08**
profile (`VSC_POSIX_VERSION=200809`) with the XSI/X/Open profile disabled
(`VSC_XOPEN_VERSION=0`). The formal certification target for that suite is the
**1003.1-2016 Shell and Utilities Product Standard**.

That wording is deliberate. IEEE Std 1003.1-2017 is a later publication in the
same POSIX.1-2008 / Issue 7 lineage, but it is not the Product Standard name for
which VSC-PCTS2016 is authorized. Do not describe this campaign as a
“VSC-PCTS2017” or “1003.1-2017 certification” run.

Running the suite provides conformance evidence; it does **not** by itself make
Bashy POSIX certified. Certification and use of an Open Group mark require a
separate submission and approval. Current runs are diagnostic campaigns used to
find and fix failures before an uninterrupted formal run. See
[`docs/conformance-statement.md`](docs/conformance-statement.md) for claim scope
and [`docs/vsc-pcts-run-status.md`](docs/vsc-pcts-run-status.md) for the
public-safe measured status.

### Shell regression gates

Shell development has two complementary regression gates:

1. `make test-bash-parallel` runs the public GNU Bash 5.3 compatibility corpus
   locally. Its current denominator is 86 fixtures and the required result is
   86/86.
2. On the licensed native VSC host,
   `make test-bash-system ARM=<unique-name>` in the sibling
   `vsc-pcts-harness-kit` runs all 493 POSIX shell TPs against `bin/bash`, with
   Bashy's pure-Go command applets excluded from `PATH`.

The 493-TP arm is relatively inexpensive and is required for release candidates
and changes to parsing, expansion, execution, jobs, signals, traps, builtins,
redirections, or locale-sensitive shell behavior. It cannot run in ordinary
public CI because VSC-PCTS is licensed. After that isolation gate passes,
`make test-bash` in the harness restores the Bashy Go applets and checks shell
integration before the larger command-and-utility campaign.

For concurrent agent work, use the isolated test-lane contract in
[`docs/isolated-test-lanes.md`](docs/isolated-test-lanes.md). It maps
weave/worktree ownership to distinct OCI containers and results, supports
multiple simultaneous instances of the same suite or POSIX profile, documents
the swappable Ubuntu base, and gives status/cleanup commands.

### Product sequence and Bash++ activation

Bashy ships two binaries carrying three named substrates:
**Classic → Bash++ → Yoke**. **Classic** is the compatible standalone shell
(`bash`) together with the pure-Go userland; **Bash++** is the opt-in language
extension and the bridge upward; **Yoke** is the agentic framework. The `bash`
binary is Classic alone; `bashy` carries all three.

| Invocation | Bash++ default | Agentic default |
|---|---:|---:|
| `bash` with `.sh`, `.bash`, or no extension | off | unavailable |
| `bash` with `.bpp`, without an explicit selector | off | unavailable |
| `bashy`, regardless of extension | on | on |

Use `--bashpp` (canonical) or `--bash++` to opt in and `--no-bashpp` to opt
out. `BASHY_BASHPP=1|0` supplies the environment default; `set -o bashpp` and
`set +o bashpp` change the mode for subsequently parsed input. Bashy's agentic
surface is independently controlled by `--agentic` / `--no-agentic` and
`BASHY_AGENTIC=1|0`.

Precedence is **explicit CLI → environment → `.bpp` extension → binary
default**. Because extended grammar must be selected before a file is parsed,
an in-file `set -o bashpp` cannot enable new syntax retroactively in an
already-parsed file. Use a flag, environment setting, Bashy's `.bpp` convention, or
`#!/usr/bin/env -S bash --bashpp` for initial selection.

Startup POSIX mode suppresses Bash++ grammar on both front doors, including
`--bashpp`, its `--bash++` alias, and `BASHY_BASHPP=1`. `bashy --posix`
retains POSIX parsing and runtime semantics while disabling Bash++ defaults
and explicit requests. The standalone `bash --posix --bashpp` combination
retains the Sprint 114 compatibility profile: Bash++ and POSIX differences
are both off, matching the selector-off, POSIX-off invocation. Ordinary
`bash --posix`, including an explicit `--no-bashpp`, retains POSIX mode.
The resolver still records the winning selector tier and whether it was
explicit; that provenance does not mean extended grammar is enabled.

The bare `agentic` modifier permits explicitly implemented LLM assistance in
Bash++ functions, methods and script blocks. See the runnable
[agentic action example](docs/agentic-action-example.md) for the same action as
a typed callable, shell script, executable tool and native embedding. A function
or a `dag` target can carry a [contract](docs/contracts.md) — `@require` /
`@ensure` / `@guard`, `Require:` / `Ensure:` / `Effects:` — judged as shell
checks at the process boundary.
