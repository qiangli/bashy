#!/bin/sh
# Build the meet SPA in the pinned coreutils sibling. Diagnostics go to stderr
# so callers can use any stdout directly.
#
# Modes:
#   optional  build the SPA and PROMOTE dist -> tracked artifact/ when a
#             toolchain is present; degrade to a no-UI build (exit 0) when it is
#             not. The default; used by the ordinary `make build`.
#   required  same promotion, but a missing toolchain is a hard failure — the
#             release path, where a no-UI binary must never ship.
#   check     NON-MUTATING freshness gate. Build a fresh dist and compare it
#             byte-for-byte / file-set against the TRACKED artifact/, WITHOUT
#             touching artifact/. Exit non-zero with an actionable diagnostic
#             when they disagree (a source change shipped without a SPA rebuild)
#             or when the toolchain is missing (fail closed — an unbuildable
#             check proves nothing, so it must not pass). This is the gate that
#             catches a stale tracked bundle; `make test` runs it before any
#             recipe that could silently repair the artifact.
#   compare DIST ARTIFACT
#             the pure comparison primitive underneath `check`: diff two trees
#             and report, with no build, no toolchain and no mutation. Exposed
#             so the freshness gate can be regression-tested without a real
#             SPA build.
set -eu

mode=${1:-optional}
# Overridable ONLY for the hermetic regression test; the default is the pinned
# sibling and is what every real build uses.
web_dir=${MEET_SPA_WEB_DIR:-../coreutils/pkg/meet/web}

# compare_trees FRESH_DIST TRACKED_ARTIFACT
# Read-only. Return 0 when the tracked artifact is byte-for-byte and file-set
# identical to a fresh dist, non-zero otherwise. Never writes to either tree.
compare_trees() {
	fresh=$1
	tracked=$2
	if [ ! -d "$fresh" ]; then
		echo "meet SPA freshness: fresh dist dir missing: $fresh" >&2
		return 1
	fi
	if [ ! -d "$tracked" ]; then
		echo "meet SPA freshness: tracked artifact dir missing: $tracked" >&2
		return 1
	fi
	# diff -r reports BOTH divergences a stale bundle can have: "Only in ..."
	# for a file-set mismatch and "Files ... differ" for a byte mismatch. -q
	# keeps the report to one actionable line per file.
	if diff_out=$(diff -rq "$fresh" "$tracked" 2>&1); then
		echo "meet SPA freshness: tracked artifact matches a fresh build" >&2
		return 0
	fi
	echo "meet SPA freshness: STALE — the tracked artifact disagrees with a fresh SPA build:" >&2
	printf '%s\n' "$diff_out" | sed 's/^/meet SPA freshness:   /' >&2
	echo "meet SPA freshness: rebuild and commit the bundle, then re-run:" >&2
	echo "meet SPA freshness:   scripts/build-meet-spa.sh required   # rebuilds and promotes dist -> artifact" >&2
	echo "meet SPA freshness:   git -C ../coreutils add pkg/meet/artifact && commit it" >&2
	return 1
}

# Pure primitive: compare two given trees and stop.
if [ "$mode" = compare ]; then
	if [ "$#" -ne 3 ]; then
		echo "usage: $0 compare DIST_DIR ARTIFACT_DIR" >&2
		exit 2
	fi
	compare_trees "$2" "$3"
	exit $?
fi

case "$mode" in
	optional|required|check) ;;
	*)
		echo "usage: $0 [optional|required|check] | $0 compare DIST ARTIFACT" >&2
		exit 2
		;;
esac

if [ ! -f "$web_dir/package.json" ] || [ ! -f "$web_dir/pnpm-lock.yaml" ]; then
	echo "meet SPA: missing $web_dir package.json or pnpm-lock.yaml" >&2
	exit 1
fi

