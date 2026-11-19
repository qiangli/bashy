#!/usr/bin/env bash
# Hello World shaping lifecycle smoke: model-free, deterministic, release-gating.
#
#   scripts/test-shaping-hello-world.sh BASHY_BINARY
#
# Drives the BUILT binary under test through the site-owner lifecycle of
# examples/shaping/hello-world in a disposable HOME/BASHY_HOME and a temp copy
# of the example. No agent, no model, no fake verifier: the candidate comes from
# `shape wish --from` (a prepared workspace) and verify runs the app's real
# checks/smoke.bsh. A console started by this run is the target of the browser
# checkpoint (TestDOMExternalShapeCheckpoint, -tags verifydom).
#
# Steps, each asserted: register -> wish --from -> show -> diff -> verify
# -> preview serves the new greeting -> approve -> a second principal still sees
# "Hello, World!" -> browser checkpoint (both principals) -> undo restores
# "Hello, World!" -> submit records the candidate without activating it.
#
# Environment:
#   SHAPING_SMOKE_BREAK=greeting  deliberately break a step (the smoke check
#                                 expects the OLD greeting); the run MUST fail.
#   SHAPING_SMOKE_SKIP_DOM=1      skip the browser checkpoint (no Chrome).
#   SHAPING_SMOKE_KEEP=1          keep the temp tree for inspection.
#   BASHY_E2E_BROWSER_WS          forwarded to the checkpoint (remote Chrome).
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd -P)
[ "$#" -eq 1 ] || { echo "usage: $0 BASHY_BINARY" >&2; exit 2; }
bin=$1
case $bin in /*) ;; *) bin=$PWD/$bin ;; esac
[ -x "$bin" ] || { echo "test-shaping-hello-world: not executable: $bin" >&2; exit 2; }

tmp=$(mktemp -d "${TMPDIR:-/tmp}/bashy-shaping-smoke.XXXXXX")
real_home=$HOME
serve_pid=
cleanup() {
	[ -z "$serve_pid" ] || kill "$serve_pid" 2>/dev/null || true
	[ -n "${SHAPING_SMOKE_KEEP:-}" ] && { echo "kept: $tmp" >&2; return; }
	rm -rf "$tmp"
}
trap cleanup EXIT HUP INT TERM

steps=0
fail() { echo "test-shaping-hello-world: FAIL: $*" >&2; exit 1; }
ok() { steps=$((steps + 1)); echo "ok $steps - $*"; }
expect_eq() { [ "$2" = "$3" ] || fail "$1: got '$2', want '$3'"; ok "$1"; }

# A private HOME keeps the todo/tracking store and agent state out of the real
# one; the Go toolchain below keeps the real HOME for its module cache.
export BASHY_HINTS=off
mkdir -p "$tmp/home-user" "$tmp/bashy-home"
# Run from the private HOME (never inside a repository: shaping tracks its work
# as todos in the cwd's tree) and outside any weave run's environment.
b() (
	cd "$tmp/home-user"
	for v in "${!WEAVE_@}"; do unset "$v"; done
	HOME=$tmp/home-user BASHY_HOME=$tmp/bashy-home exec "$bin" "$@"
)

jfield() { # jfield KEY < json  -> first top-level-looking "KEY": "value"
	sed -n "s/^  \"$1\": \"\\([^\"]*\\)\".*/\\1/p" | head -n 1
}
greeting_of() { sed -n 's/.*id="greeting">\([^<]*\)<.*/\1/p' | head -n 1; }

# --- 0. disposable copy of the example, registered through its own script ----
example=$tmp/example
mkdir -p "$example"
cp -R "$root/examples/shaping/hello-world/." "$example/"
[ -z "${SHAPING_SMOKE_BREAK:-}" ] || {
	# Deliberate breakage: the app's smoke check demands the old greeting, so a
	# candidate that changes it must fail verification.
	echo "grep -q 'Hello, World!' public/index.html" >>"$example/checks/smoke.bsh"
}
source_dir=$tmp/source
(cd "$tmp/home-user" && BASHY_BIN=$bin HOME=$tmp/home-user BASHY_HOME=$tmp/bashy-home bash "$example/register.sh" "$source_dir") >"$tmp/register.log" 2>&1 \
	|| { cat "$tmp/register.log" >&2; fail "register.sh"; }
b app show hello-world >/dev/null 2>&1 || fail "app hello-world is not registered"
ok "register.sh registered hello-world"

