#!/usr/bin/env bash
# Container-only half of uutils-scoreboard.sh. Do not invoke on the host.
set -u

test -f /opt/bashy-uutils-image-v1 || {
  echo "image does not satisfy the bashy uutils v1 offline-cache contract" > /out/run.txt
  exit 2
}
mkdir -p /work/uutils
tar -xf /input/uutils.tar -C /work/uutils
cd /work/uutils
export HOME=/work/home
export CARGO_HOME=/work/cargo
export TMPDIR=/tmp
mkdir -p "$HOME" "$CARGO_HOME"
cp -a /opt/bashy-cargo-cache/. "$CARGO_HOME"/

# Permanent quarantine. The supported path intentionally has no override:
# containment limits blast radius, but it does not make known infinite-device
# and recursive-root cases useful certification signals.
DANGEROUS_SKIPS=(
  --skip test_split::test_dev_zero
  --skip test_split::test_number_by_bytes_dev_zero
  --skip test_sort::test_verifies_input_files
  --skip test_chgrp::test_preserve_root
  --skip test_chgrp::test_preserve_root_symlink
  --skip test_chgrp::test_preserve_root_symlink_cwd_root
  --skip test_chmod::test_chmod_preserve_root_with_paths_that_resolve_to_root
  --skip test_dd::test_random_73k_test_lazy_fullblock
)

LIST=/out/list.txt
set +e
UUTESTS_BINARY_PATH=/input/coreutils \
  cargo test --offline --features unix --test tests -- --list >"$LIST" 2>&1
list_rc=$?
set -e
if [ "$list_rc" -ne 0 ]; then
  cp "$LIST" /out/run.txt
  exit "$list_rc"
fi
listed=$(awk '/: test$/ { n++ } END { print n+0 }' "$LIST")
if [ "$listed" -eq 0 ]; then
  echo "cargo preflight listed zero tests" > /out/run.txt
  exit 2
fi

set +e
UUTESTS_BINARY_PATH=/input/coreutils \
  cargo test --offline --features unix --test tests -- \
    "--test-threads=${THREADS:-2}" "${DANGEROUS_SKIPS[@]}" > /out/run.txt 2>&1
rc=$?
set -e
printf '\nBASHY_UUTILS_LISTED=%d\nBASHY_UUTILS_CARGO_EXIT=%d\n' \
  "$listed" "$rc" >> /out/run.txt
exit "$rc"
