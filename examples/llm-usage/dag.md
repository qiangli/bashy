---
name: llm-usage
description: fixture unit checks for the usage-alarm skill-script (no live door)
type: dag
default: test
---

# llm-usage — alarm checks without a live door

Run from this directory with `bashy dag test`, or from the repo root with
`bashy dag examples/llm-usage test`.

## Tasks

### test
Unit-check `usage-alarm.bsh` against the two fixture documents, with the
committed `fixtures/fakebin/bashy` double on PATH recording `notify` calls
(the outer `bashy` runs via its absolute path so the double only shadows
the alarm's child calls). Needs `jq`; needs no model, no door, no network.
Effects: read, write, exec, destroy

```bash
set -e
here=$(pwd)
real_bashy=$(command -v bashy)
stub=$(mktemp -d "${TMPDIR:-/tmp}/llm-usage-test.XXXXXX")
export STUB_DIR="$stub"
: > "$stub/calls"

# Over-cap fixture: exit 1, all over-cap principals named, one notify call.
# peer:mixed-share is unpriced with unpriced_tokens under the token cap but
# cost_usd over the USD cap, so only its USD line fires.
if PATH="$here/fixtures/fakebin:$PATH" "$real_bashy" --bashsharp "$here/usage-alarm.bsh" \
  --limits "$here/thresholds.example.json" \
  --usage-file "$here/fixtures/usage-over.json" > "$stub/over.out" 2>&1; then
  echo "over-cap fixture exited 0, want 1" >&2; exit 1
fi
grep -q 'ALARM peer:field-laptop: tokens 2700000 > 2000000' "$stub/over.out"
grep -q 'ALARM peer:lab-share: usd 41.75 > 25.0' "$stub/over.out"
grep -q 'ALARM peer:mixed-share: usd 31.5 > 25.0' "$stub/over.out"
if grep -q 'ALARM peer:mixed-share: tokens' "$stub/over.out"; then
  echo "mixed principal fired the token cap, want USD only" >&2; exit 1
fi
grep -q 'notify.*llm usage alarm: over cap: peer:field-laptop peer:lab-share' "$stub/calls"
grep -q 'notify.*peer:mixed-share' "$stub/calls"

# Under-cap fixture: exit 0, no notify call.
: > "$stub/calls"
PATH="$here/fixtures/fakebin:$PATH" "$real_bashy" --bashsharp "$here/usage-alarm.bsh" \
  --limits "$here/thresholds.example.json" \
  --usage-file "$here/fixtures/usage-ok.json" > "$stub/ok.out" 2>&1
grep -q 'OK — 2 principal(s) inside caps' "$stub/ok.out"
[ -s "$stub/calls" ] && { echo "unexpected notify call" >&2; exit 1; }

# Bad schema: exit 2.
echo '{"schema":"nope"}' > "$stub/bad.json"
if PATH="$here/fixtures/fakebin:$PATH" "$real_bashy" --bashsharp "$here/usage-alarm.bsh" \
  --limits "$here/thresholds.example.json" \
  --usage-file "$stub/bad.json" --no-notify >/dev/null 2>&1; then
  echo "bad schema exited 0, want 2" >&2; exit 1
fi
rm -rf "$stub"
echo "test: ok"
```

### ping
Minimal live probe (manual): do a bashy build with the `llm usage` and `llm peer limit` verbs exist on this host?
Never fails the gate — it reports, so the example stays honest on builds
without them.
Effects: read, exec

```bash
bashy llm usage --help 2>&1 | head -5 || echo "ping: no 'bashy llm usage' verb on this build (needs a newer bashy build)"
bashy llm peer limit --help 2>&1 | head -5 || echo "ping: no 'bashy llm peer limit' verb on this build (needs a newer bashy build)"
echo "ping: done"
```
