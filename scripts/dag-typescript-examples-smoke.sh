#!/bin/sh
# Installed-product acceptance smoke for the examples/dag TypeScript front
# doors (Sprint 186): OpenCode (Bun-managed, Bun runtime), OpenClaw
# (pnpm-managed, Bun runtime for its NodeNext `.js`-for-`.ts` imports) and
# Hermes Agent (uv + npm workspace; Python AND TypeScript fences in one body,
# Node runtime). Not part of build or test: it needs `uv`, `node`, `npm`,
# `bun` and `pnpm` (or `corepack`). It
# drives each example graph against its checkout with
# `bashy awd DIR -- bashy dag -f FILE …` (bodies run in the invoking cwd, so
# no file is copied into the checkout), asserts the JSON envelope and the
# fenced results, and verifies every checkout has byte-identical git status
# before and after.
#
#   BASHY_BIN=~/.local/bin/bashy scripts/dag-typescript-examples-smoke.sh
#
# The three checkouts are dependencies the gate provisions itself: each repo is
# pinned (URL + commit, the coordinates this gate was measured against) and,
# when its *_ROOT variable is not set, cloned shallow at that commit into
# bashy's cache — <user cache dir>/bashy/examples/<name>, i.e.
# ~/Library/Caches/bashy/examples on macOS, $XDG_CACHE_HOME/bashy/examples
# (~/.cache/bashy/examples) on Linux — on first use and reused after (the
# builds in it stay warm; a moved pin re-fetches). OPENCODE_ROOT,
# OPENCLAW_ROOT and HERMESAGENT_ROOT each name an existing checkout instead,
# at whatever commit it is — never touched by the gate. BASHY_EXAMPLES_CACHE
# overrides the cache directory.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd -P)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/bashy-dag-typescript.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

