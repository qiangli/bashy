#!/bin/sh
# Installed-product smoke for Sprint 192's gh, Hugo, and Caddy dag front
# doors. With no *_ROOT variables, pinned checkouts are provisioned into the
# bashy examples cache and reused on later runs.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd -P)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/bashy-dag-go.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

fail() {
	echo "dag-go-examples-smoke: FAIL: $*" >&2
	exit 1
}

bashy=${BASHY_BIN:-}
[ -n "$bashy" ] || bashy=$(command -v bashy 2>/dev/null || true)
[ -n "$bashy" ] && [ -x "$bashy" ] || fail "set BASHY_BIN to the installed bashy executable"
bashy_dir=$(CDPATH= cd -- "$(dirname "$bashy")" && pwd -P)
bashy=$bashy_dir/$(basename "$bashy")
case "$bashy" in "$root"/*) fail "repo-local binary is not installed-product evidence: $bashy" ;; esac
for tool in git go python3; do command -v "$tool" >/dev/null 2>&1 || fail "$tool unavailable"; done

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
		echo "dag-go-examples-smoke: cloning $3 @ ${4%"${4#???????????}"} into $dir" >&2
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

gh=$(checkout "${GH_ROOT:-}" gh https://github.com/cli/cli.git cac0dd795eeb5b9a448474fad7e295272fc5c486)
hugo=$(checkout "${HUGO_ROOT:-}" hugo https://github.com/gohugoio/hugo.git a374b865f7e99cccafcbe443dcdb47552b91af3c)
caddy=$(checkout "${CADDY_ROOT:-}" caddy https://github.com/caddyserver/caddy.git ef1877210ed3fe106edf80b11d9b580d95f0b026)
[ -f "$gh/internal/build/build.go" ] || fail "$gh is not the GitHub CLI source root"
[ -f "$hugo/common/hugo/version_current.go" ] || fail "$hugo is not the Hugo source root"
[ -f "$caddy/caddy.go" ] || fail "$caddy is not the Caddy source root"

status() { git -C "$1" status --porcelain=v1 --untracked-files=all; }
status "$gh" >"$tmp/gh.before"
status "$hugo" >"$tmp/hugo.before"
status "$caddy" >"$tmp/caddy.before"

export BASHY_HINTS=off
export DAG_CACHE_DIR="$tmp/dag-cache"
export GOFLAGS=-mod=readonly

check_envelope() { # <label> <json> <targets...>
	label=$1 json=$2
	shift 2
	[ -s "$json" ] || [ ! -s "$json.err" ] || json=$json.err
	python3 - "$label" "$json" "$@" <<'PY'
import json, sys
label, path, expected = sys.argv[1], sys.argv[2], sys.argv[3:]
with open(path) as f:
    env = json.load(f)
tasks = {t["name"]: t for t in env.get("result", {}).get("tasks", [])}
if env.get("status") != "ok":
    failed = [t for t in tasks.values() if t.get("status") == "failed"]
    detail = (failed[0].get("stderr") or "")[-2000:] if failed else ""
    sys.exit(f"dag-go-examples-smoke: FAIL: {label}: {env.get('status')}\n{detail}")
for name in expected:
    task = tasks.get(name)
    if not task or task.get("status") not in ("done", "up-to-date"):
        sys.exit(f"dag-go-examples-smoke: FAIL: {label}: {name} not completed")
print(f"{label}: " + " ".join(f"{n}={tasks[n]['status']}" for n in expected))
PY
}

task_stdout() {
	python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); t={x["name"]:x for x in d["result"]["tasks"]}; print(t[sys.argv[2]].get("stdout",""))' "$1" "$2"
}

run_graph() { # <label> <checkout> <example>
	label=$1 checkout_root=$2 example=$3
	"$bashy" awd "$checkout_root" -- "$bashy" dag -f "$example" --json test smoke run >"$tmp/$label.json" 2>"$tmp/$label.json.err" || true
	check_envelope "$label" "$tmp/$label.json" test smoke build run
	task_stdout "$tmp/$label.json" smoke | grep -q "^smoke: $label " || fail "$label smoke did not call its checkout package"
	task_stdout "$tmp/$label.json" run | grep -q '^run: ' || fail "$label run did not launch its built binary"
}

run_graph gh "$gh" "$root/examples/dag/gh/dag.md"
run_graph hugo "$hugo" "$root/examples/dag/hugo/dag.md"
run_graph caddy "$caddy" "$root/examples/dag/caddy/dag.md"

status "$gh" >"$tmp/gh.after"
status "$hugo" >"$tmp/hugo.after"
status "$caddy" >"$tmp/caddy.after"
cmp -s "$tmp/gh.before" "$tmp/gh.after" || fail "gh checkout status changed"
cmp -s "$tmp/hugo.before" "$tmp/hugo.after" || fail "Hugo checkout status changed"
cmp -s "$tmp/caddy.before" "$tmp/caddy.after" || fail "Caddy checkout status changed"

echo "dag-go-examples-smoke: OK — gh/Hugo/Caddy package fences, focused tests, builds, launches, and clean checkouts passed"
