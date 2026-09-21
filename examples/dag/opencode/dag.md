---
name: opencode
description: bashy dag front door for OpenCode — install, typecheck, lint, test, and the CLI entry point, as one dependency graph
default: typecheck
---

# OpenCode — task graph

The repo's own commands (`bun install`, `tsgo`, `oxlint`, `bun test`, the
`src/index.ts` entry), wired as a dependency graph so one front door replaces
the `package.json` script list: `bashy dag --list` shows the targets,
`bashy dag test-config` installs, then runs the config suite. Lives at the
repo root; needs `bashy` (github.com/qiangli/bashy) and `bun` on PATH —
OpenCode is a Bun workspace (`packageManager: bun@…`, `bun.lock`).

## Tasks

### install
Install the workspace from the committed `bun.lock` — `--frozen-lockfile` so
a newer `bun` never rewrites it. Runs the workspace's own `postinstall`
(`fix-node-pty`) as `bun install` always does.
Sources: package.json bun.lock
Generates: node_modules/typescript/package.json
Effects: read, write, net
Timeout: 30m

```bash
bun install --frozen-lockfile
```

### typecheck
The `opencode` package's own `typecheck` script (`tsgo --noEmit`, the native
TypeScript preview the workspace pins).
Requires: install
Effects: read

```bash
bun run --cwd packages/opencode typecheck
```

### lint
`oxlint` over the whole workspace (the root `lint` script). Reports the
repo's own warning/error state — not a quick-gate target.
Requires: install
Effects: read

```bash
bun run lint
```

### test
The `opencode` package's full `bun test` suite (its own `test` script: 30 s
per-test timeout, only failures printed). Long-running; see `test-config`
for the fast lane.
Requires: install
Effects: read, write

```bash
bun run --cwd packages/opencode test
```

### test-config
The config suite alone (`test/config`) — a few seconds, no network.
Requires: install
Effects: read, write

```bash
cd packages/opencode && bun test --timeout 30000 test/config
```

### run
Prove the CLI entry point runs from source: `src/index.ts --version` prints
`local` for an unpublished checkout (the root `dev` script's entry).
Requires: install
Effects: read

```bash
bun run --cwd packages/opencode src/index.ts --version
```

### smoke
Call the package directly from a Bash++ body: a `~~~ts` fence declares a
launcher, `ts.launch()` imports `ConfigPaths.fileInDirectory` from the
`opencode` package's source and hands the result back to the shell. No
wrapper script, no `bun -e` quoting — the fence IS the launcher. The fence
is checked by the workspace's own TypeScript compiler (`node_modules/
typescript`) and run on Bun (`BASHPP_TYPESCRIPT_RUNTIME=bun`): OpenCode's
source uses extensionless internal imports, which Bun resolves and Node
does not.
Requires: install
Env: BASHPP_TYPESCRIPT_RUNTIME=bun
Effects: read

```bsh
~~~ts as ts
import { fileInDirectory } from "./packages/opencode/src/config/paths"
export function launch(): string {
  return fileInDirectory(".opencode", "opencode").join("|")
}
~~~
got := ts.launch()
[ "$got" = ".opencode/opencode.json|.opencode/opencode.jsonc" ] || { echo "smoke: fileInDirectory -> '$got'" >&2; exit 1; }
echo "smoke: fileInDirectory -> $got"
```
