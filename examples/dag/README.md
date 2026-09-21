# `bashy dag` as a project's front door

Fifteen real repositories, each driven by ONE `dag.md` at its root instead of
a `Makefile`, a `justfile`, a `cargo`/`npm run`/`pnpm`/`uv run` alias list, a
`cmake -B …` walkthrough, or a README full of incantations:

| example | repo | stack | targets |
|---|---|---|---|
| [`mini-swe-agent/dag.md`](mini-swe-agent/dag.md) | [SWE-agent/mini-swe-agent](https://github.com/SWE-agent/mini-swe-agent) | Python (uv) | `sync` · `test` · `test-full` · `lint` · `docs` · `run` · `smoke` |
| [`nanochat/dag.md`](nanochat/dag.md) | [karpathy/nanochat](https://github.com/karpathy/nanochat) | Python (uv) | `sync` · `test` · `smoke` · `runcpu` · `chat-cli` |
| [`opencode/dag.md`](opencode/dag.md) | [anomalyco/opencode](https://github.com/anomalyco/opencode) | TypeScript (Bun workspace) | `install` · `typecheck` · `lint` · `test` · `test-config` · `run` · `smoke` |
| [`openclaw/dag.md`](openclaw/dag.md) | [openclaw/openclaw](https://github.com/openclaw/openclaw) | TypeScript (pnpm workspace, Node) | `install` · `typecheck` · `format-check` · `lint` · `build` · `test` · `test-unit-fast` · `test-file` · `run` · `smoke` |
| [`hermes-agent/dag.md`](hermes-agent/dag.md) | [NousResearch/Hermes-Agent](https://github.com/NousResearch/Hermes-Agent) | Python (uv) + TypeScript (npm workspace) | `sync` · `test` · `lint` · `run` · `install-tui` · `build-ink` · `typecheck-tui` · `test-tui` · `smoke` |
| [`uv/dag.md`](uv/dag.md) | [astral-sh/uv](https://github.com/astral-sh/uv) | Rust (Cargo workspace, `rust-toolchain.toml`) | `fetch` · `fmt-check` · `clippy` · `build` · `test` · `smoke` · `run` |
| [`codex/dag.md`](codex/dag.md) | [openai/codex](https://github.com/openai/codex) | Rust (`codex-rs/` Cargo workspace under the repo root, `justfile`) | `fetch` · `fmt-check` · `clippy` · `build` · `test` · `smoke` · `run` |
| [`bun/dag.md`](bun/dag.md) | [oven-sh/bun](https://github.com/oven-sh/bun) | Bun workspace + Rust (nightly-pinned Cargo workspace) | `install` · `lint` · `typecheck` · `fmt-check-rust` · `rust-check` · `smoke` |
| [`mise/dag.md`](mise/dag.md) | [jdx/mise](https://github.com/jdx/mise) | Rust (Cargo workspace, upstream Mise task front doors) | `build` · `test` · `smoke` · `run` |
| [`ffmpeg/dag.md`](ffmpeg/dag.md) | [FFmpeg/FFmpeg](https://github.com/FFmpeg/FFmpeg) | C (`configure` + GNU make, in-tree) | `configure` · `build` · `test` · `smoke` · `run` |
| [`curl/dag.md`](curl/dag.md) | [curl/curl](https://github.com/curl/curl) | C (CMake) | `configure` · `build` · `test` · `smoke` · `run` |
| [`git/dag.md`](git/dag.md) | [git/git](https://github.com/git/git) | C (GNU make, in-tree) | `build` · `test` · `smoke` · `run` |
| [`tesseract/dag.md`](tesseract/dag.md) | [tesseract-ocr/tesseract](https://github.com/tesseract-ocr/tesseract) | C++ (CMake + Leptonica) | `configure` · `build` · `smoke` · `run` |
| [`llama.cpp/dag.md`](llama.cpp/dag.md) | [ggml-org/llama.cpp](https://github.com/ggml-org/llama.cpp) | C/C++ (CMake) | `configure` · `build` · `test` · `smoke` · `run` |
| [`cmake/dag.md`](cmake/dag.md) | [Kitware/CMake](https://github.com/Kitware/CMake) | C++ (`bootstrap` + GNU make, out-of-tree — no cmake needed) | `bootstrap` · `build` · `test` · `smoke` · `run` |

Each target wraps the repo's OWN command (`uv sync`, `pytest`, `ruff`,
`mkdocs`, `bun test`, `tsgo`, `oxlint`, `pnpm tsgo:core`, `vitest`, `npm run
typecheck`, `cargo fmt --check`, `cargo build -p uv`, `cargo nextest run`,
`./configure`, `make`, `cmake -B build`, `ctest`, `runtests.pl`, `bootstrap`,
…) and declares what it depends on (`Requires:`), what it reads
and produces (`Sources:`/`Generates:` — an unchanged `install` is skipped by
content hash, not by mtime), and what it is allowed to do (`Effects:`).
`bashy dag --list` is the help; `bashy dag test` does the right thing in
order; `-j` parallelises; `--json` gives an agent one envelope.

## The `smoke` target: a fence as the launcher

Every file carries one target whose body is Bash# (` ```bsh `) and
declares its launcher IN the project's own language. Python:

````markdown
### smoke
Requires: sync
Env: PYTHONPATH=src

```bsh
~~~py as py
def main() -> str:
    from minisweagent.agents import get_agent_class
    return get_agent_class("default").__name__
~~~
name := py.main()
[ "$name" = DefaultAgent ]
```
````

TypeScript — the same shape, importing the checkout's own source:

````markdown
### smoke
Requires: install
Env: BASHPP_TYPESCRIPT_RUNTIME=bun

```bsh
~~~ts as ts
import { fileInDirectory } from "./packages/opencode/src/config/paths"
export function launch(): string {
  return fileInDirectory(".opencode", "opencode").join("|")
}
~~~
got := ts.launch()
[ "$got" = ".opencode/opencode.json|.opencode/opencode.jsonc" ]
```
````

`~~~py … ~~~` / `~~~ts … ~~~` are Bash++ source fences: the functions they
define become callable from the shell body (`py.main()`, `ts.launch()`), run
by a persistent worker in the project's own environment, discovered from the
working directory — no `activate`, no wrapper script, no `python -c` / `bun
-e` quoting. `~~~py`/`~~~python` and `~~~ts`/`~~~typescript` are the same
languages; `as py` / `as ts` is the alias the body calls through. The
`sync`/`install` dependency guarantees the environment exists first.

For Python the environment is the nearest `.venv`. For TypeScript it is the
nearest `package.json` + lockfile: the manager is read from
`packageManager`/the lock (npm, pnpm, Bun — never invoked), the checker is
the project-local `node_modules/typescript`, and the runtime is Node by
default — its native type stripping runs `.ts` sources as-is — or Bun when the
target says `Env: BASHPP_TYPESCRIPT_RUNTIME=bun`. Pick the runtime the repo's
own source assumes: OpenCode uses extensionless internal imports (Bun-only);
OpenClaw imports `.ts` neighbours by their `.js` output name (NodeNext — its
own scripts go through `tsx`, Bun resolves it natively); Hermes' TUI package
exports `.ts` files directly, which Node loads unaided. A relative import in
the fence may name its `.ts` file explicitly (`./src/x.ts`) — the spelling
Node requires — regardless of the project's `tsconfig.json`.

The Hermes example puts BOTH fences in one body: `py.version()` through the
venv and `ts.compact()` through the npm workspace, one shell body, two
languages, no glue.

Rust — the same shape again, native this time. A `~~~rs` (or `~~~rust`)
fence is a Rust declaration unit: its `pub fn` functions become callables,
compiled by `rustc` into a small worker when the body is prepared (std-only,
no crate dependencies, `BASHPP_RUSTC` overrides the compiler), run in the
invoking directory, `Result<T, E>` errors and panics surfacing as call
failures. Each Rust example carries TWO fence targets. `smoke` needs no
build: `rs.uv()` / `rs.codex()` / `rs.bun()` / `rs.mise()` read the checkout's own
coordinates (crate version, MSRV, the `rust-toolchain.toml` channel, workspace
members, locked packages) and the shell body cross-checks the answer against
the same files with builtins alone. `run` is the launcher proper: after the
repo's own `cargo build -p …`, `rs.launch()` executes the built binary with
`std::process::Command` and hands its `--version` line back to the shell:

````markdown
### run
Requires: build

```bsh
~~~rs as rs
use std::process::Command;

pub fn launch() -> Result<String, String> {
    let out = Command::new("target/debug/uv").arg("--version").output()
        .map_err(|e| format!("target/debug/uv: {e}"))?;
    if !out.status.success() {
        return Err(String::from_utf8_lossy(&out.stderr).trim().to_string());
    }
    Ok(String::from_utf8_lossy(&out.stdout).trim().to_string())
}
~~~
got := rs.launch()
case "$got" in "uv $version"*) ;; *) exit 1 ;; esac
```
````

The fence compiles with whatever `rustc` PATH resolves to — it is std-only,
so the workspace's own pin does not constrain it — while the repo's `cargo`
targets need the toolchain the workspace pins (`rust-version` / `rust-
toolchain.toml`): with rustup's proxies on PATH that happens by itself; a
distro `cargo` below the MSRV refuses the build, which is the repo's rule,
not bashy's. Codex keeps its workspace under `codex-rs/`, so its cargo
targets `cd codex-rs && …` and its fence names `codex-rs/…` paths; Bun's
Rust workspace only resolves after the native build has vendored
`vendor/lolhtml`, so its `rust-check` is a documented target and the gate
runs the Bun-side lanes plus the fence. The four checked-in Rust examples
are the launch shape for a Cargo repo: `bashy dag run` = fetch → build →
launch, one file, no wrapper script.

C and C++ — native again, the compiler's this time. A `~~~c` fence (C17)
or `~~~cxx` / `~~~cpp` fence (C++20) is a declaration unit: its
non-`static` top-level functions become callables, compiled once by the
`cc`/`c++` (`clang`/`clang++`) on PATH into a small worker (`BASHPP_CC` /
`BASHPP_CXX` override), run as a fresh native process per call in the
invoking directory. The checkout root is an include root for the fence's
QUOTED includes — `#include "include/curl/curlver.h"`,
`#include "ggml/include/ggml.h"` — so a self-contained project header is
read at compile time, while the compiler's own `<…>` search is left alone
(a project file named `VERSION` must not shadow C++20's `<version>`, and
does not). No build flags are inferred and nothing is linked, so a fence
that needs a generated header (FFmpeg's `libavutil/ffversion.h`) sits in a
target that `Requires: build`. Each C/C++ example carries the same two
fence targets as the Rust ones. `smoke` needs no build: `c.ffmpeg()` /
`c.curl()` / `c.git()` and `cxx.tesseract()` / `cxx.llama()` /
`cxx.cmake()` read the checkout's own coordinates (`RELEASE`, `curlver.h`'s
macros, `GIT-VERSION-GEN`, `VERSION`, `LLAMA_VERSION_*`,
`CMakeVersion.cmake`) and the shell body cross-checks the answer with
builtins alone. `run` is the launcher proper: after the repo's own
`configure`/`make`, `cmake --build`, or `bootstrap`, `launch()` runs the
built binary through `popen` and hands its `--version` line back — a
C++ launcher throws on failure and the exception is the call's error:

````markdown
### run
Requires: build

```bsh
~~~c as c
#include <stdio.h>
#include <string.h>

const char* launch(void) {
	static char line[512];
	FILE* p = popen("build/src/curl --version", "r");
	if (!p) return "popen: build/src/curl";
	if (!fgets(line, sizeof line, p)) line[0] = 0;
	if (pclose(p) != 0) return "build/src/curl --version: non-zero exit";
	line[strcspn(line, "\n")] = 0;
	return line;
}
~~~
got := c.launch()
case "$got" in "curl $version "*) ;; *) exit 1 ;; esac
```
````

Two things a dag body sees differently from a terminal, both recorded in
the examples that hit them. `make` inside a body is bashy's in-process
POSIX make — a GNU `Makefile` (git, FFmpeg, CMake's generated one) needs
`env make …`, the documented spawn-through that runs the `make` on PATH.
And the body sees PATH only: bashy's front-door shims (`bashy cmake`, the
self-provisioning CMake) are not applied inside it, so `cmake` must be on
PATH — the gate fronts it with the provisioned tree's `bin/` when the host
has none. Two repo-side traps the launchers caught: curl's CMake requires
libpsl unless told otherwise (`CMAKE_OPTS=-DCURL_USE_LIBPSL=OFF`, curl's
own switch), and tesseract adds Leptonica's include directory before its
generated `build/include`, so on a host with a packaged tesseract installed
the build compiles THAT package's `version.h` and the binary reports the
wrong version — `CMAKE_INCLUDE_DIRECTORIES_BEFORE=ON` (in the example's
default `CMAKE_OPTS`) puts the checkout's header first. The six checked-in
C/C++ examples are the launch shape for a native repo: `bashy dag run` =
configure → build → launch, one file, no wrapper script.

## The `smoke` target under a contract (gh)

[`gh/dag.md`](gh/dag.md)'s `smoke` is also the Sprint 216 (Story 541)
acceptance fixture for agentic work in a dag body: the `~~~go` island call
(`go.Gh()`, the checkout's own `internal/build`) sits inside ONE `agentic
function` under `@require`/`@ensure`/`@guard`, and the body is the harness —
it drives the function through every exit status the contract can produce
and asserts each one, so the target's own exit is the verdict:

| call | status | why |
|---|---|---|
| `gh_version ""` | 3 | `@require('test -n "$1"')` refused it; the body never ran |
| `gh_version leak` | 126 | `@guard(effects: "read")` denied the `touch` before it ran |
| `gh_version ask` | 6 | *input required* — no `GH_SMOKE_LABEL`; `@ensure` is not run |
| `GH_SMOKE_LABEL=cli/cli gh_version ask` | 0 | the resume: the answer supplied explicitly; `@ensure` sees the result |

Each call leaves one receipt in the existing skills/craft ledger
(`docs/function-attestation.md`); `bashy craft history gh_version --all`
reads them back as `FAIL`, `FAIL`, `yield`, `pass`. Offline, deterministic,
no model call, no file left behind — `make smoke-dag-go` asserts the four
lines, the four receipts and the read side. It works because a ` ```bsh `
body runs on the same agentic runner as `bashy --bashsharp` (see
`docs/dag.md` §Bash++ bodies).

## Running them against a checkout

The files are written to live at each repo's root (copy one there and run
`bashy dag`). To drive a checkout WITHOUT copying — the way the gates do —
use `awd`, bashy's "run one command over there":

```bash
bashy awd ~/src/nanochat -- bashy dag -f "$PWD/nanochat/dag.md" sync test smoke
bashy awd ~/src/opencode -- bashy dag -f "$PWD/opencode/dag.md" install typecheck smoke
```

`bashy dag` runs bodies in the invoking working directory, like `make`; `-f`
only picks the file. That is why there is no `-C DIR` flag on `dag` (or on any
other verb): `awd DIR -- CMD` is the one directory mechanism, and it works for
every command.

Four installed-product gates run the graphs this way against unchanged
checkouts and assert the JSON envelope, the fence results and a
byte-identical `git status` before and after:

- `make smoke-dag-python` (`scripts/dag-python-examples-smoke.sh`) —
  `MINISWEAGENT_ROOT`, `NANOCHAT_ROOT`.
- `make smoke-dag-typescript` (`scripts/dag-typescript-examples-smoke.sh`)
  — `OPENCODE_ROOT`, `OPENCLAW_ROOT`, `HERMESAGENT_ROOT`; needs `bun`
  (`BASHPP_BUN` if off PATH) and `pnpm` (or `corepack`). The heavy
  repo-wide lanes (`lint`, `test`, `test-unit-fast`) are documented targets,
  not gate targets: they report each checkout's own state, which is not
  bashy's to assert.
- `make smoke-dag-rust` (`scripts/dag-rust-examples-smoke.sh`) —
  `CODEX_ROOT`, `UV_ROOT`, `BUN_ROOT`, `MISE_ROOT`; needs `rustc`, `cargo` with
  `rustfmt`, `bun`, and (for the cache-owned Mise build/test lane) `mise`. It builds the uv and Codex CLIs (minutes cold,
  seconds warm) so the `run` launchers are real; when the PATH `cargo` is
  not a rustup proxy and sits below a workspace's MSRV, `RUST_TOOLCHAIN_BIN`
  names a toolchain `bin/` to front PATH with. Mise is pinned at
  `55d3b4fc789d76fbaa486cb523f92cc974ce67c7`; its cache-owned clone runs
  `mise run build` and `mise run test:unit` with its mbx wrapper enabled. An
  explicit `MISE_ROOT` is never built or tested: the gate runs only the std-only
  Rust fence with `env -i`, then checks its git status byte-for-byte. If mbx
  fails, `MBX_DISABLE=1` is the upstream diagnostic fallback that must be
  surfaced, not a gate default. `clippy`, Codex `test`
  (cargo-nextest), Bun `typecheck` and `rust-check` are documented targets,
  not gate targets.
- `make smoke-dag-c` (`scripts/dag-c-examples-smoke.sh`) — `FFMPEG_ROOT`,
  `CURL_ROOT`, `GIT_ROOT`, `TESSERACT_ROOT`, `LLAMACPP_ROOT`, `CMAKE_ROOT`;
  needs `cc`/`c++`, GNU `make`, `git`, `perl`, `pkg-config` with Leptonica,
  and a `cmake` (`CMAKE_BIN` fronts one; with none on PATH the gate uses
  `bashy cmake`'s provisioned tree). Every graph builds its binary (minutes
  cold, seconds warm; all products gitignored) so the six `run` launchers
  are real, and runs one light test per repo (curl `runtests.pl 1 2 3`, git
  `t0000-basic.sh`, FFmpeg `fate-source`, llama.cpp `test-arg-parser`,
  CMake `CMakeLib.testArgumentParser`). The full suites are documented
  targets, not gate targets.
