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
#   builds/_shared/$SHARED_FILES    shared helper scripts the tool declares
#
# Templates use @@VAR@@ (a tool.env variable or one of the values derived
# below; unknown names fail), @@MATRIX@@, @@INCLUDE name@@ (a fragment),
# #@@IF name@@ / #@@ELSE@@ / #@@END@@ (fragment present?) and
# #@@IFEQ VAR value@@ / #@@ELSE@@ / #@@END@@ (variable equals value?).
# See builds/README.md.
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
[[ $UPSTREAM_COMMIT =~ ^[0-9a-f]{40}$ ]] || die "UPSTREAM_COMMIT must be a full 40-hex sha"

# --- the DECLARED release-tag form ------------------------------------------
# Most upstreams tag vMAJOR.MINOR.PATCH, but not all: seaweedfs ships bare
# `4.48` and its last v-prefixed tag is v3.33. The shape is DECLARED, never
# sniffed from the pin, so a typo in a bumped tag is still refused.
#
# This case is the ONE place a form is mapped to its shape. The regex is
# substituted into the workflow's pin job AND into the shipped
# bashy-build/verify-pin.sh, so the two gates cannot drift apart. The bashy
# release tag is always bashy-<the pinned upstream tag>-N, which needs no form
# of its own: both gates compare the captured middle against UPSTREAM_TAG.
# shellcheck disable=SC2034  # read indirectly, as @@UPSTREAM_TAG_REGEX@@ etc.
SOURCE_ADMISSION=${SOURCE_ADMISSION:-official-release}
case $SOURCE_ADMISSION in
official-release) ;;
prima-first-party)
 [[ $TOOL == prima.cpp && $UPSTREAM_SLUG == qiangli/prima.cpp && $UPSTREAM_TAG_FORM == bashy-prima-X.Y.Z ]] || die "first-party exception is restricted to prima.cpp"
 ;;
*) die "unknown SOURCE_ADMISSION" ;;
esac
case ${UPSTREAM_TAG_FORM:=vX.Y.Z} in
bashy-prima-X.Y.Z)
 [[ $SOURCE_ADMISSION == prima-first-party ]] || die "prima tag requires explicit first-party admission"
 UPSTREAM_TAG_REGEX='^bashy-prima-[0-9]+\.[0-9]+\.[0-9]+$'
 BASHY_TAG_GLOB='bashy-bashy-prima-*'
 ;;
vX.Y.Z)
	UPSTREAM_TAG_REGEX='^v[0-9]+\.[0-9]+\.[0-9]+$'
	BASHY_TAG_GLOB='bashy-v*'
	;;
X.Y)
	UPSTREAM_TAG_REGEX='^[0-9]+\.[0-9]+$'
	BASHY_TAG_GLOB='bashy-[0-9]*'
	;;
*) die "UPSTREAM_TAG_FORM $UPSTREAM_TAG_FORM is not a declared release-tag form (vX.Y.Z, X.Y)" ;;
esac
[[ $UPSTREAM_TAG =~ $UPSTREAM_TAG_REGEX ]] ||
	die "UPSTREAM_TAG $UPSTREAM_TAG is not the declared $UPSTREAM_TAG_FORM release-tag form"

