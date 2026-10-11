---
name: zed
description: bashy dag front door for Zed — fetch, focused check/clippy, build, test and the editor CLI binary, as one dependency graph over the Cargo workspace
type: dag
default: check
vars:
  CHECK_CRATE ?= sum_tree
  TEST_CRATE ?= sum_tree
---

# Zed — task graph

The repo's own commands (`cargo fetch`, `cargo check -p ...`, `./script/clippy -p ...`,
`cargo build -p zed`, `cargo test -p ...`, the built `target/debug/zed`), wired as a
dependency graph so one front door replaces the development documentation incantations
(`docs/src/development/{macos,linux}.md`): `bashy dag --list` shows the targets,
`bashy dag run` builds the `zed` binary, then launches it. Lives at the upstream checkout
root of [zed-industries/zed](https://github.com/zed-industries/zed).

Needs `bashy` (github.com/qiangli/bashy) and a Rust toolchain satisfying `rust-toolchain.toml`
(channel 1.98.1; with `rustup`'s `cargo` proxy on PATH the pinned channel is selected automatically).
Platform prerequisites per Zed's development docs:
- macOS (`docs/src/development/macos.md`): Xcode with Command Line Tools (`xcode-select --install`)
  and `cmake` (`brew install cmake`).
- Linux (`docs/src/development/linux.md`): system libraries installed via Zed's committed
  script `script/linux`.

## Tasks

### fetch
Download the dependency graph from the committed `Cargo.lock` — `--locked`
so a newer `cargo` never rewrites it. Writes only to the cargo home, never
to the checkout.
Sources: Cargo.toml Cargo.lock
Effects: read, write, net
Timeout: 30m

```bash
cargo fetch --locked
```

### check
`cargo check -p <crate>` on a focused crate (`sum_tree` by default; set
`CHECK_CRATE=zed` on the command line to check the editor crate).
Requires: fetch
Effects: read, write

```bash
cargo check -p "$CHECK_CRATE"
```

### clippy
Run Zed's committed clippy script on a focused crate (`./script/clippy -p <crate>`).
`sum_tree` by default; `CHECK_CRATE=zed` checks the editor crate.
Requires: fetch
Effects: read, write

```bash
./script/clippy -p "$CHECK_CRATE"
```

### build
The `zed` binary, debug profile (`cargo build -p zed`). Minutes the first
time, incremental after; `target/` is gitignored.
Requires: fetch
Sources: Cargo.toml Cargo.lock crates
Generates: target/debug/zed
Effects: read, write
Timeout: 1h

```bash
cargo build -p zed
```

### test
One crate's unit tests through `cargo test -p <small crate>`.
`sum_tree` by default — a small data structure crate with focused unit tests;
`TEST_CRATE=collections` on the command line picks another focused workspace crate.
Requires: fetch
Effects: read, write

```bash
cargo test -p "$TEST_CRATE"
```

### smoke
Call into the checkout from a Bash++ body: a `~~~rs` fence declares `zed()`,
`rs.zed()` reads the workspace's own coordinates — the `zed` crate's version
(`crates/zed/Cargo.toml`), the channel pinned in `rust-toolchain.toml`, the number
of workspace members in `Cargo.toml`, and the number of locked packages in
`Cargo.lock` — and hands one string back to the shell, which cross-checks it against
the same files with shell builtins. No project build, no wrapper script, no
`rustc -o` quoting: the fence IS the program. The fence is compiled by the `rustc`
on PATH (`BASHPP_RUSTC` overrides; `Cargo.toml`, `Cargo.lock`, `crates/zed/Cargo.toml`
and `rust-toolchain.toml` are recorded in the fence's environment fingerprint) and
runs as a native worker in the invoking directory, so the relative paths are the
checkout's.
Effects: read

```bsh
~~~rs as rs
use std::fs;

pub fn zed() -> Result<String, String> {
    let version = toml_value("crates/zed/Cargo.toml", "version")?;
    let channel = toml_value("rust-toolchain.toml", "channel")?;
    let manifest = fs::read_to_string("Cargo.toml").map_err(|e| format!("Cargo.toml: {e}"))?;
    let members = manifest
        .lines()
        .skip_while(|line| *line != "members = [")
        .skip(1)
        .take_while(|line| line.trim() != "]")
        .filter(|line| line.trim_start().starts_with('"'))
        .count();
    let locked = fs::read_to_string("Cargo.lock")
        .map_err(|e| format!("Cargo.lock: {e}"))?
        .lines()
        .filter(|line| *line == "[[package]]")
        .count();
    Ok(format!("zed {version} toolchain {channel} members {members} locked {locked}"))
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
got := rs.zed()
toml_value() { # <file> <key>
	while IFS= read -r line; do
		case "$line" in "$2 = "*) line=${line#*=}; line=${line# }; line=${line#\"}; printf '%s' "${line%\"}"; return 0 ;; esac
	done <"$1"
	return 1
}
version=$(toml_value crates/zed/Cargo.toml version)
channel=$(toml_value rust-toolchain.toml channel)
members=0 in_members=
while IFS= read -r line; do
	case "$in_members$line" in
		'members = [') in_members=1 ;;
		1']') break ;;
		1*) case "${line#"${line%%[! ]*}"}" in '"'*) members=$((members + 1)) ;; esac ;;
	esac
done <Cargo.toml
locked=0
while IFS= read -r line; do [ "$line" = '[[package]]' ] && locked=$((locked + 1)); done <Cargo.lock
want="zed $version toolchain $channel members $members locked $locked"
[ "$got" = "$want" ] || { echo "smoke: rs.zed() -> '$got', want '$want'" >&2; exit 1; }
echo "smoke: $got"
```

### run
Launch the built binary from a Bash++ body: the `~~~rs` fence's `launch()`
runs `target/debug/zed --system-specs` with `std::process::Command` and returns
its stdout; the shell checks it names the crate version the checkout declares.
`--system-specs` runs headlessly and statelessly without opening a window or connecting
to a display server. The fence is the launcher; the `build` dependency guarantees
the binary exists first.
Requires: build
Effects: read

```bsh
~~~rs as rs
use std::process::Command;

pub fn launch() -> Result<String, String> {
    let out = Command::new("target/debug/zed")
        .arg("--system-specs")
        .output()
        .map_err(|e| format!("target/debug/zed: {e}"))?;
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
done <crates/zed/Cargo.toml
[ -n "$version" ] || { echo "run: crates/zed/Cargo.toml has no version" >&2; exit 1; }
case "$got" in
	*"Zed: v$version"*) ;;
	*) echo "run: rs.launch() -> '$got', want containing 'Zed: v$version'" >&2; exit 1 ;;
esac
echo "run: zed v$version"
```
