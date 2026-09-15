---
name: mise
description: bashy dag front door for mise — the project's own build and unit-test tasks plus a Rust fence that reads the checkout's workspace coordinates
default: build
---

# mise — task graph

The repo's own task front doors (`mise run build` and `mise run test:unit`),
wired as a dependency graph. This file lives at the root of
[jdx/mise](https://github.com/jdx/mise): `bashy dag build` builds its debug
CLI and `bashy dag test` runs the upstream unit-test lane. The pinned example
is measured at `55d3b4fc789d76fbaa486cb523f92cc974ce67c7`.

Mise's `AGENTS.md` makes `mise run` significant: it activates the project's
mbx Cargo wrapper. Keep that wrapper enabled. If it itself fails, upstream
directs the operator to rerun the equivalent Cargo command with
`MBX_DISABLE=1` and surface the mismatch; that is a diagnostic fallback, not
an environment this graph sets or silently retries with.

## Tasks

### build
The lightweight upstream build front door (`mise run build`), which invokes
`cargo build --all-features` through the project-selected mbx wrapper. It
writes only ignored build artifacts and tool caches.
Effects: read, write
Timeout: 1h

```bash
mise run build
```

### test
The upstream unit-test front door (`mise run test:unit`), rather than running
the e2e scripts directly. It builds test artifacts and runs the all-feature
unit suite through the project-selected environment.
Requires: build
Effects: read, write
Timeout: 1h

```bash
mise run test:unit
```

### smoke
Read checkout-owned version and workspace coordinates from a `~~~rs` worker,
then cross-check them with shell builtins. This target needs no project build,
no wrapper script, and no inherited mise environment: the Rust fence is
std-only and reads the committed manifests and lockfile in the invoking
checkout.
Effects: read

```bashpp
~~~rs as rs
use std::fs;

pub fn mise() -> Result<String, String> {
    let version = toml_value("Cargo.toml", "version")?;
    let msrv = toml_value("Cargo.toml", "rust-version")?;
    let minimum = toml_value("mise.toml", "min_version")?;
    let manifest = fs::read_to_string("Cargo.toml").map_err(|e| format!("Cargo.toml: {e}"))?;
    let members = manifest.lines().skip_while(|line| *line != "members = [").skip(1)
        .take_while(|line| line.trim() != "]").filter(|line| line.trim_start().starts_with('"')).count();
    let locked = fs::read_to_string("Cargo.lock").map_err(|e| format!("Cargo.lock: {e}"))?
        .lines().filter(|line| *line == "[[package]]").count();
    Ok(format!("mise {version} msrv {msrv} minimum {minimum} members {members} locked {locked}"))
}

fn toml_value(path: &str, key: &str) -> Result<String, String> {
    let text = fs::read_to_string(path).map_err(|e| format!("{path}: {e}"))?;
    text.lines().filter_map(|line| line.split_once('='))
        .find(|(k, _)| k.trim() == key)
        .map(|(_, v)| v.trim().trim_matches('"').to_string())
        .ok_or_else(|| format!("{path}: no {key}"))
}
~~~
got := rs.mise()
toml_value() { # <file> <key>
	while IFS= read -r line; do
		case "$line" in "$2 = "*) line=${line#*=}; line=${line# }; line=${line#\"}; printf '%s' "${line%\"}"; return 0 ;; esac
	done <"$1"
	return 1
}
version=$(toml_value Cargo.toml version)
msrv=$(toml_value Cargo.toml rust-version)
minimum=$(toml_value mise.toml min_version)
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
want="mise $version msrv $msrv minimum $minimum members $members locked $locked"
[ "$got" = "$want" ] || { echo "smoke: rs.mise() -> '$got', want '$want'" >&2; exit 1; }
echo "smoke: $got"
```

### run
Launch the debug CLI from a Rust fence after the upstream build task. The
fence returns its `--version` line and the shell checks it against the
checkout's declared version.
Requires: build
Effects: read

```bashpp
~~~rs as rs
use std::process::Command;

pub fn launch() -> Result<String, String> {
    let out = Command::new("target/debug/mise").arg("--version").output()
        .map_err(|e| format!("target/debug/mise: {e}"))?;
    if !out.status.success() { return Err(String::from_utf8_lossy(&out.stderr).trim().to_string()); }
    Ok(String::from_utf8_lossy(&out.stdout).trim().to_string())
}
~~~
got := rs.launch()
version=
while IFS= read -r line; do
	case "$line" in 'version = "'*) version=${line#*\"}; version=${version%\"}; break ;; esac
done <Cargo.toml
case "$got" in
	"mise $version"*) ;;
	*) echo "run: rs.launch() -> '$got', want 'mise $version…'" >&2; exit 1 ;;
esac
echo "run: $got"
```
