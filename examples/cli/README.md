# YAML CLI adapters

Sprint 132 story #31 demonstrates ycode's own CLI contract through a thin
Bash++ adapter. The YAML declares commands, flags, help, validation and dispatch.
The supplied ycode binary compiles that document and executes its operations.
The adapter only resolves its local files and forwards arguments and streams.

This POC supports the native ycode surface. P1 foreign CLI profiles remain
unopened; no Codex, OpenCode or other CLI compatibility is claimed.

`ycode/profile.yaml` is a full snapshot of the canonical
`ycode/examples/agent.yaml`, including the runtime graph. Its source commit is
ycode `1db28283e25d38e7ed46314823041ed9b3e03fd9`.
Edit the canonical document first, then refresh this snapshot.

From the dhnt umbrella, after building the candidate ycode binary:

```sh
YCODE_BIN="$PWD/ycode/bin/ycode" bashy --bashpp bashy/examples/cli/ycode/main.bpp --help
YCODE_BIN="$PWD/ycode/bin/ycode" bashy --bashpp bashy/examples/cli/ycode/main.bpp validate
YCODE_BIN="$PWD/ycode/bin/ycode" bashy --bashpp bashy/examples/cli/ycode/main.bpp config get spec.runtime.defaultAgentRef
```

Set `YCODE_BIN` to an absolute executable path. Without it, the adapter resolves
the sibling checkout's `ycode/bin/ycode`. It supplies its adjacent
`profile.yaml` as the first `--file` value; a later explicit `--file` or `-f`
selects another YAML document using ycode's normal last-value precedence.
Arguments following `--` retain their normal meaning. Exit status, stdout and
stderr come from the ycode process.

`main.bpp` uses a typed Bash++ function and a fenced Go helper for path
resolution, following the native patterns in `examples/agentic/typed.bpp` and
`examples/dag/gh/dag.md`. The helper does not implement command parsing,
providers, policy or an agent loop. The shell keeps the invoking working
directory. Before running an actual prompt, configure the YAML runtime roots,
provider and credentials for that workspace.

The offline comparison runner checks help, version, validation, read-only
inspection and failures against direct invocation of the same binary and YAML:

```sh
YCODE_BIN="$PWD/ycode/bin/ycode" BASHY_BIN="$(command -v bashy)" \
  /bin/sh bashy/examples/cli/fixtures/run.sh
```

The runner uses a temporary working directory and makes no model requests.
It compares stdout, stderr and process status without normalizing away errors.
