#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
checker=$script_dir/check-seed-bands.sh
tmp=${TMPDIR:-/tmp}/check-seed-bands.$$
trap 'rm -rf "$tmp"' 0 HUP INT TERM
mkdir -p "$tmp/repo" "$tmp/yoke/pkg/fleet/baseline/models"
git -C "$tmp/repo" init -q
git -C "$tmp/repo" config user.name test
git -C "$tmp/repo" config user.email test@example.invalid
git -C "$tmp/repo" commit --allow-empty -qm init
(
  cd "$tmp/repo"
  GIT_COMMITTER_DATE='2025-01-01T00:00:00Z' git tag -a v1.0.0 -m v1.0.0
)

model=$tmp/yoke/pkg/fleet/baseline/models/model.yaml
stamp=$tmp/yoke/pkg/fleet/baseline/seed-bands.txt
write_stamp() {
  cat > "$stamp" <<EOF
# release seed stamp
date: $1
models: $2
source: test fixture
EOF
}
write_seed() {
  cat > "$model" <<EOF
name: agent-a
band: L2
band_source: seeded
notes: seeded $1
EOF
}
run() { (cd "$tmp/repo" && sh "$checker" "$@"); }
expect_ok() {
  if ! run "$@"; then echo "expected success: $*" >&2; exit 1; fi
}
expect_fail() {
  if run "$@" >"$tmp/out" 2>&1; then echo "expected failure: $*" >&2; exit 1; fi
}

write_seed 2025-01-02
write_stamp 2025-01-02 1
expect_ok v2.0.0 --yoke "$tmp/yoke"
write_seed 2024-12-31
write_stamp 2024-12-31 1
expect_fail v2.0.0 --yoke "$tmp/yoke"
write_seed 2025-01-02
write_stamp 2025-01-02 2
run v1.1.0 --yoke "$tmp/yoke" >"$tmp/out" 2>&1
grep -q 'warning: stamp says 2 models but found 1 seeded model' "$tmp/out"
expect_ok v1.1.0 --yoke "$tmp/yoke"
write_stamp 2024-12-31 1
expect_fail v1.1.0 --yoke "$tmp/yoke" --reseed
rm -f "$model"
expect_fail v2.0.0 --yoke "$tmp/yoke"
grep -q 'no seeded models found' "$tmp/out"
expect_fail v1.1.0 --yoke "$tmp/yoke"
grep -q 'no seeded models found' "$tmp/out"
if grep -q 'major release' "$tmp/out"; then echo 'minor failure mislabeled as major release' >&2; exit 1; fi
write_seed 2025-01-02
rm -f "$stamp"
expect_fail v1.1.0 --yoke "$tmp/yoke"
write_stamp invalid 1
expect_fail v1.1.0 --yoke "$tmp/yoke"

echo 'check-seed-bands tests passed'
