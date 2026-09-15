#!/bin/sh
# Installed-product acceptance smoke for the examples/dag Rust front doors
# (Sprint 188): Codex (codex-rs Cargo workspace under the repo root), uv
# (Cargo workspace at the root, rust-toolchain.toml pinned) and Bun (a Bun
# workspace AND a nightly-pinned Cargo workspace at one root). Not part of
# build or test: it needs three local third-party checkouts plus `rustc`,
# `cargo` (with `rustfmt`), `bun` and `git`. It drives each example graph
# against its checkout with `bashy awd DIR -- bashy dag -f FILE …` (bodies run
# in the invoking cwd, so no file is copied into the checkout), asserts the
# JSON envelope and the fenced results, and verifies every checkout has
# exactly the same git status before and after.
#
#   BASHY_BIN=~/.local/bin/bashy \
#   CODEX_ROOT=/path/to/codex UV_ROOT=/path/to/uv BUN_ROOT=/path/to/bun \
#   scripts/dag-rust-examples-smoke.sh
#
# The uv and Codex graphs BUILD their CLIs (`cargo build -p uv`,
# `cargo build -p codex-cli` — minutes cold, seconds warm; `target/` stays
# gitignored) because their `run` targets launch the built binary from a
# `~~~rs` fence. That needs a `cargo` satisfying each workspace's
# `rust-version`: with rustup's proxies on PATH the pinned channel is used
# automatically; on a host whose PATH `cargo` is a distro build below the
# MSRV, point RUST_TOOLCHAIN_BIN at a toolchain's `bin/` (for rustup:
# ~/.rustup/toolchains/<name>/bin) and it goes first on PATH for the run.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd -P)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/bashy-dag-rust.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

fail() {
	echo "dag-rust-examples-smoke: FAIL: $*" >&2
	exit 1
}

bashy=${BASHY_BIN:-}
if [ -z "$bashy" ]; then
	bashy=$(command -v bashy 2>/dev/null || true)
