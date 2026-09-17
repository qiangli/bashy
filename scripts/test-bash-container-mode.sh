#!/bin/sh
# Exercise the real Makefile/helper wiring without building or running OCI.
# The shebang's fake bash consumes the product selector, as Bashy does.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd -P)
scratch=$(mktemp -d "${TMPDIR:-/tmp}/bashy-container-mode.XXXXXX")
trap 'rm -rf "$scratch"' EXIT HUP INT TERM
mkdir -p "$scratch/repo/scripts" "$scratch/fakebin"
cp "$root/Makefile" "$scratch/repo/Makefile"
cp "$root/scripts/test-bash-container.sh" "$root/scripts/test-lane-id.sh" "$scratch/repo/scripts/"

cat >"$scratch/fakebin/bash" <<'EOF'
#!/bin/sh
unset BASHY_BASHPP
printf '%s\n' consumed >>"$MODE_SHELL_LOG"
exec /bin/bash "$@"
EOF
cat >"$scratch/fakebin/go" <<'EOF'
#!/bin/sh
[ "$*" = 'env GOARCH' ] || exit 81
printf '%s\n' amd64
EOF
cat >"$scratch/fakebin/oci" <<'EOF'
#!/bin/sh
printf '%s\n' "$@" >>"$MODE_OCI_LOG"
case "$1" in info|run) exit 0 ;; *) exit 82 ;; esac
EOF
cat >"$scratch/repo/scripts/build-conformance-image.sh" <<'EOF'
#!/bin/sh
exit 0
EOF
chmod +x "$scratch/fakebin/bash" "$scratch/fakebin/go" "$scratch/fakebin/oci" \
  "$scratch/repo/scripts/build-conformance-image.sh"
export MODE_OCI_LOG=$scratch/oci.log MODE_SHELL_LOG=$scratch/shell.log
export PATH=$scratch/fakebin:/usr/bin:/bin BASHY_TEST_LANE=mode-regression
export GO=go BASH53_OCI=oci CONTAINER_HOST=unused
fail() { echo "test-bash-container-mode: $*" >&2; exit 1; }

for mode in 0 1; do
  target=test-bash-container
  [ "$mode" = 0 ] || target=test-bash-container-bashpp
  : >"$MODE_OCI_LOG"
  : >"$MODE_SHELL_LOG"
  # An inherited product selector and a stale harness request cannot override
  # either target. The helper's env-bash consumes the former before forwarding.
  BASHY_BASHPP=1 BASH53_BASHPP=stale MAKEFLAGS= \
    make -j1 -C "$scratch/repo" "$target" >"$scratch/out" 2>"$scratch/err" || {
      cat "$scratch/err" >&2
      fail "target $target failed"
    }
  grep -qx consumed "$MODE_SHELL_LOG" || fail 'env-bash did not consume the product selector'
  grep -qx "BASH53_BASHPP=$mode" "$MODE_OCI_LOG" || fail "lost requested mode $mode"
  if grep -q '^BASHY_BASHPP=' "$MODE_OCI_LOG"; then
    fail 'helper forwarded the consumed product selector'
  fi
  awk -v expected="$mode" '
    previous == "-bashpp-mode" && $0 == expected { found = 1 }
    { previous = $0 }
    END { exit !found }
  ' "$MODE_OCI_LOG" || fail "runner did not receive an independent mode $mode assertion"
done

for mode in '' lost; do
  : >"$MODE_OCI_LOG"
  if BASH53_BASHPP="$mode" /bin/bash "$scratch/repo/scripts/test-bash-container.sh" \
    >"$scratch/out" 2>"$scratch/err"; then
    fail "invalid selector '$mode' succeeded"
  else
    status=$?
  fi
  [ "$status" = 2 ] || fail "invalid selector exited $status, want 2"
  grep -q 'require BASH53_BASHPP=0 or 1' "$scratch/err" || fail 'missing selector was silent'
  [ ! -s "$MODE_OCI_LOG" ] || fail 'invalid selector reached OCI'
done
echo 'test-bash-container-mode: PASS'