# --- the upstream forge -----------------------------------------------------
# A GitHub upstream is checked out with actions/checkout; anything else is
# cloned from UPSTREAM_REPO with plain git (act_runner's home is gitea.com).
# The forge is DERIVED from the URL rather than declared, so the two cannot
# disagree. The pin gate is identical either way: verify-pin.sh still demands
# that HEAD and the tag both be UPSTREAM_COMMIT over a pristine tree, so a
# second forge buys no weaker provenance.
[[ $UPSTREAM_REPO == https://* ]] || die "UPSTREAM_REPO must be an https:// URL"
[[ $UPSTREAM_REPO == */$UPSTREAM_SLUG ]] ||
	die "UPSTREAM_REPO $UPSTREAM_REPO does not end in UPSTREAM_SLUG $UPSTREAM_SLUG"
# shellcheck disable=SC2034  # read indirectly, as #@@IFEQ UPSTREAM_FORGE ...@@
if [[ $UPSTREAM_REPO == https://github.com/* ]]; then
	UPSTREAM_FORGE=github
	: "${UPSTREAM_API_BASE:=https://api.github.com}"
else
	UPSTREAM_FORGE=git
	# No default is possible off GitHub, and the pin job cannot check a release
	# it cannot query, so this is required rather than inferred. The endpoint
	# must answer /repos/SLUG/releases/tags/TAG with .draft and .prerelease
	# (gitea's /api/v1 does; it is the same REST shape).
	[[ -n ${UPSTREAM_API_BASE:-} ]] ||
		die "a non-GitHub UPSTREAM_REPO must declare UPSTREAM_API_BASE (the forge's release-API root)"
fi

# --- GO_SECURITY_OVERRIDES (the module half of a recorded override) ---------
# Syntax only; the workflow enforces the real gate (the go.mod diff). Checking
# it here means a typo fails at render time instead of in CI.
if [[ -n ${GO_SECURITY_OVERRIDES:-} ]]; then
	[[ $KIND == go ]] || die "GO_SECURITY_OVERRIDES is a go-kind key, but KIND=$KIND"
	for spec in $GO_SECURITY_OVERRIDES; do
		[[ $spec =~ ^[^@[:space:]]+@v[0-9][0-9A-Za-z.+-]*$ ]] ||
			die "GO_SECURITY_OVERRIDES entry $spec is not module@vVERSION"
	done
fi

# --- go-kind toolchain policy -----------------------------------------------
# Two facts about the Go RELEASE CYCLE rather than about any one upstream, so
# they are written down once, here, and substituted into the shared go
# fragments. Both are overridable from tool.env if a tool ever needs to differ.
if [[ $KIND == go ]]; then
	# GO_AUDIT_TOOLCHAIN builds the audit TOOLS (govulncheck, cyclonedx-gomod)
	# and nothing else. It is deliberately INDEPENDENT of the toolchain that
	# builds the artifact: govulncheck is a module with a `go` line of its own
	# (v1.9.0 declares `go 1.26.0`), so installing it under an older pinned
	# toolchain fails before any analysis happens — which is how qiangli/kopia
	# run 38326642419 died on kopia v0.23.1's go1.25.11. Keep it at the newest
	# patch of the newest supported minor. See _shared/kinds/go/audit.yml.
	: "${GO_AUDIT_TOOLCHAIN:=1.27.2}"
	[[ $GO_AUDIT_TOOLCHAIN =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] ||
		die "GO_AUDIT_TOOLCHAIN $GO_AUDIT_TOOLCHAIN is not MAJOR.MINOR.PATCH"
	# GO_SUPPORTED_MINORS: the Go minors upstream still ships security fixes
	# for, OLDEST FIRST. Go supports the two newest minors, so go1.27 shipping
	# retired 1.25 — it gets no further fixes at any patch level. The toolchain
	# gate reads this to tell an in-minor patch bump from an EOL-minor security
	# override (_shared/kinds/go/setup.yml); keep it current when Go releases a
	# minor, which is also when a pin on the retired minor needs the override.
	: "${GO_SUPPORTED_MINORS:=1.26 1.27}"
	for minor in $GO_SUPPORTED_MINORS; do
		[[ $minor =~ ^[0-9]+\.[0-9]+$ ]] ||
			die "GO_SUPPORTED_MINORS entry $minor is not MAJOR.MINOR"
	done
	# Syntax only, as above; setup.yml decides whether the move is sanctioned.
	if [[ -n ${GO_TOOLCHAIN_SECURITY_OVERRIDE:-} ]]; then
		[[ $GO_TOOLCHAIN_SECURITY_OVERRIDE =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] ||
			die "GO_TOOLCHAIN_SECURITY_OVERRIDE $GO_TOOLCHAIN_SECURITY_OVERRIDE is not MAJOR.MINOR.PATCH"
	fi
	# GO_AUDIT_TAGS: the build tags govulncheck and the SBOM analyze with. They
	# must be the tags the release is BUILT with, so that what the audit judges
	# is the artifact. A tool that builds with tags therefore has to declare
	# this, even to say "none" — the audit configuration is never left to a
	# default. The one sanctioned divergence is a tag that needs generated
	# sources the audit job does not produce (gitea's `bindata` embeds a vite
	# build), which changes which file supplies an asset, not which modules the
	# binary requires.
	if [[ -n ${GO_BUILD_TAGS:-} && -z ${GO_AUDIT_TAGS+x} ]]; then
		die "GO_BUILD_TAGS is set, so tool.env must declare GO_AUDIT_TAGS (what the audit analyzes with; empty is a decision, not a default)"
	fi
else
	for v in GO_TOOLCHAIN_SECURITY_OVERRIDE GO_AUDIT_TAGS; do
		[[ -z ${!v:-} ]] || die "$v is a go-kind key, but KIND=$KIND"
	done
fi
for f in otel.env no-egress-test.sh; do
	[[ -f $dir/files/$f ]] || die "files/$f is required (builds/README.md conventions)"
done
# SHARED_FILES: helper scripts copied from _shared/ into bashy-build/ next to
# the tool's own files, so one reviewed copy serves every tool that declares it.
for f in ${SHARED_FILES:-}; do
	if [[ $f == */* ]]; then die "SHARED_FILES entry $f must be a plain file name"; fi
	[[ -f $shared/$f ]] || die "SHARED_FILES names $f, which is not in builds/_shared"
	if [[ -f $dir/files/$f ]]; then die "files/$f shadows the shared builds/_shared/$f"; fi
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
	# Conditionals NEST, so the state is a stack, not a flag: one digit per
	# open #@@IF@@/#@@IFEQ@@ saying whether its current branch emits. A line
	# is emitted only when every open conditional does, so a false outer
	# branch cannot be re-opened by an inner #@@END@@ — with a single flag,
	# the forge conditional inside #@@IF audit@@ leaked the audit job's steps
	# into the build job of every tool without an audit fragment, and the
	# regenerate-and-diff gate could not see it (the output was still
	# deterministic, and still valid YAML).
	local line stack="" frag pair cond
	while IFS= read -r line || [[ -n $line ]]; do
		case $line in
		'#@@IFEQ '*'@@')
			# #@@IFEQ VAR value@@ — a derived/declared variable equals value.
			cond=${line#'#@@IFEQ '}
			cond=${cond%@@}
			frag=${cond%% *}
			[[ -n ${!frag+x} ]] || die "template references undeclared #@@IFEQ $frag@@"
			if [[ ${!frag} == "${cond#* }" ]]; then stack+="1"; else stack+="0"; fi
			continue
			;;
		'#@@IF '*'@@')
			frag=${line#'#@@IF '}
			frag=${frag%@@}
			if [[ -n $(fragment "$frag") ]]; then stack+="1"; else stack+="0"; fi
			continue
			;;
		'#@@ELSE@@')
			[[ -n $stack ]] || die "#@@ELSE@@ without an open conditional in ${1##*/}"
			stack=${stack%?}$((1 - ${stack: -1}))
			continue
			;;
		'#@@END@@')
			[[ -n $stack ]] || die "#@@END@@ without an open conditional in ${1##*/}"
			stack=${stack%?}
			continue
			;;
		esac
		[[ $stack == *0* ]] && continue
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
	[[ -z $stack ]] || die "unclosed #@@IF@@/#@@IFEQ@@ in ${1##*/}"
}

[[ -n $(fragment build) ]] || die "no build fragment: add $tool/build.yml or a builds/_shared/kinds/$KIND/build.yml"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/.github/workflows" "$tmp/bashy-build"
render "$shared/bashy-build.yml" >"$tmp/.github/workflows/bashy-build.yml"
cp -R "$dir/files/." "$tmp/bashy-build/"
cp "$dir/tool.env" "$tmp/bashy-build/PIN"
# verify-pin.sh is rendered, not copied: it carries @@UPSTREAM_TAG_REGEX@@ so
# the release-tag form has exactly one definition (the case above).
render "$shared/verify-pin.sh" >"$tmp/bashy-build/verify-pin.sh"
chmod +x "$tmp/bashy-build/verify-pin.sh"
for f in ${SHARED_FILES:-}; do
	cp "$shared/$f" "$tmp/bashy-build/$f"
	if [[ $f == *.sh ]]; then chmod +x "$tmp/bashy-build/$f"; fi
done

rm -rf "$out"
mkdir -p "$(dirname "$out")"
cp -R "$tmp" "$out"
echo "render: $tool -> $out"
