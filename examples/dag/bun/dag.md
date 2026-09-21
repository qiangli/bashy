---
name: bun
description: bashy dag front door for Bun's source tree — install, lint, typecheck, the Rust workspace's format check and cargo check, as one dependency graph
default: lint
---

# Bun — task graph

The repo's own commands (`bun install`, `bun run lint`, `bun run typecheck`,
`cargo fmt --all` from the `fmt:rust` script, `bun run rust:check`), wired
as a dependency graph so one front door replaces the `package.json` script
list: `bashy dag --list` shows the targets, `bashy dag lint` installs, then
lints. Lives at the repo root, which is both the Bun workspace
(`package.json`, `bun.lock`) and the Rust workspace (`Cargo.toml`, ~100
`src/*` crates, `rust-toolchain.toml` pinning a nightly). Needs `bashy`
(github.com/qiangli/bashy) and a release `bun` on PATH — CONTRIBUTING.md
requires one to build Bun itself, and the scripts below run under it.
Building Bun (`bun bd`: cmake, Zig, LLVM, a vendored WebKit) is the
CONTRIBUTING.md path, not a target here.

## Tasks

### install
Install the root workspace from the committed `bun.lock` —
`--frozen-lockfile` so a newer `bun` never rewrites it (the dev tooling:
`oxlint`, `prettier`, `typescript`).
Sources: package.json bun.lock
Generates: node_modules/oxlint/package.json
Effects: read, write, net
Timeout: 30m

```bash
bun install --frozen-lockfile
```

### lint
The root `lint` script (`oxlint --config=oxlint.json src/js`) — the
built-in JavaScript modules, a second.
Requires: install
Effects: read

```bash
bun run lint
```

### typecheck
The root `typecheck` script: `tsc --noEmit` over the tree, then the
`test/` package's own typecheck. Reports the checkout's own state (the
`test/` half type-checks against a built Bun's generated types) — not a
quick-gate target.
Requires: install
Effects: read

```bash
bun run typecheck
```

### fmt-check-rust
`cargo fmt --all --check` — the check form of the `fmt:rust` script
(`cargo fmt --all`), read-only over every workspace crate. Needs only a
`rustfmt`; the workspace's nightly pin is for compiling, not formatting.
Effects: read

```bash
cargo fmt --all --check
```

### rust-check
The `rust:check` script (`cargo check --workspace --keep-going`). Needs the
toolchain `rust-toolchain.toml` pins (a nightly with `rust-src`) AND the
build's vendored path dependency `vendor/lolhtml` — `scripts/build.ts`
fetches it before cargo runs, so a fresh checkout's `cargo check` stops at
the missing path dependency until `bun bd` has been run once. Documented
here as the repo's own Rust lane; not a quick-gate target.
Effects: read, write

```bash
bun run rust:check
```

### smoke
Call into the checkout from a Bash++ body: a `~~~rs` fence declares
`bun()`, `rs.bun()` reads the tree's own coordinates — the `package.json`
version, the last release in `LATEST`, the channel pinned in
`rust-toolchain.toml`, the number of workspace member crates and the
number of locked packages — and hands one string back to the shell, which
cross-checks it against the same files. No build, no wrapper script, no
`rustc -o` quoting: the fence IS the program. The fence is std-only, so it
compiles with the `rustc` on PATH (`BASHPP_RUSTC` overrides) whatever
channel the workspace itself pins — `Cargo.toml`, `Cargo.lock` and
`rust-toolchain.toml` are recorded in its environment fingerprint — and
runs as a native worker in the invoking directory.
Effects: read

```bsh
~~~rs as rs
use std::fs;

pub fn bun() -> Result<String, String> {
    let version = json_string("package.json", "version")?;
    let latest = fs::read_to_string("LATEST").map_err(|e| format!("LATEST: {e}"))?.trim().to_string();
    let channel = toml_value("rust-toolchain.toml", "channel")?;
    let manifest = fs::read_to_string("Cargo.toml").map_err(|e| format!("Cargo.toml: {e}"))?;
    let members = manifest
        .lines()
        .skip_while(|line| *line != "members = [")
        .skip(1)
        .take_while(|line| *line != "]")
        .filter(|line| line.trim_start().starts_with('"'))
        .count();
    let locked = fs::read_to_string("Cargo.lock")
        .map_err(|e| format!("Cargo.lock: {e}"))?
        .lines()
        .filter(|line| *line == "[[package]]")
        .count();
    Ok(format!("bun {version} latest {latest} toolchain {channel} members {members} locked {locked}"))
}

fn toml_value(path: &str, key: &str) -> Result<String, String> {
    let text = fs::read_to_string(path).map_err(|e| format!("{path}: {e}"))?;
    text.lines()
        .filter_map(|line| line.split_once('='))
        .find(|(k, _)| k.trim() == key)
        .map(|(_, v)| v.trim().trim_matches('"').to_string())
        .ok_or_else(|| format!("{path}: no {key}"))
}

fn json_string(path: &str, key: &str) -> Result<String, String> {
    let text = fs::read_to_string(path).map_err(|e| format!("{path}: {e}"))?;
    let quoted = format!("\"{key}\":");
    text.lines()
        .filter_map(|line| line.trim().strip_prefix(quoted.as_str()))
        .map(|rest| rest.trim().trim_end_matches(',').trim_matches('"').to_string())
        .next()
        .ok_or_else(|| format!("{path}: no {key}"))
}
~~~
got := rs.bun()
# The same lookups in shell builtins (no sed/grep: a body should not depend
# on the host's text tools), so the fence's answer is checked, not trusted.
toml_value() { # <file> <key>
	while IFS= read -r line; do
		case "$line" in "$2 = "*) line=${line#*=}; line=${line# }; line=${line#\"}; printf '%s' "${line%\"}"; return 0 ;; esac
	done <"$1"
	return 1
}
version=
while IFS= read -r line; do
	case "${line#"${line%%[! ]*}"}" in '"version":'*) version=${line#*: }; version=${version%,}; version=${version#\"}; version=${version%\"}; break ;; esac
done <package.json
read -r latest <LATEST
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
want="bun $version latest $latest toolchain $channel members $members locked $locked"
[ "$got" = "$want" ] || { echo "smoke: rs.bun() -> '$got', want '$want'" >&2; exit 1; }
echo "smoke: $got"
```
