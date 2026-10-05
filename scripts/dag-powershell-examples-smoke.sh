#!/bin/sh
# Installed-product acceptance smoke for the examples/dag PowerShell front
# door (Sprint 358). Drives the example graph with
# `bashy awd DIR -- bashy dag -f FILE …` (bodies run in the invoking cwd),
# asserts the JSON envelope and the fenced-PowerShell results, and verifies
# that Bash#-owned control flow (loops, conditionals, pipe command form,
# error binding) calling the fenced guest language succeeds.
#
#   BASHY_BIN=~/.local/bin/bashy scripts/dag-powershell-examples-smoke.sh
#
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd -P)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/bashy-dag-powershell.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

fail() {
	echo "dag-powershell-examples-smoke: FAIL: $*" >&2
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
command -v python3 >/dev/null 2>&1 || fail "python3 unavailable"

example_dir=${POWERSHELL_DAG_DIR:-$root/examples/dag/powershell}
example_file="$example_dir/dag.md"
[ -d "$example_dir" ] && [ -f "$example_file" ] || fail "examples/dag/powershell/dag.md missing under $root"

export BASHY_HINTS=off
export DAG_CACHE_DIR="$tmp/dag-cache"

# Discovery: the graph lists, and --explain reports the directory.
"$bashy" awd "$example_dir" -- "$bashy" dag -f "$example_file" --list >"$tmp/powershell.list" || fail "powershell --list"
grep -q '^smoke' "$tmp/powershell.list" || fail "powershell --list has no smoke target"
grep -q '^test' "$tmp/powershell.list" || fail "powershell --list has no test target"

"$bashy" awd "$example_dir" -- "$bashy" dag -f "$example_file" --explain --json smoke >"$tmp/powershell.explain" || fail "powershell --explain"
explained=$(python3 -c 'import json,os,sys; d=json.load(open(sys.argv[1]))["result"]["dir"]; print(d.replace("$HOME", os.environ["HOME"], 1) if d.startswith("$HOME") else d)' "$tmp/powershell.explain")
[ "$(cd "$explained" && pwd -P)" = "$(cd "$example_dir" && pwd -P)" ] || fail "powershell --explain dir = $explained, want $example_dir"

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
    sys.exit(f"dag-powershell-examples-smoke: FAIL: {label}: envelope status {env.get('status')!r}")
tasks = {t["name"]: t for t in env.get("result", {}).get("tasks", [])}
for name in expected:
    t = tasks.get(name)
    if t is None:
        sys.exit(f"dag-powershell-examples-smoke: FAIL: {label}: target {name} missing from envelope")
    if t.get("status") not in ("done", "up-to-date"):
        sys.exit(f"dag-powershell-examples-smoke: FAIL: {label}: target {name} is {t.get('status')} (exit {t.get('exit_code')})\n{t.get('stderr','')[-2000:]}")
print(f"{label}: " + " ".join(f"{n}={tasks[n]['status']}" for n in expected))
PY
}

smoke_line() { # <json-file> <target>
	python3 -c 'import json,sys; t={x["name"]:x for x in json.load(open(sys.argv[1]))["result"]["tasks"]}[sys.argv[2]]; print(t.get("stdout",""))' "$1" "$2"
}

# Run test and smoke targets
"$bashy" awd "$example_dir" -- "$bashy" dag -f "$example_file" --json test smoke >"$tmp/powershell.json" \
	|| fail "powershell dag exited non-zero: $(tail -c 2000 "$tmp/powershell.json")"
check_envelope powershell "$tmp/powershell.json" test smoke

smoke_line "$tmp/powershell.json" test | grep -q "test: powershell ok (res=42)" || fail "powershell test target failed verification"
smoke_line "$tmp/powershell.json" smoke | grep -q "smoke: sum_squares=30" || fail "powershell smoke loop accumulation failed"
smoke_line "$tmp/powershell.json" smoke | grep -q "smoke: branch=gt40" || fail "powershell smoke conditional failed"
smoke_line "$tmp/powershell.json" smoke | grep -q "smoke: shout_pipeline=ok" || fail "powershell smoke pipeline filter failed"
smoke_line "$tmp/powershell.json" smoke | grep -q "smoke: error_handling=ok" || fail "powershell smoke error handling failed"
smoke_line "$tmp/powershell.json" smoke | grep -q "smoke: powershell PASS" || fail "powershell smoke did not report PASS"

echo "bashy=$bashy"
echo "pwsh=$("$bashy" pwsh --version 2>&1 || true)"
echo "dag-powershell-examples-smoke: PASS"
