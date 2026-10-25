#!/bin/sh
# Sprint 407 / Story 1949: detect expensive package init in every bashy applet
# and job carrier, then exercise the carrier path at certification scale.
set -eu

if [ "$(uname -s)" != Linux ]; then
    echo 'test-startup-linux: skipped (Linux only)'
    exit 0
fi

bin=${1:-bin/bashy}
case "$bin" in
    /*) ;;
    *) bin="$(pwd)/$bin" ;;
esac
if [ ! -x "$bin" ]; then
    echo "test-startup-linux: missing executable: $bin" >&2
    exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
ln -s "$bin" "$tmp/sh"

GODEBUG=inittrace=1 "$bin" -c true >/dev/null 2>"$tmp/inittrace"
awk -F ', ' '
    /^init / {
        split($2, clock, " ")
        cost = clock[1] + 0
        total += cost
        count++
        if (cost > max) {
            max = cost
            name = $1
            sub(/^init /, "", name)
            sub(/ @.*/, "", name)
        }
    }
    END {
        printf "inittrace: %.2f ms total (%d packages); max %.2f ms (%s)\n", total, count, max, name
        if (count == 0 || max > 5 || total > 15) exit 1
    }
' "$tmp/inittrace"

start=$(date +%s%N)
i=0
while [ "$i" -lt 20 ]; do
    "$bin" -c true >/dev/null
    i=$((i + 1))
done
end=$(date +%s%N)
awk -v elapsed="$((end - start))" 'BEGIN { printf "bashy -c true: %.2f ms/process (20 runs)\n", elapsed / 20000000 }'

start=$(date +%s%N)
"$tmp/sh" -c 'i=0; while ((i < 100)); do /bin/true & wait $! || exit; ((i++)); done'
end=$(date +%s%N)
awk -v elapsed="$((end - start))" 'BEGIN {
    seconds = elapsed / 1000000000
    printf "100x /bin/true & wait $!: %.2f s\n", seconds
    if (seconds > 5) exit 1
}'
