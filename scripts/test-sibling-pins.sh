#!/bin/sh
# Fail closed when a direct ../sibling replacement cannot be reproduced from
# .sibling-pins. This is structural and offline; the clean-bootstrap gate also
# proves the recorded commits are origin-reachable.
set -eu

root=$(CDPATH= cd -P "$(dirname "$0")/.." && pwd)
pins=$root/.sibling-pins
bootstrap=$root/scripts/bootstrap-siblings.sh
dag=$root/dag.md

fail() {
    echo "test-sibling-pins: $*" >&2
    exit 1
}

[ -f "$pins" ] || fail "missing .sibling-pins"

tmp=$(mktemp -d "${TMPDIR:-/tmp}/bashy-sibling-pins.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

sed -n 's|^[[:space:]]*replace[[:space:]].*=>[[:space:]]*\.\./\([^/[:space:]]*\).*$|\1|p' \
    "$root/go.mod" | LC_ALL=C sort -u >"$tmp/replaces"

: >"$tmp/pins"
while IFS= read -r line; do
    case "$line" in
        ''|'#'*) continue ;;
    esac
    name=${line%%=*}
    sha=${line#*=}
    [ -n "$name" ] && [ "$name" != "$sha" ] || fail "malformed pin: $line"
    [ "${#sha}" -eq 40 ] || fail "$name pin is not a full 40-character SHA"
    case "$sha" in *[!0-9a-f]*) fail "$name pin is not lowercase hexadecimal" ;; esac
    printf '%s\n' "$name" >>"$tmp/pins"

    grep -Fq "$name) echo \"https://github.com/qiangli/$name.git\" ;;" "$bootstrap" ||
        fail "$name has no bootstrap repository mapping"
    grep -Fq "$name) url=https://github.com/qiangli/$name.git ;;" "$dag" ||
        fail "$name has no fleet-prepare repository mapping"
done <"$pins"

LC_ALL=C sort "$tmp/pins" >"$tmp/pins.sorted"
[ "$(wc -l <"$tmp/pins")" -eq "$(wc -l <"$tmp/pins.sorted" | tr -d ' ')" ] ||
    fail "internal pin count error"
[ "$(uniq "$tmp/pins.sorted" | wc -l | tr -d ' ')" -eq "$(wc -l <"$tmp/pins.sorted" | tr -d ' ')" ] ||
    fail "duplicate sibling pin"

cmp -s "$tmp/replaces" "$tmp/pins.sorted" || {
    echo "test-sibling-pins: direct flat replacements and pins differ:" >&2
    diff -u "$tmp/replaces" "$tmp/pins.sorted" >&2 || true
    exit 1
}

echo "test-sibling-pins: PASS"
