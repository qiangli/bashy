#!/usr/bin/env bash
# Outpost is compiled beside bashy, so their shared replace targets must agree.
set -euo pipefail
cd "$(dirname "$0")/.."
for name in sh coreutils yoke filebrowser; do
  bashy_pin=$(sed -n "s/^${name}=//p" .sibling-pins)
  outpost_pin=$(sed -n "s/^${name}=//p" ../outpost/.sibling-pins)
  if [[ ! $bashy_pin =~ ^[0-9a-f]{40}$ || $bashy_pin != "$outpost_pin" ]]; then
    echo "shared sibling pin mismatch: $name (bashy=$bashy_pin outpost=$outpost_pin)" >&2
    exit 1
  fi
done
echo 'shared sibling pins: PASS'
