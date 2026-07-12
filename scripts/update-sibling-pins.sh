#!/usr/bin/env bash
# Resync .sibling-pins to the current HEAD of each sibling on disk.
#
# The counterpart to scripts/hooks/pre-push: the hook refuses a push while a pin
# disagrees with its sibling's HEAD, and this rewrites the pins so it agrees.
# Comments and line order are preserved; a sibling that isn't checked out next
# to this repo keeps its existing pin (a standalone clone has none to read).
#
# Idempotent. Safe to re-run.
set -euo pipefail

cd "$(dirname "$0")/.."
pins=.sibling-pins

if [ ! -f "$pins" ]; then
    echo "update-sibling-pins: missing $pins" >&2
    exit 1
fi

tmp=$(mktemp "${TMPDIR:-/tmp}/sibling-pins.XXXXXX")
trap 'rm -f "$tmp"' EXIT

changed=0
while IFS= read -r line; do
    case "$line" in
        '' | '#'*)
            printf '%s\n' "$line" >>"$tmp"
            continue
            ;;
    esac

    name=${line%%=*}
    pinned=${line#*=}
    pinned=$(printf %s "$pinned" | tr -d '[:space:]')

    dir=../$name
    if [ ! -e "$dir/.git" ]; then
        echo "update-sibling-pins: $name not checked out at $dir — keeping ${pinned:0:12}" >&2
        printf '%s\n' "$line" >>"$tmp"
        continue
    fi

    actual=$(git -C "$dir" rev-parse HEAD)
    printf '%s=%s\n' "$name" "$actual" >>"$tmp"

    if [ "$pinned" = "$actual" ]; then
        echo "update-sibling-pins: $name ${actual:0:12} (unchanged)"
    else
        echo "update-sibling-pins: $name ${pinned:0:12} -> ${actual:0:12}"
        changed=1
    fi
done <"$pins"

cat "$tmp" >"$pins"

if [ "$changed" -eq 0 ]; then
    echo "update-sibling-pins: already in sync"
    exit 0
fi

cat <<'EOF'

Pins updated. Commit them alongside the change that needs them:

  git commit -am 'chore: bump .sibling-pins'

Push each bumped sibling to its own origin first — CI clones the pin from
GitHub, so a SHA that exists only on this machine fails there.
EOF