# BASHY BUILDS BASHY. The tiers below are ordered so a host with no system Node
# still produces a COMPLETE binary, because the third tier provisions its own.
#
# The failure this exists to prevent: on a machine with no node/pnpm the optional
# mode printed one line and produced a UI-LESS binary that is otherwise
# indistinguishable from a good one — same name, same verbs, 1 MB smaller. A
# remote host built exactly that and it was caught only by diffing sizes against
# a local build. A silent downgrade of the product is worse than a failed build.
#
#   direct   a system pnpm on PATH
#   corepack a system node's corepack (honours package.json packageManager)
#   bashy    `bashy pnpm` — binmgr provisions a pinned Node tree and runs corepack
#            out of it, so no system Node is required at all. This is the
#            self-hosting tier: bashy provisioning the toolchain that builds bashy.
pnpm_kind=
if command -v node >/dev/null 2>&1; then
	if command -v pnpm >/dev/null 2>&1; then
		pnpm_kind=direct
	elif command -v corepack >/dev/null 2>&1 &&
		corepack pnpm --version >/dev/null 2>&1; then
		pnpm_kind=corepack
	fi
fi

# Third tier. Probed INSIDE $web_dir because corepack resolves the pnpm named by
# that directory's package.json `packageManager` field — probing from elsewhere
# resolves LATEST pnpm instead, which has a different Node floor and gives a
# false verdict either way. (Measured: the same probe passed from the project dir
# and failed from $HOME.)
bashy_bin=${BASHY_BIN:-}
if [ -z "$pnpm_kind" ]; then
	if [ -z "$bashy_bin" ] && command -v bashy >/dev/null 2>&1; then
		bashy_bin=$(command -v bashy)
	fi
	if [ -n "$bashy_bin" ] && [ -x "$bashy_bin" ]; then
		if (cd "$web_dir" && "$bashy_bin" pnpm --version >/dev/null 2>&1); then
			pnpm_kind=bashy
			echo "meet SPA: using bashy-provisioned pnpm ($bashy_bin) — no system Node needed" >&2
		else
			echo "meet SPA: $bashy_bin pnpm is present but not usable here; run '$bashy_bin pnpm --version' in $web_dir to see why" >&2
		fi
	fi
fi

if [ -z "$pnpm_kind" ]; then
	# The freshness gate FAILS CLOSED, exactly like a release build: a check that
	# cannot build a fresh bundle cannot prove the tracked one is current, and a
	# green "no toolchain" pass is precisely how a stale artifact would slip
	# through the gate meant to catch it.
	if [ "$mode" = required ] || [ "$mode" = check ]; then
		echo "meet SPA: $mode requires node and pnpm (or corepack, or a working 'bashy pnpm'), but none is available" >&2
		exit 1
	fi
	echo "meet SPA: node/pnpm unavailable AND no usable 'bashy pnpm'; building the no-UI bashy binary" >&2
	echo "meet SPA: THE RESULT HAS NO WEB CONSOLE UI. Install node/pnpm, or put a working bashy on PATH (or set BASHY_BIN), then rebuild." >&2
	exit 0
fi

run_pnpm() {
	case "$pnpm_kind" in
	direct) pnpm "$@" ;;
	bashy) "$bashy_bin" pnpm "$@" ;;
	*) corepack pnpm "$@" ;;
	esac
}

echo "meet SPA: installing locked dependencies and building $web_dir/dist" >&2
(
	cd "$web_dir"
	run_pnpm install --frozen-lockfile >&2
	run_pnpm build >&2
)

if [ ! -s "$web_dir/dist/index.html" ] ||
	! grep -Eiq '<(html|head)([[:space:]>])' "$web_dir/dist/index.html"; then
	echo "meet SPA: build completed without a usable dist/index.html" >&2
	exit 1
fi

artifact_dir=$(cd "$web_dir/.." && pwd)/artifact

if [ "$mode" = check ]; then
	# NON-MUTATING gate: compare the fresh dist against the tracked artifact and
	# report. Do NOT promote — a check that silently repaired the artifact would
	# hide the very drift it exists to surface (and this runs from `make test`,
	# where a mutation would rewrite someone's tree from under them).
	echo "meet SPA: freshness check — comparing fresh $web_dir/dist against tracked $artifact_dir (no promotion)" >&2
	compare_trees "$web_dir/dist" "$artifact_dir"
	exit $?
fi

# Promote the freshly built bundle into the TRACKED artifact directory that
# pkg/meet embeds. dist/ is the SPA's own scratch and stays ignored; artifact/
# is "these bytes ship", and moving them is a deliberate step that shows up in a
# diff rather than a side effect of whoever last ran vite.
rm -rf "$artifact_dir"
mkdir -p "$artifact_dir"
cp -R "$web_dir/dist/." "$artifact_dir/"
echo "meet SPA: promoted $web_dir/dist -> $artifact_dir" >&2
