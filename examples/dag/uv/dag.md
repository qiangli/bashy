---
name: uv
description: bashy dag front door for uv — fetch, format check, clippy, build, test and the CLI binary, as one dependency graph over the Cargo workspace
default: fmt-check
vars:
  TEST_CRATE ?= uv-pep440
---

# uv — task graph

The repo's own commands (`cargo fetch`, `cargo fmt`, `cargo clippy`,
`cargo build -p uv`, `cargo test`, the built `target/debug/uv`), wired as a
dependency graph so one front door replaces the CONTRIBUTING.md incantation
list: `bashy dag --list` shows the targets, `bashy dag run` builds the CLI,
then launches it. Lives at the repo root; needs `bashy`
(github.com/qiangli/bashy) and a Rust toolchain that satisfies the
workspace's `rust-version` — with `rustup`'s `cargo` proxy on PATH the
channel pinned in `rust-toolchain.toml` is picked automatically; a distro
`cargo` uses whatever it is and refuses the build below its MSRV.

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

### fmt-check
`cargo fmt --all --check` — the CONTRIBUTING.md formatting gate, read-only.
Effects: read

```bash
cargo fmt --all --check
```

### clippy
The CONTRIBUTING.md lint line (`--workspace --all-targets --all-features`,
warnings are errors). Builds the whole workspace's check artifacts — not a
quick-gate target.
Requires: fetch
Effects: read, write

```bash
cargo clippy --workspace --all-targets --all-features --locked -- -D warnings
```

### build
The `uv` binary, debug profile (`cargo build -p uv`). Minutes the first
time, incremental after; `target/` is gitignored.
Requires: fetch
Sources: Cargo.toml Cargo.lock crates
Generates: target/debug/uv
Effects: read, write
Timeout: 1h

```bash
cargo build -p uv
```

### test
One crate's tests through `cargo test` (CONTRIBUTING.md recommends
`nextest`, which runs the same tests; `cargo test` needs no extra install).
`uv-pep440` by default — a few seconds; `TEST_CRATE=uv-resolver` on the
command line picks another workspace crate.
Requires: fetch
Effects: read, write

```bash
cargo test -p "$TEST_CRATE"
```

### smoke
Call into the checkout from a Bash++ body: a `~~~rs` fence declares
`uv()`, `rs.uv()` reads the workspace's own coordinates — the `uv` crate's
version, the MSRV (`rust-version`), the channel pinned in
`rust-toolchain.toml` and the number of locked packages — and hands one
string back to the shell, which cross-checks it against the same files. No
build, no wrapper script, no `rustc -o` quoting: the fence IS the program.
The fence is compiled by the `rustc` on PATH (`BASHPP_RUSTC` overrides;
`Cargo.toml`, `Cargo.lock` and `rust-toolchain.toml` are recorded in the
fence's environment fingerprint) and runs as a native worker in the
invoking directory, so the relative paths are the checkout's.
Effects: read

```bsh
~~~rs as rs
use std::fs;

pub fn uv() -> Result<String, String> {
    let version = toml_value("crates/uv/Cargo.toml", "version")?;
    let msrv = toml_value("Cargo.toml", "rust-version")?;
    let channel = toml_value("rust-toolchain.toml", "channel")?;
    let locked = fs::read_to_string("Cargo.lock")
        .map_err(|e| format!("Cargo.lock: {e}"))?
        .lines()
        .filter(|line| *line == "[[package]]")
        .count();
    Ok(format!("uv {version} msrv {msrv} toolchain {channel} locked {locked}"))
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
got := rs.uv()
# The same lookups in shell builtins (no sed/grep: a body should not depend
# on the host's text tools), so the fence's answer is checked, not trusted.
toml_value() { # <file> <key>
	while IFS= read -r line; do
		case "$line" in "$2 = "*) line=${line#*=}; line=${line# }; line=${line#\"}; printf '%s' "${line%\"}"; return 0 ;; esac
	done <"$1"
	return 1
}
version=$(toml_value crates/uv/Cargo.toml version)
msrv=$(toml_value Cargo.toml rust-version)
channel=$(toml_value rust-toolchain.toml channel)
locked=0
while IFS= read -r line; do [ "$line" = '[[package]]' ] && locked=$((locked + 1)); done <Cargo.lock
want="uv $version msrv $msrv toolchain $channel locked $locked"
[ "$got" = "$want" ] || { echo "smoke: rs.uv() -> '$got', want '$want'" >&2; exit 1; }
echo "smoke: $got"
```

### run
Launch the built CLI from a Bash++ body: the `~~~rs` fence's `launch()`
runs `target/debug/uv --version` with `std::process::Command` and returns
its stdout; the shell checks it names the crate version the checkout
declares. The fence is the launcher, the `build` dependency guarantees the
binary exists first.
Requires: build
Effects: read

```bsh
~~~rs as rs
use std::process::Command;

pub fn launch() -> Result<String, String> {
    let out = Command::new("target/debug/uv")
        .arg("--version")
        .output()
        .map_err(|e| format!("target/debug/uv: {e}"))?;
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
done <crates/uv/Cargo.toml
case "$got" in
	"uv $version"*) ;;
	*) echo "run: rs.launch() -> '$got', want 'uv $version…'" >&2; exit 1 ;;
esac
echo "run: $got"
```
