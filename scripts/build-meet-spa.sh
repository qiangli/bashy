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

pnpm_kind=
if command -v node >/dev/null 2>&1; then
	if command -v pnpm >/dev/null 2>&1; then
		pnpm_kind=direct
	elif command -v corepack >/dev/null 2>&1 &&
		corepack pnpm --version >/dev/null 2>&1; then
		pnpm_kind=corepack
	fi
fi

if [ -z "$pnpm_kind" ]; then
	if [ "$mode" = required ]; then
		echo "meet SPA: release build requires node and pnpm (or corepack), but they are unavailable" >&2
		exit 1
	fi
	echo "meet SPA: node/pnpm unavailable; building the honest no-UI bashy binary" >&2
	exit 0
fi

run_pnpm() {
	if [ "$pnpm_kind" = direct ]; then
		pnpm "$@"
	else
		corepack pnpm "$@"
	fi
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

printf '%s\n' meetspa
