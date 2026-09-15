#!/bin/sh
# Explicit installed-product acceptance smoke for Sprint 183. This is not part
# of build or test: it needs two local third-party checkouts and their prepared
# Python environments. It performs no installation and verifies both checkouts
# have exactly the same git status before and after the probes.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd -P)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/bashy-s183-python.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

fail() {
	echo "s183-python-import-smoke: FAIL: $*" >&2
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

nano=${NANOCHAT_ROOT:-/Users/qiangli/projects/poc/nanochat}
mini=${MINISWEAGENT_ROOT:-/Users/qiangli/projects/poc/mini-swe-agent}
for checkout in "$nano" "$mini"; do
	git -C "$checkout" rev-parse --is-inside-work-tree >/dev/null 2>&1 || fail "missing checkout: $checkout"
done
git -C "$nano" status --porcelain=v1 --untracked-files=all >"$tmp/nano.before"
git -C "$mini" status --porcelain=v1 --untracked-files=all >"$tmp/mini.before"

# A non-Python source unit must not even validate the Python override.
BASHPP_PYTHON="$tmp/does-not-exist" "$bashy" --bashpp -c 'printf "%s\n" no-python-ok' >"$tmp/lazy.out"
[ "$(cat "$tmp/lazy.out")" = no-python-ok ] || fail "plain Bash++ lazy probe"

nano_python=${BASHPP_PYTHON:-$(command -v python3 2>/dev/null || true)}
[ -n "$nano_python" ] || fail "python3 unavailable for nanochat"
export PYTHONDONTWRITEBYTECODE=1
PYTHONPATH="$nano" "$nano_python" -c 'import importlib.metadata as m, json, nanochat, os, platform, sys
try: version=m.version("nanochat")
except m.PackageNotFoundError: version="unpackaged"
print(json.dumps({"fixture":"nanochat","python":os.path.realpath(sys.executable),"python_version":platform.python_version(),"package_version":version,"module":os.path.realpath(nanochat.__file__)}))' \
	>"$tmp/nano.environment.json"

printf '%s\n' \
	'import python "nanochat.execution" as nano' \
	'result := nano.execute_code("print(6 * 7)", timeout: 5)' \
	'success := result.success' \
	'stdout := result.stdout' \
	'printf "%s %s" "$success" "$stdout"' >"$tmp/nano.bpp"
PYTHONPATH="$nano" BASHPP_PYTHON="$nano_python" "$bashy" --bashpp "$tmp/nano.bpp" >"$tmp/nano.out"
[ "$(cat "$tmp/nano.out")" = "true 42" ] || fail "nanochat direct-import probe: $(cat "$tmp/nano.out")"

command -v uv >/dev/null 2>&1 || fail "uv unavailable for mini-SWE-agent"
(cd "$mini" && uv run --no-sync python -c 'import importlib.metadata as m, json, minisweagent, os, platform, sys
print(json.dumps({"fixture":"mini-swe-agent","python":os.path.realpath(sys.executable),"python_version":platform.python_version(),"package_version":m.version("mini-swe-agent"),"module":os.path.realpath(minisweagent.__file__)}))') \
	>"$tmp/mini.environment.json"
printf '%s\n' \
	'import python "minisweagent.agents" as agents' \
	'agentType := agents.get_agent_class("default")' \
	'name := agentType.__name__' \
	'echo "$name"' >"$tmp/mini.bpp"
(cd "$mini" && PYTHONPATH="$mini/src" BASHPP_PYTHON="$mini/.venv/bin/python" "$bashy" --bashpp "$tmp/mini.bpp") >"$tmp/mini.out"
[ "$(tail -n 1 "$tmp/mini.out")" = DefaultAgent ] || fail "mini-SWE-agent direct-import probe: $(cat "$tmp/mini.out")"

git -C "$nano" status --porcelain=v1 --untracked-files=all >"$tmp/nano.after"
git -C "$mini" status --porcelain=v1 --untracked-files=all >"$tmp/mini.after"
cmp -s "$tmp/nano.before" "$tmp/nano.after" || fail "nanochat checkout changed during probe"
cmp -s "$tmp/mini.before" "$tmp/mini.after" || fail "mini-SWE-agent checkout changed during probe"

echo "bashy=$bashy"
echo "nanochat_commit=$(git -C "$nano" rev-parse HEAD)"
cat "$tmp/nano.environment.json"
echo "mini_swe_agent_commit=$(git -C "$mini" rev-parse HEAD)"
cat "$tmp/mini.environment.json"
echo "s183-python-import-smoke: PASS"
