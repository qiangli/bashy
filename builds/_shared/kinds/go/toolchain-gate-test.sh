#!/usr/bin/env bash
# toolchain-gate-test.sh — self-test for the go kind's two toolchain gates.
#
# builds/check.sh runs this. The gates live in `run:` blocks inside
# builds/_shared/kinds/go/setup.yml, so they are only ever executed on a GitHub
# runner during a real release — which is a bad place to discover that a rule
# inverted. This extracts the shell out of that fragment and drives it against
# synthetic declarations, so the rules are checkable on a laptop:
#
#   the TOOLCHAIN gate   GO_TOOLCHAIN_SECURITY_OVERRIDE is an in-minor patch
#                        bump, or an EOL-minor move to the oldest supported
#                        minor, and nothing else
#   the DIRECTIVE gate   `go get` may raise go.mod's `go` line only up to the
#                        effective toolchain, never past it and never down
#
# GO_SUPPORTED_MINORS is substituted with a FIXED value here, not render.sh's
# current one: the subject is the rule, not today's Go release.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
setup=$here/setup.yml
[[ -f $setup ]] || { echo "toolchain-gate-test: missing $setup" >&2; exit 1; }

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# --- extract the two `run:` blocks -----------------------------------------
# Every step body in the fragment is indented exactly 10 spaces under `run: |`.
# Finding anything but two blocks is a failure, not a reason to skip: the point
# of this test is that the gates are covered.
awk '
	/^        run: \|$/ { n++; inblock = 1; next }
	inblock && /^          / { print > ("'"$work"'/block." n); next }
	inblock && /^[[:space:]]*$/ { next }
	{ inblock = 0 }
' "$setup"
for n in 1 2; do
	[[ -s $work/block.$n ]] || { echo "toolchain-gate-test: no run block $n in setup.yml" >&2; exit 1; }
	sed -e 's/^          //' -e 's/@@GO_SUPPORTED_MINORS@@/1.26 1.27/g' \
		"$work/block.$n" >"$work/step$n.sh"
done
[[ ! -e $work/block.3 ]] || { echo "toolchain-gate-test: setup.yml grew a third run block — extend this test" >&2; exit 1; }
grep -q 'GO_TOOLCHAIN_SECURITY_OVERRIDE' "$work/step1.sh" ||
	{ echo "toolchain-gate-test: block 1 is not the toolchain gate" >&2; exit 1; }

# The directive half of block 2, between its BEGIN and END markers.
{
	echo 'set -euo pipefail'
	sed -n '/the go\/toolchain directive gate (BEGIN)/,/the go\/toolchain directive gate (END)/p' \
		"$work/step2.sh" | sed '1d;$d'
	echo 'echo "directives_ok=$directives_ok"'
} >"$work/directives.sh"
grep -q 'directives_ok=0' "$work/directives.sh" ||
	{ echo "toolchain-gate-test: setup.yml's directive-gate markers did not delimit the gate" >&2; exit 1; }

# A `go` that reports whatever GOTOOLCHAIN asks for, so the toolchain gate's
# closing `go version` neither downloads a release nor needs one installed.
mkdir -p "$work/bin"
cat >"$work/bin/go" <<'STUB'
#!/bin/sh
[ "$1" = version ] || { echo "stub go: unexpected: $*" >&2; exit 1; }
echo "go version ${GOTOOLCHAIN:-go0.0.0} test/test"
STUB
chmod +x "$work/bin/go"
PATH=$work/bin:$PATH
export PATH

rc=0
pass=0

toolchain_case() { # DESC GOMOD_TOOLCHAIN PIN_TOOLCHAIN OVERRIDE accept|refuse [EFFECTIVE]
	local desc=$1 gomod=$2 pin=$3 override=$4 expect=$5 want=${6:-}
	local w=$work/tc.$pass out status got effective
	rm -rf "$w"
	mkdir -p "$w/fork/bashy-build" "$w/src"
	printf 'module example.test\n\ngo %s\ntoolchain go%s\n' "${gomod%.*}" "$gomod" >"$w/src/go.mod"
	printf 'GO_TOOLCHAIN=%s\nGO_TOOLCHAIN_SECURITY_OVERRIDE=%s\n' "$pin" "$override" >"$w/fork/bashy-build/PIN"
	GITHUB_ENV=$w/github.env
	export GITHUB_ENV
	: >"$GITHUB_ENV"
	out=$(cd "$w" && bash "$work/step1.sh" 2>&1) && status=0 || status=$?
	got=refuse
	((status == 0)) && got=accept
	effective=$(sed -n 's/^GOTOOLCHAIN=go//p' "$GITHUB_ENV")
	if [[ $got != "$expect" ]]; then
		printf 'toolchain-gate FAIL  %s: expected %s, got %s\n%s\n' \
			"$desc" "$expect" "$got" "$(sed 's/^/    /' <<<"$out")" >&2
		rc=1
	elif [[ -n $want && $effective != "$want" ]]; then
		printf 'toolchain-gate FAIL  %s: effective go%s, expected go%s\n' \
			"$desc" "$effective" "$want" >&2
		rc=1
	fi
	pass=$((pass + 1))
}

