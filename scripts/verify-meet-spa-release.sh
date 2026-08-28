#!/bin/sh
# GoReleaser post-build hook: start the native artifact and ask its HTTP server
# for the REAL room, failing before GoReleaser reaches its publish phase.
#
# This used to also assert the meetspa build tag. The tag is gone — the room is
# compiled in unconditionally from pkg/meet/artifact — so the only question left
# is the one that always mattered: does the shipped binary actually serve the UI?
# A tracked artifact directory can go stale (a source change with no SPA rebuild
# ships yesterday's bundle), which is exactly the failure a runtime check sees
# and a tag check never could.
set -eu

if [ "$#" -ne 2 ]; then
	echo "usage: $0 BINARY TARGET" >&2
	exit 2
fi

binary=$1
target=$2

# Developer builds on Linux and macOS put the Go program beside a tiny native
# signal-forwarding launcher as BINARY.real; GoReleaser artifacts are direct Go
# executables. Smoke whichever form is actually runnable here.
artifact=$binary
if [ ! -x "$artifact" ] && [ -x "$binary.real" ]; then
	artifact=$binary.real
fi

# Only the native target can be executed on this machine.
case "$target" in
*"$(go env GOOS)"*"$(go env GOARCH)"*) ;;
*)
	echo "meet SPA release gate: $target is not native here; skipping runtime smoke"
	exit 0
	;;
esac

state_dir=$(mktemp -d "${TMPDIR:-/tmp}/bashy-meet-spa.XXXXXX")
port=${BASHY_MEET_SMOKE_PORT:-18641}

cleanup() {
	BASHY_MEET_DIR="$state_dir" "$artifact" meet service stop --port "$port" >/dev/null 2>&1 || true
	rm -rf "$state_dir"
}
trap cleanup EXIT HUP INT TERM

BASHY_MEET_DIR="$state_dir" "$artifact" meet service start --port "$port" >/dev/null

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
