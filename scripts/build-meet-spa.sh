#!/bin/sh
# Build the meet SPA in the pinned coreutils sibling and print the Go build tag
# on stdout when it is safe to embed. Diagnostics go to stderr so callers can
# use the output directly in a tag list.
set -eu

mode=${1:-optional}
web_dir=../coreutils/pkg/meet/web

case "$mode" in
	optional|required) ;;
	*)
		echo "usage: $0 [optional|required]" >&2
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
	if [ "$mode" = required ]; then
		echo "meet SPA: release build requires node and pnpm (or corepack, or a working 'bashy pnpm'), but none is available" >&2
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

# Promote the freshly built bundle into the TRACKED artifact directory that
# pkg/meet embeds. dist/ is the SPA's own scratch and stays ignored; artifact/
# is "these bytes ship", and moving them is a deliberate step that shows up in a
# diff rather than a side effect of whoever last ran vite.
artifact_dir=$(cd "$web_dir/.." && pwd)/artifact
rm -rf "$artifact_dir"
mkdir -p "$artifact_dir"
cp -R "$web_dir/dist/." "$artifact_dir/"
echo "meet SPA: promoted $web_dir/dist -> $artifact_dir" >&2
