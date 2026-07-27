#!/bin/sh
# GoReleaser post-build hook. It asks every actual bashy artifact for its build
# tags, then starts the native Linux artifact and asks its HTTP server for the
# real SPA. Any failure happens before GoReleaser reaches its publish phase.
set -eu

if [ "$#" -ne 2 ]; then
	echo "usage: $0 BINARY TARGET" >&2
	exit 2
fi

binary=$1
target=$2

if ! go version -m "$binary" 2>&1 |
	grep -E -- '-tags=([^[:space:]]*,)?meetspa(,|[[:space:]]|$)' >/dev/null; then
	echo "meet SPA release gate: $target artifact lacks the meetspa build tag" >&2
	exit 1
fi

host_target=$(go env GOOS)_$(go env GOARCH)
case "$target" in
	"$host_target"*)
		;;
	*)
		echo "meet SPA release gate: $target artifact records -tags=meetspa"
		exit 0
		;;
esac

state_dir=$(mktemp -d "${TMPDIR:-/tmp}/bashy-meet-spa.XXXXXX")
port=${BASHY_MEET_SMOKE_PORT:-18641}

cleanup() {
	BASHY_MEET_DIR="$state_dir" "$binary" meet service stop --port "$port" >/dev/null 2>&1 || true
	rm -rf "$state_dir"
}
trap cleanup EXIT HUP INT TERM

BASHY_MEET_DIR="$state_dir" "$binary" meet service start --port "$port" >/dev/null

body=$state_dir/index.html
ready=false
i=0
while [ "$i" -lt 50 ]; do
	if curl -fsS "http://127.0.0.1:$port/" >"$body" 2>/dev/null; then
		ready=true
		break
	fi
	i=$((i + 1))
	sleep 0.1
done

if [ "$ready" != true ]; then
	echo "meet SPA release gate: $target server did not become ready" >&2
	exit 1
fi
if grep -Fq 'built without the web room' "$body"; then
	echo "meet SPA release gate: $target served the no-UI fallback" >&2
	exit 1
fi
if ! grep -Eiq '<base[[:space:]][^>]*href="/"' "$body"; then
	echo "meet SPA release gate: $target did not serve an injected <base href=\"/\"> tag" >&2
	exit 1
fi

echo "meet SPA release gate: $target served the embedded UI"
head -c 200 "$body"
printf '\n'
