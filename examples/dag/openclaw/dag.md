---
name: openclaw
description: bashy dag front door for OpenClaw — install, typecheck, format check, tests, and the CLI entry point, as one dependency graph
default: typecheck
vars:
  TEST_FILE ?= src/utils/chunk-items.test.ts
---

# OpenClaw — task graph

The repo's own commands (`pnpm install`, `tsgo`, `oxfmt`, `vitest`, the
`openclaw` wrapper), wired as a dependency graph so one front door replaces
the 400-line `package.json` script list: `bashy dag --list` shows the
targets, `bashy dag typecheck` installs, then typechecks. Lives at the repo
root; needs `bashy` (github.com/qiangli/bashy), `node` (the `engines`
range) and `pnpm` (`packageManager: pnpm@…`, `pnpm-lock.yaml`; `corepack`
provides it) on PATH.

## Tasks

### install
Install the workspace from the committed `pnpm-lock.yaml` —
`--frozen-lockfile`, as CONTRIBUTING.md says — into pnpm's isolated
`node_modules/.pnpm` layout. Every other target depends on this one.
Sources: package.json pnpm-lock.yaml pnpm-workspace.yaml
Generates: node_modules/typescript/package.json
Effects: read, write, net
Timeout: 30m

```bash
pnpm install --frozen-lockfile
```

### typecheck
The core typecheck lane (`tsgo:core`: `tsconfig.core.json`, incremental,
build info under `.artifacts/`).
Requires: install
Effects: read, write

```bash
pnpm tsgo:core
```

### format-check
`oxfmt --check` over the tree (the `format:check` script).
Requires: install
Effects: read

```bash
pnpm format:check
```

### lint
The repo's lint runner (`scripts/run-lint.mts`: oxlint + the custom rules).
Requires: install
Effects: read

```bash
pnpm lint
```

### build
`scripts/build-all.mts` — the full build into `dist/` (tsdown, plugin
assets, runtime stamps). The `openclaw` wrapper rebuilds a stale `dist/`
by itself, so `run` does not require this target.
Requires: install
Generates: dist/entry.js
Effects: read, write

```bash
pnpm build
```

### test
The repo's canonical test entry (`scripts/test-projects.mts`: every vitest
project). Long-running; see `test-unit-fast` and `test-file`.
Requires: install
Effects: read, write

```bash
pnpm test
```

### test-unit-fast
The repo's own fast unit lane (`vitest.unit-fast.config.ts`; ~1,400 files).
Minutes, not seconds; reports the checkout's own state.
Requires: install
Effects: read, write

```bash
pnpm test:unit:fast
```

### test-file
One targeted vitest run, the form AGENTS.md prescribes for scoped work
(`node scripts/run-vitest.mjs run FILE`). `TEST_FILE=path` on the command
line picks the file.
Requires: install
Effects: read, write

```bash
node scripts/run-vitest.mjs run $TEST_FILE
```

### run
Prove the CLI runs from source through its own wrapper (`pnpm openclaw`,
never `node --import tsx src/index.ts`): `--version` prints
`OpenClaw <version> (<commit>)`, building `dist/` first when it is stale.
Requires: install
Effects: read, write

```bash
pnpm openclaw --version
```

### smoke
Call the package directly from a Bash++ body: a `~~~ts` fence declares a
launcher, `ts.launch()` imports two source utilities (`parseBooleanValue`,
`chunkItems`) and hands the result back to the shell. No wrapper script,
no `node -e` quoting — the fence IS the launcher. The fence is checked by
the workspace's own TypeScript compiler (`node_modules/typescript`) and
run on Bun (`BASHPP_TYPESCRIPT_RUNTIME=bun`): OpenClaw's sources import
their `.ts` neighbours by the `.js` output name (NodeNext), which its own
scripts resolve through the `tsx` loader and Bun resolves natively.
Requires: install
Env: BASHPP_TYPESCRIPT_RUNTIME=bun
Effects: read

```bsh
~~~ts as ts
import { parseBooleanValue } from "./src/utils/boolean.js"
import { chunkItems } from "./src/utils/chunk-items.js"
export function launch(): string {
  const rows = chunkItems(["a", "b", "c", "d", "e"], 2)
  return `${parseBooleanValue("yes")}:${parseBooleanValue("maybe")}:${rows.length}`
}
~~~
got := ts.launch()
[ "$got" = "true:undefined:3" ] || { echo "smoke: launch -> '$got'" >&2; exit 1; }
echo "smoke: parseBooleanValue/chunkItems -> $got"
```
