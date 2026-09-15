#!/bin/sh
# Installed-product acceptance smoke for the examples/dag Python front doors
# (Sprint 185). Not part of build or test: it needs `uv`. It drives each
# example graph against its checkout with `bashy awd DIR -- bashy dag -f
# FILE …` (bodies run in the invoking cwd, so no file is copied into the
# checkout), asserts the JSON envelope and the fenced-Python results, and
# verifies both checkouts have byte-identical git status before and after.
#
#   BASHY_BIN=~/.local/bin/bashy scripts/dag-python-examples-smoke.sh
#
# The two checkouts are dependencies the gate provisions itself: each repo is
# pinned (URL + commit, the coordinates this gate was measured against) and,
# when its *_ROOT variable is not set, cloned shallow at that commit into
# bashy's cache — <user cache dir>/bashy/examples/<name>, i.e.
# ~/Library/Caches/bashy/examples on macOS, $XDG_CACHE_HOME/bashy/examples
# (~/.cache/bashy/examples) on Linux — on first use and reused after (the
# builds in it stay warm; a moved pin re-fetches). MINISWEAGENT_ROOT and
# NANOCHAT_ROOT each name an existing checkout instead, at whatever commit it
# is — never touched by the gate. BASHY_EXAMPLES_CACHE overrides the cache
# directory.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd -P)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/bashy-dag-python.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

fail() {
	echo "dag-python-examples-smoke: FAIL: $*" >&2
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
command -v uv >/dev/null 2>&1 || fail "uv unavailable"
command -v python3 >/dev/null 2>&1 || fail "python3 unavailable"

# The pinned checkouts (name, URL, commit) — the gate's dependencies.
cache=${BASHY_EXAMPLES_CACHE:-}
if [ -z "$cache" ]; then
	case $(uname -s) in
		Darwin) cache=$HOME/Library/Caches/bashy/examples ;;
		*) cache=${XDG_CACHE_HOME:-$HOME/.cache}/bashy/examples ;;
	esac