directive_case() { # DESC EFFECTIVE GO_BEFORE GO_AFTER TC_BEFORE TC_AFTER ok|refused
	local desc=$1 effective=$2 gb=$3 ga=$4 tb=$5 ta=$6 expect=$7
	local w=$work/dir.$pass out got
	rm -rf "$w"
	mkdir -p "$w"
	printf 'go\t%s\ntoolchain\t%s\n' "$gb" "$tb" >"$w/gomod.before"
	printf 'go\t%s\ntoolchain\t%s\n' "$ga" "$ta" >"$w/gomod.after"
	# Only the directive half of block 2 is under test: everything before it
	# needs a real module and a real `go get`, which the fork CI exercises. The
	# half is delimited by BEGIN/END markers in setup.yml, extracted above.
	out=$(RUNNER_TEMP=$w GOTOOLCHAIN="go$effective" bash "$work/directives.sh" 2>&1) || true
	got=refused
	grep -q 'directives_ok=1' <<<"$out" && got=ok
	if [[ $got != "$expect" ]]; then
		printf 'directive-gate FAIL  %s: expected %s, got %s\n%s\n' \
			"$desc" "$expect" "$got" "$(sed 's/^/    /' <<<"$out")" >&2
		rc=1
	fi
	pass=$((pass + 1))
}

# --- the toolchain gate -----------------------------------------------------
toolchain_case "no override"                     1.27.1  1.27.1  ""      accept 1.27.1
toolchain_case "in-minor patch bump"             1.27.1  1.27.1  1.27.2  accept 1.27.2
toolchain_case "EOL minor -> oldest supported"   1.25.11 1.25.11 1.26.9  accept 1.26.9
toolchain_case "EOL minor -> newest supported"   1.25.11 1.25.11 1.27.2  refuse
toolchain_case "supported minor, jumped anyway"  1.26.6  1.26.6  1.27.2  refuse
toolchain_case "downgrade inside the minor"      1.27.2  1.27.2  1.27.1  refuse
toolchain_case "downgrade across minors"         1.27.1  1.27.1  1.26.9  refuse
toolchain_case "no-op override"                  1.27.2  1.27.2  1.27.2  refuse
toolchain_case "override without a patch"        1.27.1  1.27.1  1.27    refuse
toolchain_case "PIN disagrees with go.mod"       1.27.1  1.27.0  ""      refuse

# --- the go/toolchain directive gate ---------------------------------------
directive_case "unchanged"                   1.26.9  1.25.8 1.25.8   go1.25.11 go1.25.11 ok
directive_case "raised, toolchain dropped"   1.26.9  1.25.8 1.26.0   go1.25.11 ""        ok
directive_case "raised to the effective"     1.26.9  1.25.8 1.26.9   go1.25.11 ""        ok
directive_case "raised past the effective"   1.26.9  1.25.8 1.27.0   go1.25.11 ""        refused
directive_case "in-minor override caps it"   1.25.12 1.25.8 1.26.0   go1.25.11 ""        refused
directive_case "directive dropped"           1.26.9  1.26.0 1.25.8   ""        ""        refused
directive_case "toolchain raised past it"    1.26.9  1.25.8 1.25.8   go1.25.11 go1.27.2  refused
directive_case "toolchain lowered"           1.26.9  1.25.8 1.25.8   go1.25.11 go1.25.9  ok
directive_case "unparseable directive"       1.26.9  1.25.8 1.26-rc1 go1.25.11 ""        refused

((rc == 0)) &&
	echo "toolchain-gate-test: $pass cases, the go kind's toolchain and directive gates hold"
exit "$rc"
