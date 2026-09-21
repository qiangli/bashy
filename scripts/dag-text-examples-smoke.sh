#!/bin/sh
# Installed-product smoke for the Sprint 234 (B30) text fences: the Caddy
# graph's `image` target — a ```bashpp body holding a ~~~dockerfile fence,
# built with the checkout's Linux build as a named build context and run
# from the image — and the quickstart pipeline.bsh, a script carrying its own
# CI/CD as a ~~~dag island whose targets' Effects: are enforced by @guard.
# The Caddy checkout is provisioned into the bashy examples cache unless
# CADDY_ROOT names one. Needs a working container engine (`bashy podman
# info`); without one this lane FAILS by name rather than skipping — a
# fence that cannot reach its processor is not evidence.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd -P)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/bashy-dag-text.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

fail() {
	echo "dag-text-examples-smoke: FAIL: $*" >&2
	exit 1
}

bashy=${BASHY_BIN:-}
[ -n "$bashy" ] || bashy=$(command -v bashy 2>/dev/null || true)
[ -n "$bashy" ] && [ -x "$bashy" ] || fail "set BASHY_BIN to the installed bashy executable"
bashy_dir=$(CDPATH= cd -- "$(dirname "$bashy")" && pwd -P)
bashy=$bashy_dir/$(basename "$bashy")
case "$bashy" in "$root"/*) fail "repo-local binary is not installed-product evidence: $bashy" ;; esac
for tool in git go python3; do command -v "$tool" >/dev/null 2>&1 || fail "$tool unavailable"; done
"$bashy" podman info >/dev/null 2>&1 || fail "no working container engine: 'bashy podman info' failed (a podman machine must be running)"

cache=${BASHY_EXAMPLES_CACHE:-}
if [ -z "$cache" ]; then
	case $(uname -s) in
	Darwin) cache=$HOME/Library/Caches/bashy/examples ;;
	*) cache=${XDG_CACHE_HOME:-$HOME/.cache}/bashy/examples ;;
	esac
fi

checkout() { # <override> <name> <url> <commit>
	if [ -n "$1" ]; then
		git -C "$1" rev-parse --is-inside-work-tree >/dev/null 2>&1 || fail "missing checkout: $1"
		printf '%s\n' "$1"
		return
	fi
	dir=$cache/$2
	if ! git -C "$dir" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
		echo "dag-text-examples-smoke: cloning $3 @ ${4%"${4#???????????}"} into $dir" >&2
		rm -rf "$dir"
		mkdir -p "$dir"
		git -C "$dir" init -q
		git -C "$dir" remote add origin "$3"
	fi
	if [ "$(git -C "$dir" rev-parse HEAD 2>/dev/null)" != "$4" ]; then
		git -C "$dir" fetch -q --depth 1 origin "$4" || fail "$2: cannot fetch $4"
		git -C "$dir" checkout -q --detach FETCH_HEAD || fail "$2: cannot check out $4"
	fi
	printf '%s\n' "$dir"
}

caddy=$(checkout "${CADDY_ROOT:-}" caddy https://github.com/caddyserver/caddy.git ef1877210ed3fe106edf80b11d9b580d95f0b026)
[ -f "$caddy/caddy.go" ] || fail "$caddy is not the Caddy source root"

status() { git -C "$1" status --porcelain=v1 --untracked-files=all; }
status "$caddy" >"$tmp/caddy.before"

export BASHY_HINTS=off
export DAG_CACHE_DIR="$tmp/dag-cache"
export GOFLAGS=-mod=readonly

# 1. The fenced Dockerfile through bashy dag: build the image from the
#    checkout's Linux build, run `caddy version` from it.
"$bashy" awd "$caddy" -- "$bashy" dag -f "$root/examples/dag/caddy/dag.md" --json image >"$tmp/image.json" 2>"$tmp/image.json.err" || true
python3 - "$tmp/image.json" "$tmp/image.json.err" <<'PY'
import json, sys
path, errpath = sys.argv[1], sys.argv[2]
try:
    env = json.load(open(path))
except Exception:
    env = json.load(open(errpath))
tasks = {t["name"]: t for t in env.get("result", {}).get("tasks", [])}
if env.get("status") != "ok":
    failed = [t for t in tasks.values() if t.get("status") == "failed"]
    detail = (failed[0].get("stderr") or "")[-2000:] if failed else ""
    sys.exit(f"dag-text-examples-smoke: FAIL: caddy image: {env.get('status')}\n{detail}")
for name in ("test", "image"):
    task = tasks.get(name)
    if not task or task.get("status") not in ("done", "up-to-date"):
        sys.exit(f"dag-text-examples-smoke: FAIL: caddy image: {name} not completed")
out = tasks["image"].get("stdout", "")
if not any(line.startswith("image: caddy ") for line in out.splitlines()):
    sys.exit(f"dag-text-examples-smoke: FAIL: caddy image did not run caddy from the fenced image:\n{out}")
print("caddy: test=%s image=%s" % (tasks["test"]["status"], tasks["image"]["status"]))
PY
status "$caddy" >"$tmp/caddy.after"
cmp -s "$tmp/caddy.before" "$tmp/caddy.after" || fail "caddy checkout changed (git status differs)"

# 2. The script that carries its own pipeline: a ~~~dag island, targets as
#    methods, Effects: enforced by @guard (126), from an unrelated cwd.
(cd "$tmp" && "$bashy" --bashsharp "$root/examples/quickstart/pipeline.bsh" >"$tmp/pipeline.out" 2>&1) || true
if ! cmp -s "$tmp/pipeline.out" "$root/examples/quickstart/pipeline.expected"; then
	diff -u "$root/examples/quickstart/pipeline.expected" "$tmp/pipeline.out" >&2 || true
	fail "pipeline.bsh output differs from pipeline.expected"
fi
echo "pipeline: ok (dag island, guard denial 126, published)"
echo "dag-text-examples-smoke: OK"