fi
checkout() { # <root-or-empty> <name> <url> <commit>: prints the checkout to use
	if [ -n "$1" ]; then
		git -C "$1" rev-parse --is-inside-work-tree >/dev/null 2>&1 || fail "missing checkout: $1"
		printf '%s\n' "$1"
		return 0
	fi
	dir=$cache/$2
	if ! git -C "$dir" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
		echo "dag-python-examples-smoke: cloning $3 @ ${4%"${4#???????????}"} into $dir" >&2
		rm -rf "$dir"
		mkdir -p "$dir"
		git -C "$dir" init -q
		git -C "$dir" remote add origin "$3"
	fi
	if [ "$(git -C "$dir" rev-parse HEAD 2>/dev/null)" != "$4" ]; then
		git -C "$dir" fetch -q --depth 1 origin "$4" || fail "$2: cannot fetch $4 from $3"
		git -C "$dir" checkout -q --detach FETCH_HEAD || fail "$2: cannot check out $4"
	fi
	printf '%s\n' "$dir"
}
mini=$(checkout "${MINISWEAGENT_ROOT:-}" mini-swe-agent https://github.com/SWE-agent/mini-swe-agent.git 04d809ceab9df28f9adaed044884180159172930)
nano=$(checkout "${NANOCHAT_ROOT:-}" nanochat https://github.com/karpathy/nanochat.git 92d63d4e8bb4df75c3b71618f31ddde2378b2bcd)
[ -f "$mini/pyproject.toml" ] && [ -d "$mini/src/minisweagent" ] || fail "$mini is not the mini-SWE-agent source root"
[ -f "$nano/pyproject.toml" ] && [ -f "$nano/uv.lock" ] && [ -d "$nano/nanochat" ] || fail "$nano is not the nanochat source root"
status() { git -C "$1" status --porcelain=v1 --untracked-files=all; }
status "$mini" >"$tmp/mini.before"
status "$nano" >"$tmp/nano.before"

export BASHY_HINTS=off
export DAG_CACHE_DIR="$tmp/dag-cache" # never leave a run journal in the checkouts

# The front-door awd itself: runs one command over there, cwd untouched.
got=$("$bashy" awd "$tmp" -- pwd)
[ "$(cd "$got" && pwd -P)" = "$(cd "$tmp" && pwd -P)" ] || fail "bashy awd: pwd in $tmp gave $got"

# Every task in the envelope must be done or up-to-date; a target that
# "failed" or "skipped" is a red gate even if the process exits 0.
check_envelope() { # <label> <json-file> <expected-target>...
	label=$1
	json=$2
	shift 2
	python3 - "$label" "$json" "$@" <<'PY' || exit 1
import json, sys
label, path, expected = sys.argv[1], sys.argv[2], sys.argv[3:]
with open(path) as f:
    env = json.load(f)
if env.get("status") != "ok":
    sys.exit(f"dag-python-examples-smoke: FAIL: {label}: envelope status {env.get('status')!r}")
tasks = {t["name"]: t for t in env["result"]["tasks"]}
for name in expected:
    t = tasks.get(name)
    if t is None:
        sys.exit(f"dag-python-examples-smoke: FAIL: {label}: target {name} missing from envelope")
    if t["status"] not in ("done", "up-to-date"):
        sys.exit(f"dag-python-examples-smoke: FAIL: {label}: target {name} is {t['status']} (exit {t.get('exit_code')})\n{t.get('stderr','')[-2000:]}")
print(f"{label}: " + " ".join(f"{n}={tasks[n]['status']}" for n in expected))
PY
}

example_mini="$root/examples/dag/mini-swe-agent/dag.md"
example_nano="$root/examples/dag/nanochat/dag.md"
[ -f "$example_mini" ] && [ -f "$example_nano" ] || fail "examples/dag files missing under $root"

# Discovery from another directory: the graph lists, and --explain reports the
# checkout (not the example's directory) as the effective working directory.
"$bashy" awd "$mini" -- "$bashy" dag -f "$example_mini" --list >"$tmp/mini.list" || fail "mini-swe-agent --list"
grep -q '^smoke' "$tmp/mini.list" || fail "mini-swe-agent --list has no smoke target"
"$bashy" awd "$nano" -- "$bashy" dag -f "$example_nano" --explain --json smoke >"$tmp/nano.explain" || fail "nanochat --explain"
# Output reduction canonicalizes the home directory to the literal `$HOME`
# token (lossless: expand it back before comparing).
explained=$(python3 -c 'import json,os,sys; d=json.load(open(sys.argv[1]))["result"]["dir"]; print(d.replace("$HOME", os.environ["HOME"], 1) if d.startswith("$HOME") else d)' "$tmp/nano.explain")
[ "$(cd "$explained" && pwd -P)" = "$(cd "$nano" && pwd -P)" ] || fail "nanochat --explain dir = $explained, want the checkout"

# mini-SWE-agent: sync → lint, run, test, and the fenced-Python smoke.
"$bashy" awd "$mini" -- "$bashy" dag -f "$example_mini" --json sync lint run test smoke >"$tmp/mini.json" \
	|| fail "mini-swe-agent graph exited non-zero: $(tail -c 2000 "$tmp/mini.json")"
check_envelope mini-swe-agent "$tmp/mini.json" sync lint run test smoke
smoke_line() { # <json-file> <target>: the target's captured stdout, decoded
	python3 -c 'import json,sys; t={x["name"]:x for x in json.load(open(sys.argv[1]))["result"]["tasks"]}[sys.argv[2]]; print(t.get("stdout",""))' "$1" "$2"
}
smoke_line "$tmp/mini.json" smoke | grep -q "smoke: get_agent_class('default') -> DefaultAgent" || fail "mini-swe-agent smoke did not report DefaultAgent"

# nanochat: sync → test and the fenced-Python smoke (execute_code -> 42).
"$bashy" awd "$nano" -- "$bashy" dag -f "$example_nano" --json sync test smoke >"$tmp/nano.json" \
	|| fail "nanochat graph exited non-zero: $(tail -c 2000 "$tmp/nano.json")"
check_envelope nanochat "$tmp/nano.json" sync test smoke
smoke_line "$tmp/nano.json" smoke | grep -q "smoke: execute_code -> 42" || fail "nanochat smoke did not report 42"

status "$mini" >"$tmp/mini.after"
status "$nano" >"$tmp/nano.after"
cmp -s "$tmp/mini.before" "$tmp/mini.after" || fail "mini-SWE-agent checkout changed during the run"
cmp -s "$tmp/nano.before" "$tmp/nano.after" || fail "nanochat checkout changed during the run"

echo "bashy=$bashy"
echo "mini_swe_agent_commit=$(git -C "$mini" rev-parse HEAD) python=$("$mini/.venv/bin/python" --version 2>&1)"
echo "nanochat_commit=$(git -C "$nano" rev-parse HEAD) python=$("$nano/.venv/bin/python" --version 2>&1)"
echo "dag-python-examples-smoke: PASS"
