#!/bin/sh
set -eu
repo=$(CDPATH= cd -P "$(dirname "$0")/.." && pwd)
want=$(printf 'shell\t--version\tworks\tbashy, GNU Bash <ver> compatible, version <ver>(1)-bashy-<ver> (<sha>)')
for build in 71e9426 71e94263dc64 v0.26.0-dev v0.26.0; do
  for version in dev v0.26.0; do
    got=$(printf 'shell\t--version\tworks\tbashy, GNU Bash 5.3 compatible, version 5.3.0(1)-bashy-%s (%s)\n' "$version" "$build" | sh "$repo/scripts/airgap-normalize-notes.sh")
    [ "$got" = "$want" ] || { printf 'FAIL build=%s version=%s\n%s\n' "$build" "$version" "$got" >&2; exit 1; }
  done
done
# A failure stays a failure; unrelated parenthesized versions remain versions.
want=$(printf 'verbs\tprobe\tFAIL\tunexpected (<ver>) on <arch>')
got=$(printf 'verbs\tprobe\tFAIL\tunexpected (v0.26.0-dev) on arm64\n' | sh "$repo/scripts/airgap-normalize-notes.sh")
[ "$got" = "$want" ] || { printf 'FAIL unrelated row: %s\n' "$got" >&2; exit 1; }
echo 'airgap-normalize-notes: PASS (8 build identities + unrelated failure row)'
