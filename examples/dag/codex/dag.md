---
name: codex
description: bashy dag front door for Codex — fetch, format check, clippy, build, nextest and the CLI binary of the codex-rs workspace, as one dependency graph
default: fmt-check
vars:
  TEST_CRATE ?= codex-arg0
---

# Codex — task graph

The repo's own commands (`cargo fetch`, `cargo fmt`, `cargo clippy --tests`,
`cargo build -p codex-cli`, `cargo nextest run` with the `justfile`'s
environment, the built `codex-rs/target/debug/codex`), wired as a dependency
graph so one front door replaces the `justfile` recipe list: `bashy dag
--list` shows the targets, `bashy dag run` builds the CLI, then launches it.
Lives at the repo root; the Rust workspace is `codex-rs/` (the `justfile`
sets `working-directory := "codex-rs"` for the same reason), so every cargo
target runs there. Needs `bashy` (github.com/qiangli/bashy) and a Rust
toolchain — with `rustup`'s `cargo` proxy on PATH the channel pinned in
`codex-rs/rust-toolchain.toml` is picked automatically; a distro `cargo`
uses whatever it is.

## Tasks

### fetch
Download the dependency graph from the committed `Cargo.lock` (the
`justfile`'s `install` recipe is `cargo fetch`) — `--locked` so a newer
`cargo` never rewrites it. Writes only to the cargo home, never to the
checkout.
Sources: codex-rs/Cargo.toml codex-rs/Cargo.lock
Effects: read, write, net
Timeout: 30m

```bash
cd codex-rs && cargo fetch --locked
```

### fmt-check
`cargo fmt --all --check` over the workspace — the Rust half of `just
fmt-check` (`scripts/format.py --check` also runs the Bazel, Python and
justfile formatters), read-only. The repo's `rustfmt.toml` uses nightly
options; a stable `rustfmt` warns about them and still checks.
Effects: read

```bash
cd codex-rs && cargo fmt --all --check
```

### clippy
The `justfile`'s `clippy` recipe (`cargo clippy --tests`). Builds the whole
workspace's check artifacts — not a quick-gate target; `just fix -p <crate>`
is the repo's scoped form.
Requires: fetch
Effects: read, write

```bash
cd codex-rs && cargo clippy --tests
```

### build
The `codex` binary, debug profile (`cargo build -p codex-cli`, what the
`justfile`'s `codex` recipe runs through `cargo run`). Minutes the first
time, incremental after; `codex-rs/target/` is gitignored.
Requires: fetch
Sources: codex-rs/Cargo.toml codex-rs/Cargo.lock codex-rs/cli codex-rs/core
Generates: codex-rs/target/debug/codex
Effects: read, write
Timeout: 1h

```bash
cd codex-rs && cargo build -p codex-cli
```

### test
One crate's tests exactly as `just test -p <crate>` runs them
(`RUST_MIN_STACK=8388608 NEXTEST_PROFILE=local cargo nextest run
--no-fail-fast`; AGENTS.md: never bare `cargo test`). Needs `cargo-nextest`
installed. `codex-arg0` by default — a small crate; `TEST_CRATE=codex-tui`
on the command line picks another.
Requires: fetch
Effects: read, write

```bash
cd codex-rs && RUST_MIN_STACK=8388608 NEXTEST_PROFILE=local cargo nextest run --no-fail-fast -p "$TEST_CRATE"
```

### smoke
Call into the checkout from a Bash++ body: a `~~~rs` fence declares
`codex()`, `rs.codex()` reads the workspace's own coordinates — the
`workspace.package` version, the channel pinned in
`codex-rs/rust-toolchain.toml`, the number of workspace members and the
number of locked packages — and hands one string back to the shell, which
cross-checks it against the same files. No build, no wrapper script, no
`rustc -o` quoting: the fence IS the program. The fence is compiled by the
`rustc` on PATH (`BASHPP_RUSTC` overrides) and runs as a native worker in
the invoking directory — the repo root, so the paths below say `codex-rs/`
(the fence's own environment discovery looks for `Cargo.toml` beside the
source; here the workspace lives one level down, which the paths spell out).
Effects: read

```bashpp
~~~rs as rs
use std::fs;

pub fn codex() -> Result<String, String> {
    let version = toml_value("codex-rs/Cargo.toml", "version")?;
    let channel = toml_value("codex-rs/rust-toolchain.toml", "channel")?;
    let manifest = fs::read_to_string("codex-rs/Cargo.toml").map_err(|e| format!("codex-rs/Cargo.toml: {e}"))?;
    let members = manifest
        .lines()
        .skip_while(|line| *line != "members = [")
        .skip(1)
        .take_while(|line| *line != "]")
        .filter(|line| line.trim_start().starts_with('"'))
        .count();
    let locked = fs::read_to_string("codex-rs/Cargo.lock")
        .map_err(|e| format!("codex-rs/Cargo.lock: {e}"))?
        .lines()
        .filter(|line| *line == "[[package]]")
        .count();
    Ok(format!("codex {version} toolchain {channel} members {members} locked {locked}"))
}

fn toml_value(path: &str, key: &str) -> Result<String, String> {
    let text = fs::read_to_string(path).map_err(|e| format!("{path}: {e}"))?;
    text.lines()
        .filter_map(|line| line.split_once('='))
        .find(|(k, _)| k.trim() == key)
        .map(|(_, v)| v.trim().trim_matches('"').to_string())
        .ok_or_else(|| format!("{path}: no {key}"))
}
~~~
got := rs.codex()
# The same lookups in shell builtins (no sed/grep: a body should not depend
# on the host's text tools), so the fence's answer is checked, not trusted.
toml_value() { # <file> <key>
	while IFS= read -r line; do
		case "$line" in "$2 = "*) line=${line#*=}; line=${line# }; line=${line#\"}; printf '%s' "${line%\"}"; return 0 ;; esac
	done <"$1"
	return 1
}
version=$(toml_value codex-rs/Cargo.toml version)
channel=$(toml_value codex-rs/rust-toolchain.toml channel)
members=0 in_members=
while IFS= read -r line; do
	case "$in_members$line" in
		'members = [') in_members=1 ;;
		1']') break ;;
		1*) case "${line#"${line%%[! ]*}"}" in '"'*) members=$((members + 1)) ;; esac ;;
	esac
done <codex-rs/Cargo.toml
locked=0
while IFS= read -r line; do [ "$line" = '[[package]]' ] && locked=$((locked + 1)); done <codex-rs/Cargo.lock
want="codex $version toolchain $channel members $members locked $locked"
[ "$got" = "$want" ] || { echo "smoke: rs.codex() -> '$got', want '$want'" >&2; exit 1; }
echo "smoke: $got"
```

### run
Launch the built CLI from a Bash++ body: the `~~~rs` fence's `launch()`
runs `codex-rs/target/debug/codex --version` with `std::process::Command`
and returns its stdout; the shell checks it names `codex-cli` at the
workspace version. The fence is the launcher, the `build` dependency
guarantees the binary exists first.
Requires: build
Effects: read

```bashpp
~~~rs as rs
use std::process::Command;

pub fn launch() -> Result<String, String> {
    let out = Command::new("codex-rs/target/debug/codex")
        .arg("--version")
        .output()
        .map_err(|e| format!("codex-rs/target/debug/codex: {e}"))?;
    if !out.status.success() {
        return Err(String::from_utf8_lossy(&out.stderr).trim().to_string());
    }
    Ok(String::from_utf8_lossy(&out.stdout).trim().to_string())
}
~~~
got := rs.launch()
version=
while IFS= read -r line; do
	case "$line" in 'version = "'*) version=${line#*\"}; version=${version%\"}; break ;; esac
done <codex-rs/Cargo.toml
case "$got" in
	"codex-cli $version"*) ;;
	*) echo "run: rs.launch() -> '$got', want 'codex-cli $version…'" >&2; exit 1 ;;
esac
echo "run: $got"
```