fail() {
	echo "dag-typescript-examples-smoke: FAIL: $*" >&2
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
for tool in uv node npm python3 git; do
	command -v "$tool" >/dev/null 2>&1 || fail "$tool unavailable"
done

# Bun: the runtime the OpenCode and OpenClaw examples select
# (BASHPP_TYPESCRIPT_RUNTIME=bun); the engine honours BASHPP_BUN, so a Bun
# outside PATH is fine.
bun=${BASHPP_BUN:-$(command -v bun 2>/dev/null || true)}
[ -n "$bun" ] && [ -x "$bun" ] || fail "bun unavailable (set BASHPP_BUN or put bun on PATH)"
bun_dir=$(CDPATH= cd -- "$(dirname "$bun")" && pwd -P)
bun=$bun_dir/$(basename "$bun")
export BASHPP_BUN="$bun"
# The repos' own targets (`bun install`, `bun test`, `bun run …`) call `bun`
# by name, so a Bun named only through BASHPP_BUN joins PATH for the run.
command -v bun >/dev/null 2>&1 || { PATH=$bun_dir:$PATH; export PATH; }
# pnpm: OpenClaw's package manager. corepack (bundled with Node 16-24, still
# installable afterwards) can provide it without a global install.
if ! command -v pnpm >/dev/null 2>&1; then
	command -v corepack >/dev/null 2>&1 || fail "pnpm unavailable (install pnpm or corepack)"
	mkdir -p "$tmp/bin"
	printf '#!/bin/sh\nexec corepack pnpm "$@"\n' >"$tmp/bin/pnpm"
	chmod +x "$tmp/bin/pnpm"
	PATH=$tmp/bin:$PATH
	export PATH
fi

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
		echo "dag-typescript-examples-smoke: cloning $3 @ ${4%"${4#???????????}"} into $dir" >&2
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
opencode=$(checkout "${OPENCODE_ROOT:-}" opencode https://github.com/sst/opencode.git e03db9bc6908f75c9334d8aa997deeaac81c0298)
openclaw=$(checkout "${OPENCLAW_ROOT:-}" openclaw https://github.com/openclaw/openclaw.git 067c75b8aa65607a41d0a5b7cd102d80eec33bd6)
hermes=$(checkout "${HERMESAGENT_ROOT:-}" hermes-agent https://github.com/NousResearch/Hermes-Agent.git 5655920f9aeb6bb6d9b0d8b96b0f989c5f90c276)
[ -f "$opencode/package.json" ] && [ -f "$opencode/bun.lock" ] && [ -d "$opencode/packages/opencode" ] || fail "$opencode is not the OpenCode source root"
[ -f "$openclaw/package.json" ] && [ -f "$openclaw/pnpm-lock.yaml" ] && [ -f "$openclaw/pnpm-workspace.yaml" ] || fail "$openclaw is not the OpenClaw source root"
[ -f "$hermes/pyproject.toml" ] && [ -f "$hermes/package-lock.json" ] && [ -d "$hermes/ui-tui" ] || fail "$hermes is not the Hermes Agent source root"
status() { git -C "$1" status --porcelain=v1 --untracked-files=all; }
status "$opencode" >"$tmp/opencode.before"
status "$openclaw" >"$tmp/openclaw.before"
status "$hermes" >"$tmp/hermes.before"

export BASHY_HINTS=off
export DAG_CACHE_DIR="$tmp/dag-cache" # never leave a run journal in the checkouts
# One Hermes TUI suite reads the terminal program to pick a default palette;
# the clean-terminal baseline it asserts is the one CI sees.
unset TERM_PROGRAM

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
    sys.exit(f"dag-typescript-examples-smoke: FAIL: {label}: envelope status {env.get('status')!r}")
tasks = {t["name"]: t for t in env["result"]["tasks"]}
for name in expected:
    t = tasks.get(name)
    if t is None:
        sys.exit(f"dag-typescript-examples-smoke: FAIL: {label}: target {name} missing from envelope")
    if t["status"] not in ("done", "up-to-date"):
        sys.exit(f"dag-typescript-examples-smoke: FAIL: {label}: target {name} is {t['status']} (exit {t.get('exit_code')})\n{t.get('stderr','')[-2000:]}")
print(f"{label}: " + " ".join(f"{n}={tasks[n]['status']}" for n in expected))
PY
}
task_stdout() { # <json-file> <target>: the target's captured stdout, decoded
	python3 -c 'import json,sys; t={x["name"]:x for x in json.load(open(sys.argv[1]))["result"]["tasks"]}[sys.argv[2]]; print(t.get("stdout",""))' "$1" "$2"
}

example_opencode="$root/examples/dag/opencode/dag.md"
example_openclaw="$root/examples/dag/openclaw/dag.md"
example_hermes="$root/examples/dag/hermes-agent/dag.md"
[ -f "$example_opencode" ] && [ -f "$example_openclaw" ] && [ -f "$example_hermes" ] || fail "examples/dag files missing under $root"

# Discovery from another directory: the graph lists, and --explain reports the
# checkout (not the example's directory) as the effective working directory.
"$bashy" awd "$opencode" -- "$bashy" dag -f "$example_opencode" --list >"$tmp/opencode.list" || fail "opencode --list"
grep -q '^smoke' "$tmp/opencode.list" || fail "opencode --list has no smoke target"
"$bashy" awd "$openclaw" -- "$bashy" dag -f "$example_openclaw" --explain --json smoke >"$tmp/openclaw.explain" || fail "openclaw --explain"
# Output reduction canonicalizes the home directory to the literal `$HOME`
# token (lossless: expand it back before comparing).
explained=$(python3 -c 'import json,os,sys; d=json.load(open(sys.argv[1]))["result"]["dir"]; print(d.replace("$HOME", os.environ["HOME"], 1) if d.startswith("$HOME") else d)' "$tmp/openclaw.explain")
[ "$(cd "$explained" && pwd -P)" = "$(cd "$openclaw" && pwd -P)" ] || fail "openclaw --explain dir = $explained, want the checkout"

# OpenCode (Bun workspace): install → typecheck, the config suite, the CLI
# entry, and the fenced-TypeScript smoke on Bun.
"$bashy" awd "$opencode" -- "$bashy" dag -f "$example_opencode" --json install typecheck test-config run smoke >"$tmp/opencode.json" \
	|| fail "opencode graph exited non-zero: $(tail -c 2000 "$tmp/opencode.json")"
check_envelope opencode "$tmp/opencode.json" install typecheck test-config run smoke
task_stdout "$tmp/opencode.json" run | grep -qx 'local' || fail "opencode run did not print local"
task_stdout "$tmp/opencode.json" smoke | grep -q 'smoke: fileInDirectory -> .opencode/opencode.json|.opencode/opencode.jsonc' || fail "opencode smoke did not report the config candidates"

# OpenClaw (pnpm workspace): install → typecheck, format check, one targeted
# vitest file, the CLI wrapper, and the fenced-TypeScript smoke on Bun.
"$bashy" awd "$openclaw" -- "$bashy" dag -f "$example_openclaw" --json install typecheck format-check test-file run smoke >"$tmp/openclaw.json" \
	|| fail "openclaw graph exited non-zero: $(tail -c 2000 "$tmp/openclaw.json")"
check_envelope openclaw "$tmp/openclaw.json" install typecheck format-check test-file run smoke
task_stdout "$tmp/openclaw.json" run | grep -q '^OpenClaw [0-9]' || fail "openclaw run did not print a version"
task_stdout "$tmp/openclaw.json" smoke | grep -q 'smoke: parseBooleanValue/chunkItems -> true:undefined:3' || fail "openclaw smoke did not report true:undefined:3"

# Hermes Agent (uv + npm workspace): the Python lane (sync → lint, run, the
# skills tests) and the TUI lane (install → build-ink → typecheck, vitest),
# then one body with a Python fence AND a TypeScript fence (Node runtime).
"$bashy" awd "$hermes" -- "$bashy" dag -f "$example_hermes" --json sync lint run install-tui typecheck-tui test-tui smoke test TEST_PATHS=tests/skills >"$tmp/hermes.json" \
	|| fail "hermes-agent graph exited non-zero: $(tail -c 2000 "$tmp/hermes.json")"
check_envelope hermes-agent "$tmp/hermes.json" sync lint run install-tui build-ink typecheck-tui test-tui smoke test
task_stdout "$tmp/hermes.json" smoke | grep -q 'smoke: hermes-agent [0-9][0-9.]*; compactNumber(1500000) -> 1.5M' || fail "hermes-agent smoke did not report both fences"
task_stdout "$tmp/hermes.json" test | grep -q 'tests passed, 0 failed' || fail "hermes-agent test summary is not clean"

status "$opencode" >"$tmp/opencode.after"
status "$openclaw" >"$tmp/openclaw.after"
status "$hermes" >"$tmp/hermes.after"
cmp -s "$tmp/opencode.before" "$tmp/opencode.after" || fail "OpenCode checkout changed during the run"
cmp -s "$tmp/openclaw.before" "$tmp/openclaw.after" || fail "OpenClaw checkout changed during the run"
cmp -s "$tmp/hermes.before" "$tmp/hermes.after" || fail "Hermes Agent checkout changed during the run"

echo "bashy=$bashy"
echo "opencode_commit=$(git -C "$opencode" rev-parse HEAD) bun=$("$bun" --version 2>&1)"
echo "openclaw_commit=$(git -C "$openclaw" rev-parse HEAD) node=$(node --version 2>&1) pnpm=$(cd "$openclaw" && pnpm --version 2>&1)"
echo "hermes_agent_commit=$(git -C "$hermes" rev-parse HEAD) python=$("$hermes/.venv/bin/python" --version 2>&1) node=$(node --version 2>&1)"
echo "dag-typescript-examples-smoke: PASS"
