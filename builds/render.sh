#!/usr/bin/env bash
# render.sh TOOL [OUT_DIR]
#
# Renders builds/TOOL/fork (or OUT_DIR) — the tree installed as the orphan
# `bashy-build` branch of github.com/qiangli/TOOL — from:
#
#   builds/TOOL/tool.env          the declaration; shipped verbatim as bashy-build/PIN
#   builds/TOOL/files/            tool-specific files, copied into bashy-build/
#   builds/TOOL/{setup,build,audit}.yml
#                                 optional workflow step fragments; each one
#                                 missing falls back to builds/_shared/kinds/$KIND/
#   builds/_shared/bashy-build.yml  the workflow skeleton (pin/build/audit/release)
#   builds/_shared/verify-pin.sh    the shared provenance gate
#
# Templates use @@VAR@@ (a tool.env variable; unknown names fail), @@MATRIX@@,
# @@INCLUDE name@@ (a fragment) and #@@IF name@@ / #@@ELSE@@ / #@@END@@
# (fragment present?). See builds/README.md.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
tool=${1:?usage: render.sh TOOL [OUT_DIR]}
dir=$here/$tool
out=${2:-$dir/fork}
shared=$here/_shared

die() { echo "render: $tool: $*" >&2; exit 1; }

[[ -f $dir/tool.env ]] || die "missing $dir/tool.env"
set +u
set -a
# shellcheck source=/dev/null
. "$dir/tool.env"
set +a
set -u

# --- policy checks on the declaration (fail before writing anything) --------
for v in TOOL FORK_REPO UPSTREAM_SLUG UPSTREAM_TAG UPSTREAM_COMMIT KIND COSIGN_VERSION MATRIX EGRESS_TARGET RELEASE_ASSETS; do
	[[ -n ${!v:-} ]] || die "tool.env must set $v"
done
[[ $TOOL == "$tool" ]] || die "TOOL=$TOOL does not match directory $tool"
[[ $FORK_REPO == "qiangli/$tool" ]] || die "FORK_REPO must be qiangli/$tool"
[[ $UPSTREAM_TAG =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "UPSTREAM_TAG $UPSTREAM_TAG is not a plain release tag"
[[ $UPSTREAM_COMMIT =~ ^[0-9a-f]{40}$ ]] || die "UPSTREAM_COMMIT must be a full 40-hex sha"
for f in otel.env no-egress-test.sh; do
	[[ -f $dir/files/$f ]] || die "files/$f is required (builds/README.md conventions)"
done
# Rule 2: OTel goes to bashy's local collector, never anywhere else.
grep -qx 'OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:31415' "$dir/files/otel.env" ||
	die "files/otel.env must set OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:31415"
if grep -E '^[A-Z_]*ENDPOINT=' "$dir/files/otel.env" | grep -Ev '=https?://127\.0\.0\.1[:/]' | grep -q .; then
	die "files/otel.env has a non-loopback endpoint"
fi

fragment() { # NAME -> path of the fragment file, or nothing
	if [[ -f $dir/$1.yml ]]; then
		echo "$dir/$1.yml"
	elif [[ -f $shared/kinds/$KIND/$1.yml ]]; then
		echo "$shared/kinds/$KIND/$1.yml"
	fi
}

subst() { # LINE -> LINE with every @@VAR@@ replaced
	local line=$1 res="" name
	while [[ $line =~ @@([A-Z_][A-Z0-9_]*)@@ ]]; do
		name=${BASH_REMATCH[1]}
		[[ -n ${!name+x} ]] || die "template references undeclared @@$name@@"
		res+=${line%%"@@$name@@"*}${!name}
		line=${line#*"@@$name@@"}
	done
	printf '%s\n' "$res$line"
}

render() { # TEMPLATE
	local line skip=0 frag pair
	while IFS= read -r line || [[ -n $line ]]; do
		case $line in
		'#@@IF '*'@@')
			frag=${line#'#@@IF '}
			frag=${frag%@@}
			[[ -n $(fragment "$frag") ]] && skip=0 || skip=1
			continue
			;;
		'#@@ELSE@@') skip=$((1 - skip)); continue ;;
		'#@@END@@') skip=0; continue ;;
		esac
		((skip)) && continue
		case $line in
		'@@MATRIX@@')
			for pair in $MATRIX; do
				[[ $pair == *=* ]] || die "MATRIX entry $pair must be ASSET=RUNNER"
				printf '          - asset: %s\n            runner: %s\n' "${pair%%=*}" "${pair#*=}"
			done
			;;
		'@@INCLUDE '*'@@')
			frag=${line#'@@INCLUDE '}
			frag=$(fragment "${frag%@@}")
			[[ -z $frag ]] || render "$frag"
			;;
		*) subst "$line" ;;
		esac
	done <"$1"
}

[[ -n $(fragment build) ]] || die "no build fragment: add $tool/build.yml or a builds/_shared/kinds/$KIND/build.yml"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/.github/workflows" "$tmp/bashy-build"
render "$shared/bashy-build.yml" >"$tmp/.github/workflows/bashy-build.yml"
cp -R "$dir/files/." "$tmp/bashy-build/"
cp "$dir/tool.env" "$tmp/bashy-build/PIN"
cp "$shared/verify-pin.sh" "$tmp/bashy-build/verify-pin.sh"
chmod +x "$tmp/bashy-build/verify-pin.sh"

rm -rf "$out"
mkdir -p "$(dirname "$out")"
cp -R "$tmp" "$out"
echo "render: $tool -> $out"