fi
[ -n "$bashy" ] && [ -x "$bashy" ] || fail "set BASHY_BIN to the installed bashy executable"
bashy_dir=$(CDPATH= cd -- "$(dirname "$bashy")" && pwd -P)
bashy=$bashy_dir/$(basename "$bashy")
case "$bashy" in
	"$root"/*) fail "repo-local binary is not installed-product evidence: $bashy" ;;
esac

# A toolchain directory named by the caller goes first: the repos' own
# `cargo …` targets and the fence's `rustc` both resolve through PATH.
if [ -n "${RUST_TOOLCHAIN_BIN:-}" ]; then
	[ -x "$RUST_TOOLCHAIN_BIN/cargo" ] && [ -x "$RUST_TOOLCHAIN_BIN/rustc" ] || fail "RUST_TOOLCHAIN_BIN=$RUST_TOOLCHAIN_BIN has no cargo/rustc"
	PATH=$RUST_TOOLCHAIN_BIN:$PATH
	export PATH
fi
for tool in rustc cargo git python3; do
	command -v "$tool" >/dev/null 2>&1 || fail "$tool unavailable"
done
cargo fmt --version >/dev/null 2>&1 || fail "rustfmt unavailable (cargo fmt)"

# Bun: the Bun example's own scripts (`bun install`, `bun run lint`) call
# `bun` by name; a Bun named only through BASHPP_BUN joins PATH for the run.
bun=${BASHPP_BUN:-$(command -v bun 2>/dev/null || true)}
[ -n "$bun" ] && [ -x "$bun" ] || fail "bun unavailable (set BASHPP_BUN or put bun on PATH)"
bun_dir=$(CDPATH= cd -- "$(dirname "$bun")" && pwd -P)
bun=$bun_dir/$(basename "$bun")
export BASHPP_BUN="$bun"
command -v bun >/dev/null 2>&1 || { PATH=$bun_dir:$PATH; export PATH; }

codex=${CODEX_ROOT:?set CODEX_ROOT to the Codex checkout}
uv=${UV_ROOT:?set UV_ROOT to the uv checkout}
bunroot=${BUN_ROOT:?set BUN_ROOT to the Bun checkout}
for checkout in "$codex" "$uv" "$bunroot"; do
	git -C "$checkout" rev-parse --is-inside-work-tree >/dev/null 2>&1 || fail "missing checkout: $checkout"
done
[ -f "$codex/codex-rs/Cargo.toml" ] || fail "$codex has no codex-rs/Cargo.toml"
[ -f "$uv/Cargo.toml" ] && [ -f "$uv/rust-toolchain.toml" ] || fail "$uv is not the uv workspace root"
[ -f "$bunroot/Cargo.toml" ] && [ -f "$bunroot/bun.lock" ] || fail "$bunroot is not the Bun source root"
git -C "$codex" status --porcelain=v1 --untracked-files=all >"$tmp/codex.before"
git -C "$uv" status --porcelain=v1 --untracked-files=all >"$tmp/uv.before"
git -C "$bunroot" status --porcelain=v1 --untracked-files=all >"$tmp/bun.before"

export BASHY_HINTS=off
export DAG_CACHE_DIR="$tmp/dag-cache" # never leave a run journal in the checkouts

# Every task in the envelope must be done or up-to-date; a target that
# "failed" or "skipped" is a red gate even if the process exits 0. An error
# envelope (`status: error`, a failed target) is written to stderr, so the
# graph runs are captured `>x.json 2>x.err` and the envelope is whichever
# stream carries it — the failed target's own stderr is then reported.
check_envelope() { # <label> <json-file> <expected-target>...
	label=$1
	json=$2
	shift 2
	[ -s "$json" ] || [ ! -s "$json.err" ] || json=$json.err
	python3 - "$label" "$json" "$@" <<'PY' || exit 1
import json, sys
label, path, expected = sys.argv[1], sys.argv[2], sys.argv[3:]
try:
    with open(path) as f:
        env = json.load(f)
except ValueError as e:
    sys.exit(f"dag-rust-examples-smoke: FAIL: {label}: no JSON envelope ({e})")
tasks = {t["name"]: t for t in env.get("result", {}).get("tasks", [])}
for t in tasks.values():
    if t["status"] not in ("done", "up-to-date", "skipped"):
        sys.exit(f"dag-rust-examples-smoke: FAIL: {label}: target {t['name']} is {t['status']} (exit {t.get('exit_code')})\n{(t.get('stderr') or '')[-2000:]}")
if env.get("status") != "ok":
    sys.exit(f"dag-rust-examples-smoke: FAIL: {label}: envelope status {env.get('status')!r}")
for name in expected:
    t = tasks.get(name)
    if t is None:
        sys.exit(f"dag-rust-examples-smoke: FAIL: {label}: target {name} missing from envelope")
    if t["status"] not in ("done", "up-to-date"):
        sys.exit(f"dag-rust-examples-smoke: FAIL: {label}: target {name} is {t['status']} (exit {t.get('exit_code')})\n{t.get('stderr','')[-2000:]}")
print(f"{label}: " + " ".join(f"{n}={tasks[n]['status']}" for n in expected))
PY
}
task_stdout() { # <json-file> <target>: the target's captured stdout, decoded
	python3 -c 'import json,sys; t={x["name"]:x for x in json.load(open(sys.argv[1]))["result"]["tasks"]}[sys.argv[2]]; print(t.get("stdout",""))' "$1" "$2"
}
# The value a manifest line `key = "value"` carries — what the fences report
# and what this gate expects them to report, read independently here.
toml_value() { # <file> <key>
	while IFS= read -r line; do
		case "$line" in "$2 = "*) line=${line#*=}; line=${line# }; line=${line#\"}; printf '%s' "${line%\"}"; return 0 ;; esac
	done <"$1"
	return 1
}

example_codex="$root/examples/dag/codex/dag.md"
example_uv="$root/examples/dag/uv/dag.md"
example_bun="$root/examples/dag/bun/dag.md"
[ -f "$example_codex" ] && [ -f "$example_uv" ] && [ -f "$example_bun" ] || fail "examples/dag files missing under $root"

# Discovery from another directory: the graph lists, and --explain reports the
# checkout (not the example's directory) as the effective working directory.
"$bashy" awd "$uv" -- "$bashy" dag -f "$example_uv" --list >"$tmp/uv.list" || fail "uv --list"
grep -q '^smoke' "$tmp/uv.list" && grep -q '^run' "$tmp/uv.list" || fail "uv --list lacks smoke/run"
"$bashy" awd "$codex" -- "$bashy" dag -f "$example_codex" --explain --json smoke >"$tmp/codex.explain" || fail "codex --explain"
# Output reduction canonicalizes the home directory to the literal `$HOME`
# token (lossless: expand it back before comparing).
explained=$(python3 -c 'import json,os,sys; d=json.load(open(sys.argv[1]))["result"]["dir"]; print(d.replace("$HOME", os.environ["HOME"], 1) if d.startswith("$HOME") else d)' "$tmp/codex.explain")
[ "$(cd "$explained" && pwd -P)" = "$(cd "$codex" && pwd -P)" ] || fail "codex --explain dir = $explained, want the checkout"

# uv (Cargo workspace at the root): fetch → format check, one crate's tests,
# the fenced-Rust smoke (no build) and the fenced-Rust launcher over the
# built `target/debug/uv`.
"$bashy" awd "$uv" -- "$bashy" dag -f "$example_uv" --json fetch fmt-check test smoke run >"$tmp/uv.json" 2>"$tmp/uv.json.err" || true # an error envelope lands on stderr
check_envelope uv "$tmp/uv.json" fetch fmt-check test build smoke run
uv_version=$(toml_value "$uv/crates/uv/Cargo.toml" version)
uv_channel=$(toml_value "$uv/rust-toolchain.toml" channel)
task_stdout "$tmp/uv.json" smoke | grep -q "^smoke: uv $uv_version msrv [0-9][0-9.]* toolchain $uv_channel locked [1-9][0-9]*\$" || fail "uv smoke did not report the workspace coordinates"
task_stdout "$tmp/uv.json" run | grep -q "^run: uv $uv_version" || fail "uv run did not launch target/debug/uv at $uv_version"
task_stdout "$tmp/uv.json" test | grep -q 'test result: ok' || fail "uv test summary is not clean"

# Codex (codex-rs workspace under the root): fetch → format check, the
# fenced-Rust smoke and the fenced-Rust launcher over the built
# `codex-rs/target/debug/codex`. `test` needs cargo-nextest (the repo's own
# runner) and is a documented target, not a gate target.
"$bashy" awd "$codex" -- "$bashy" dag -f "$example_codex" --json fetch fmt-check smoke run >"$tmp/codex.json" 2>"$tmp/codex.json.err" || true
check_envelope codex "$tmp/codex.json" fetch fmt-check build smoke run
codex_version=$(toml_value "$codex/codex-rs/Cargo.toml" version)
codex_channel=$(toml_value "$codex/codex-rs/rust-toolchain.toml" channel)
task_stdout "$tmp/codex.json" smoke | grep -q "^smoke: codex $codex_version toolchain $codex_channel members [1-9][0-9]* locked [1-9][0-9]*\$" || fail "codex smoke did not report the workspace coordinates"
task_stdout "$tmp/codex.json" run | grep -q "^run: codex-cli $codex_version" || fail "codex run did not launch codex-rs/target/debug/codex at $codex_version"

# Bun (Bun workspace + nightly-pinned Cargo workspace at one root): install →
# lint, the Rust format check and the fenced-Rust smoke. `typecheck` and
# `rust-check` report the checkout's own state (the latter needs the build's
# vendored deps) — documented targets, not gate targets.
"$bashy" awd "$bunroot" -- "$bashy" dag -f "$example_bun" --json install lint fmt-check-rust smoke >"$tmp/bun.json" 2>"$tmp/bun.json.err" || true
check_envelope bun "$tmp/bun.json" install lint fmt-check-rust smoke
bun_channel=$(toml_value "$bunroot/rust-toolchain.toml" channel)
task_stdout "$tmp/bun.json" smoke | grep -q "^smoke: bun [0-9][0-9.]* latest [0-9][0-9.]* toolchain $bun_channel members [1-9][0-9]* locked [1-9][0-9]*\$" || fail "bun smoke did not report the tree's coordinates"
task_stdout "$tmp/bun.json" lint | grep -q 'Found 0 warnings and 0 errors' || fail "bun lint is not clean"

git -C "$codex" status --porcelain=v1 --untracked-files=all >"$tmp/codex.after"
git -C "$uv" status --porcelain=v1 --untracked-files=all >"$tmp/uv.after"
git -C "$bunroot" status --porcelain=v1 --untracked-files=all >"$tmp/bun.after"
cmp -s "$tmp/codex.before" "$tmp/codex.after" || fail "Codex checkout changed during the run"
cmp -s "$tmp/uv.before" "$tmp/uv.after" || fail "uv checkout changed during the run"
cmp -s "$tmp/bun.before" "$tmp/bun.after" || fail "Bun checkout changed during the run"

echo "bashy=$bashy"
echo "rustc=$(rustc --version 2>&1) cargo=$(cargo --version 2>&1)"
echo "codex_commit=$(git -C "$codex" rev-parse HEAD) pinned=$codex_channel"
echo "uv_commit=$(git -C "$uv" rev-parse HEAD) pinned=$uv_channel"
echo "bun_commit=$(git -C "$bunroot" rev-parse HEAD) pinned=$bun_channel bun=$("$bun" --version 2>&1)"
echo "dag-rust-examples-smoke: PASS"
