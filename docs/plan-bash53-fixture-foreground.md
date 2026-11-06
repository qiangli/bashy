# Guarded fixture foreground terminal handoff

The Bash 5.3 runner starts each Unix fixture through procguard in a separate
process group. Headless Makefile runs have a controlling PTY, but that group
starts in the background. GNU `read2.sub` opens `/dev/tty` and stops on a
background terminal read, invalidating the native baseline.

Keep the guarded group and the idle stdin pipe. Before releasing procguard's
command specification, give its group the controlling terminal foreground.
After completion, timeout, or cancellation, kill and reap the group, then
restore the original foreground group. Refuse to take a terminal already owned
by another group; without a controlling terminal, retain the old behavior.
The fixture corpus, expected output, and deadlines stay unchanged.

Verification: focused PTY tests cover read, ownership, restoration, timeout,
cancellation, containment, and no-TTY operation. The parent runs both the
product and unchanged GNU `read` fixture on the remote host.

The parent runs these controls in its isolated Linux checkout, after binding
`BASH53_GNU_BASH` to the pinned GNU Bash provider and recording its resolved
path, version, and SHA-256. The source checkout must contain this delivery
commit and the original fixture tree. Keep the existing per-fixture deadlines.

```sh
sha256sum external/bash-5.3/tests/read.tests external/bash-5.3/tests/read.right \
  external/bash-5.3/tests/read2.sub external/bash-5.3/tests/read6.sub
go test ./tools/bash53suite -run '^TestFixtureTerminalControl$' -count=1 -v
make test-bash TESTS=read
script -qefc 'bin/bash53suite -tests-dir external/bash-5.3/tests -bash "$BASH53_GNU_BASH" -tests read' /dev/null
```

Expected original fixture SHA-256 values, in the order above: `c89f3dd8d0e21d5d0789598bad8edf60845b8a98ac6bd9d99959f19f633d15d8`,
`b391768ecc4cc7e45d1664a44de38246a973a39f70243a6d0515ba1225704e47`,
`d6df59fe921a1d63d23b62f36a36c57869843a26345ffc05c33e19e63ff2b52c`,
and `f66bd7bb6026c9f02f8c6816fa356e36910f27b7ed10a2ce6d27d864292df198`.
The first `read` run measures the product; the second measures the unchanged
GNU Bash provider against the same original fixture and expected output.
