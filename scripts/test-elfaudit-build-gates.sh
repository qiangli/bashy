#!/usr/bin/env bash
# Four cold cross-builds must not share one package's ten-minute test clock.
# Keep each existing build/audit assertion bounded by that same clock.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
if [ -n "${EVIDENCE_DIR:-}" ]; then
  logs=$(mktemp -d "$EVIDENCE_DIR/elfaudit-build-gates.XXXXXX")
else
  logs=$(mktemp -d "${TMPDIR:-/tmp}/elfaudit-build-gates.XXXXXX")
fi
printf 'elfaudit build-gate evidence: %s\n' "$logs"
top='^(TestLinuxBashySignalSymbolBuildGate|TestDarwinBashySignalSnapshotBuildGate)$'
if go test -list "$top" ./tools/elfaudit >"$logs/discovery.log" 2>&1; then
  cat "$logs/discovery.log"
else
  rc=$?; cat "$logs/discovery.log"; exit "$rc"
fi
python3 - "$logs/discovery.log" <<'PY'
import pathlib, sys
names = [s for s in pathlib.Path(sys.argv[1]).read_text().splitlines() if s.startswith("Test")]
expected = ["TestLinuxBashySignalSymbolBuildGate", "TestDarwinBashySignalSnapshotBuildGate"]
if sorted(names) != sorted(expected):
    raise SystemExit(f"elfaudit build-gate discovery mismatch: {names!r}")
PY
for selected in \
  TestLinuxBashySignalSymbolBuildGate/retained \
  TestLinuxBashySignalSymbolBuildGate/stripped \
  TestDarwinBashySignalSnapshotBuildGate/product \
  TestDarwinBashySignalSnapshotBuildGate/plain; do
  parent=${selected%/*}; child=${selected#*/}
  log="$logs/$parent-$child.jsonl"
  printf 'elfaudit build gate: %s (timeout=10m)\n' "$selected"
  if go test -json -count=1 -timeout=10m -run "^$parent\$/^$child\$" ./tools/elfaudit >"$log" 2>&1; then
    cat "$log"
  else
    rc=$?; cat "$log"; exit "$rc"
  fi
  # A zero exit with no matching test, a skip, or a renamed child is not proof.
  python3 - "$log" "$selected" <<'PY'
import json, pathlib, sys
events = [json.loads(line) for line in pathlib.Path(sys.argv[1]).read_text().splitlines() if line.strip()]
selected = sys.argv[2]
children = [(e.get("Test"), e.get("Action")) for e in events
            if "/" in e.get("Test", "") and e.get("Action") in ("pass", "fail", "skip")]
if children != [(selected, "pass")]:
    raise SystemExit(f"elfaudit build-gate coverage mismatch: expected {selected!r}, got {children!r}")
if not any(e.get("Action") == "pass" and not e.get("Test") for e in events):
    raise SystemExit("elfaudit build-gate package did not pass")
PY
done
printf 'elfaudit build gates: PASS (4/4 existing subtests)\n'