# --- 1. a console started by this run, with two paired principals ------------
port=$((20000 + $(od -An -N2 -tu2 /dev/urandom | tr -d ' ') % 20000))
b app serve --port "$port" --bind lan --pair >"$tmp/serve.log" 2>&1 &
serve_pid=$!
base=http://127.0.0.1:$port
for _ in $(seq 1 60); do
	[ "$(curl -s -o /dev/null -w '%{http_code}' "$base/" 2>/dev/null)" != 000 ] && break
	kill -0 "$serve_pid" 2>/dev/null || { cat "$tmp/serve.log" >&2; fail "console exited early"; }
	sleep 0.5
done
[ "$(curl -s -o /dev/null -w '%{http_code}' "$base/")" != 000 ] || { cat "$tmp/serve.log" >&2; fail "console did not come up on $base"; }
ok "console serving on $base"

pair() { # pair NAME -> NAME.jar and NAME.cookie (value only)
	local url
	url=$(b app pair --no-wait --json --port "$port" --host 127.0.0.1 --principal "$1" --allow hello-world 2>/dev/null |
		sed -n 's/^  "url": "\(.*\)",$/\1/p' | sed 's/\\u0026/\&/g')
	[ -n "$url" ] || fail "pair $1: no ticket"
	local code
	code=$(curl -s -o /dev/null -w '%{http_code}' -c "$tmp/$1.jar" "$url")
	[ "$code" = 303 ] || fail "pair $1: redeem returned $code"
	awk '$6 == "bashy_console" { printf "%s", $7 }' "$tmp/$1.jar" >"$tmp/$1.cookie"
	[ -s "$tmp/$1.cookie" ] || fail "pair $1: no session cookie"
	chmod 600 "$tmp/$1.cookie"
}
pair alice
pair bob
page() { curl -fs -b "$tmp/$1.jar" "$base$2"; }
# A paired device acts as device:NAME; the CLI must name the same principal for
# the console and the CLI to see the same candidates and active versions.
alice=device:alice
bob=device:bob
expect_eq "console principal for alice" "$(page alice /api/shape/hello-world | sed -n 's/.*"principal":"\([^"]*\)".*/\1/p')" "$alice"
ok "paired principals alice and bob"

base_rev=$(b shape status hello-world --user "$alice" --json | jfield active)
[ -n "$base_rev" ] || fail "no active revision"
expect_eq "alice sees the original greeting" "$(page alice /hello-world/ | greeting_of)" "Hello, World!"
expect_eq "bob sees the original greeting" "$(page bob /hello-world/ | greeting_of)" "Hello, World!"

# --- 2. wish --from a prepared workspace --------------------------------------
ws=$tmp/prepared
cp -R "$source_dir" "$ws"
rm -rf "$ws/.git"
sed 's/Hello, World!/Hello, Shaping!/' "$ws/public/index.html" >"$ws/public/index.html.new"
mv "$ws/public/index.html.new" "$ws/public/index.html"
grep -q 'Hello, Shaping!' "$ws/public/index.html" || fail "prepared workspace not changed"
b shape wish hello-world 'Change the displayed greeting from "Hello, World!" to "Hello, Shaping!".' \
	--from "$ws" --user "$alice" --json >"$tmp/wish.json" || { cat "$tmp/wish.json" >&2; fail "wish --from"; }
cand=$(jfield id <"$tmp/wish.json")
rev=$(jfield rev <"$tmp/wish.json")
[ -n "$cand" ] && [ -n "$rev" ] || fail "wish returned no candidate"
expect_eq "wish created a ready candidate" "$(jfield status <"$tmp/wish.json")" ready
expect_eq "active unchanged by wish" "$(b shape status hello-world --user "$alice" --json | jfield active)" "$base_rev"

# --- 3. show, diff ------------------------------------------------------------
expect_eq "show reports the candidate" "$(b shape show hello-world "$cand" --user "$alice" --json | jfield id)" "$cand"
diff_out=$(b shape diff hello-world "$cand" --user "$alice")
case $diff_out in *'-<main><h1 id="greeting">Hello, World!'*'+<main><h1 id="greeting">Hello, Shaping!'*) ok "diff shows the greeting change" ;; *) fail "diff lacks the greeting change: $diff_out" ;; esac

# --- 4. verify: the app's real checks/smoke.bsh -------------------------------
set +e
b shape verify hello-world "$cand" --user "$alice" --json >"$tmp/verify.json" 2>"$tmp/verify.err"
vrc=$?
set -e
[ "$vrc" -eq 0 ] || { cat "$tmp/verify.json" "$tmp/verify.err" >&2; fail "verify exited $vrc"; }
expect_eq "verify status" "$(jfield status <"$tmp/verify.json")" verified
evidence=$(sed -n 's/^    "evidence": "\(.*\)"$/\1/p' "$tmp/verify.json" | head -n 1)
[ -s "$evidence" ] || fail "no verifier evidence file ($evidence)"
tr -d ' \n' <"$evidence" | grep -q '"name":"checks/smoke.bsh","status":"pass"' || fail "evidence does not record checks/smoke.bsh passing"
ok "verifier ran the app's checks/smoke.bsh (evidence sealed)"

# --- 5. preview serves the new greeting, active still the old -----------------
prev_url=$(b shape preview hello-world "$cand" --user "$alice" --console "$base" | sed -n 's/^  "url": "\(.*\)"$/\1/p')
expect_eq "preview url" "$prev_url" "$base/shape/hello-world/preview/$cand/"
expect_eq "preview serves the new greeting" "$(page alice "/shape/hello-world/preview/$cand/" | greeting_of)" "Hello, Shaping!"
expect_eq "active page unchanged during preview" "$(page alice /hello-world/ | greeting_of)" "Hello, World!"

# --- 6. approve; second principal unaffected ---------------------------------
b shape approve hello-world "$cand" --user "$alice" --json >"$tmp/approve.json" || { cat "$tmp/approve.json" >&2; fail "approve"; }
expect_eq "approve status" "$(jfield status <"$tmp/approve.json")" approved
expect_eq "alice's active revision is the candidate" "$(b shape status hello-world --user "$alice" --json | jfield active)" "$rev"
expect_eq "alice sees the new greeting" "$(page alice /hello-world/ | greeting_of)" "Hello, Shaping!"
expect_eq "bob still sees Hello, World!" "$(page bob /hello-world/ | greeting_of)" "Hello, World!"
expect_eq "bob's active revision is the base" "$(b shape status hello-world --user "$bob" --json | jfield active)" "$base_rev"

# --- 7. browser checkpoint against this run's console -------------------------
checkpoint() { # checkpoint PRINCIPAL GREETING ACTIVE
	[ -z "${SHAPING_SMOKE_SKIP_DOM:-}" ] || return 0
	(
		cd "$root"
		HOME=$real_home GOWORK=off \
			BASHY_E2E_CONSOLE_URL=$base \
			BASHY_E2E_COOKIE_FILE=$tmp/$1.cookie \
			BASHY_E2E_ARTIFACT_DIR=$tmp/dom-$1 \
			BASHY_E2E_APP=hello-world \
			BASHY_E2E_PAGE_PATH=/hello-world/ \
			BASHY_E2E_GREETING=$2 \
			BASHY_E2E_ACTIVE=$3 \
			go test github.com/qiangli/yoke/pkg/webconsole -tags verifydom \
			-run '^TestDOMExternalShapeCheckpoint$' -count=1 -v
	) >"$tmp/dom-$1.log" 2>&1 || { tail -n 40 "$tmp/dom-$1.log" >&2; fail "browser checkpoint for $1"; }
	grep -q -- '--- PASS: TestDOMExternalShapeCheckpoint' "$tmp/dom-$1.log" ||
		{ tail -n 20 "$tmp/dom-$1.log" >&2; fail "browser checkpoint for $1 did not pass (skipped?)"; }
	ok "browser checkpoint: $1 sees '$2' at ${3:0:8}"
}
checkpoint alice 'Hello, Shaping!' "$rev"
checkpoint bob 'Hello, World!' "$base_rev"

# --- 8. undo restores the original -------------------------------------------
b shape undo hello-world --user "$alice" >"$tmp/undo.out" 2>&1 || { cat "$tmp/undo.out" >&2; fail "undo"; }
expect_eq "undo restores the active revision" "$(b shape status hello-world --user "$alice" --json | jfield active)" "$base_rev"
expect_eq "undo restores Hello, World!" "$(page alice /hello-world/ | greeting_of)" "Hello, World!"

# --- 9. submit records the candidate without activating it --------------------
b shape submit hello-world "$cand" --user "$alice" >"$tmp/submit.out" 2>&1 || { cat "$tmp/submit.out" >&2; fail "submit"; }
grep -q "\"candidate\": \"$cand\"" "$tmp/submit.out" && grep -q "\"source_revision\": \"$rev\"" "$tmp/submit.out" ||
	{ cat "$tmp/submit.out" >&2; fail "submit did not record the candidate and its source revision"; }
ok "submit recorded the candidate"
expect_eq "submit does not activate" "$(b shape status hello-world --user "$alice" --json | jfield active)" "$base_rev"
expect_eq "page still Hello, World! after submit" "$(page alice /hello-world/ | greeting_of)" "Hello, World!"

echo "test-shaping-hello-world: PASS ($steps checks) binary=$bin candidate=$rev"
