#!/usr/bin/env bash
# Deterministic, offline tests for scripts/dks-profile.sh: the cloudbox/peer
# KUBECTL-resolution profile used by the DKS emitter/result/gate scripts.
# No cluster, no kubectl execution, no network.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap '/bin/rm -rf "$tmp"' EXIT
fail=0

note() { printf '%s\n' "$*" >&2; }
check() {
  local desc="$1" got="$2" want="$3"
  if [ "$got" != "$want" ]; then
    note "FAIL: $desc"
    note "  want: $want"
    note "  got:  $got"
    fail=1
  fi
}

# --- dks_resolve_kubectl unit coverage -------------------------------------

# 1. Default (no DKS_PROFILE) resolves to the caller's existing default -
#    today's cloudbox behavior, unchanged.
out="$(bash -c '. "'"$root"'/scripts/dks-profile.sh"; dks_resolve_kubectl "outpost kubectl"')"
check "unset profile leaves the cloudbox default untouched" "$out" "outpost kubectl"

out="$(bash -c '. "'"$root"'/scripts/dks-profile.sh"; DKS_PROFILE=cloudbox dks_resolve_kubectl "bashy kubectl"')"
check "explicit cloudbox profile leaves the default untouched" "$out" "bashy kubectl"

# Redo the peer-path checks under an isolated HOME so we never touch the
# operator's real ~/.kube.
fake_home="$tmp/home"
mkdir -p "$fake_home/.kube/outpost-control-plane"
: >"$fake_home/.kube/outpost-control-plane/k3s.yaml"

out="$(HOME="$fake_home" bash -c '. "'"$root"'/scripts/dks-profile.sh"; DKS_PROFILE=peer dks_resolve_kubectl "outpost kubectl"')"
check "peer profile resolves default kubeconfig path" "$out" "kubectl --kubeconfig $fake_home/.kube/outpost-control-plane/k3s.yaml"

# 2b. profile=peer honors an overriding DKS_PEER_KUBECONFIG.
custom_kc="$tmp/custom-k3s.yaml"
: >"$custom_kc"
out="$(bash -c '. "'"$root"'/scripts/dks-profile.sh"; DKS_PROFILE=peer DKS_PEER_KUBECONFIG="'"$custom_kc"'" dks_resolve_kubectl "outpost kubectl"')"
check "peer profile honors DKS_PEER_KUBECONFIG override" "$out" "kubectl --kubeconfig $custom_kc"

# 3. peer profile with a missing kubeconfig fails loudly, never falls back.
missing_kc="$tmp/does-not-exist.yaml"
set +e
err="$(bash -c '. "'"$root"'/scripts/dks-profile.sh"; DKS_PROFILE=peer DKS_PEER_KUBECONFIG="'"$missing_kc"'" dks_resolve_kubectl "outpost kubectl"' 2>&1 1>/dev/null)"
rc=$?
set -e
[ "$rc" -ne 0 ] || { note "FAIL: peer profile with missing kubeconfig exited 0"; fail=1; }
case "$err" in
  *"DKS_PROFILE=peer"*"$missing_kc"*) ;;
  *) note "FAIL: missing-kubeconfig error is unclear: $err"; fail=1 ;;
esac
case "$err" in
  *outpost*|*cloudbox*fallback*|*"falling back"*)
    note "FAIL: missing-kubeconfig error mentions a cloudbox fallback"; fail=1 ;;
esac

# 4. Unknown profile fails loudly too (not a silent cloudbox default).
set +e
err="$(bash -c '. "'"$root"'/scripts/dks-profile.sh"; DKS_PROFILE=bogus dks_resolve_kubectl "outpost kubectl"' 2>&1 1>/dev/null)"
rc=$?
set -e
[ "$rc" -ne 0 ] || { note "FAIL: unknown profile exited 0"; fail=1; }
case "$err" in
  *"unknown DKS_PROFILE"*bogus*) ;;
  *) note "FAIL: unknown-profile error is unclear: $err"; fail=1 ;;
esac

# --- end-to-end wiring: the emitter/result/gate scripts --------------------
# Prove the DEFAULT path each script actually takes (not just the helper
# function) is byte-identical to today: prepend a fake "outpost"/"bashy"
# command that records its argv, and assert it is invoked with exactly the
# historical default prefix when no profile/KUBECTL is set.

bindir="$tmp/bin"
mkdir -p "$bindir"

cat >"$bindir/outpost" <<'EOF'
#!/usr/bin/env bash
printf 'outpost %s\n' "$*" >>"$CALL_LOG"
exit 9
EOF
cat >"$bindir/bashy" <<'EOF'
#!/usr/bin/env bash
printf 'bashy %s\n' "$*" >>"$CALL_LOG"
exit 9
EOF
chmod +x "$bindir/outpost" "$bindir/bashy"

call_log="$tmp/calls.log"
: >"$call_log"
( cd "$root" && PATH="$bindir:/usr/bin:/bin" CALL_LOG="$call_log" NS=default JOB=probe-job \
  scripts/k8s-job-aggregate.sh >/dev/null 2>&1 ) || true
case "$(cat "$call_log")" in
  'outpost kubectl get job probe-job -n default'*) ;;
  *) note "FAIL: k8s-job-aggregate.sh default KUBECTL changed (log: $(cat "$call_log"))"; fail=1 ;;
esac

: >"$call_log"
( cd "$root" && PATH="$bindir:/usr/bin:/bin" CALL_LOG="$call_log" NS=default JOB=probe-job \
  scripts/dks-native-result.sh >/dev/null 2>&1 ) || true
case "$(cat "$call_log")" in
  'bashy kubectl get job probe-job -n default'*) ;;
  *) note "FAIL: dks-native-result.sh default KUBECTL changed (log: $(cat "$call_log"))"; fail=1 ;;
esac

: >"$call_log"
set +e
( cd "$root" && PATH="$bindir:/usr/bin:/bin" CALL_LOG="$call_log" \
  DKS_PROFILE=peer DKS_PEER_KUBECONFIG="$tmp/still-missing.yaml" NS=default JOB=probe-job \
  scripts/dks-native-result.sh >"$tmp/peer-missing.out" 2>&1 )
rc=$?
set -e
[ "$rc" -ne 0 ] || { note "FAIL: dks-native-result.sh with peer+missing kubeconfig exited 0"; fail=1; }
[ ! -s "$call_log" ] || { note "FAIL: dks-native-result.sh fell back to cloudbox kubectl (log: $(cat "$call_log"))"; fail=1; }
grep -q 'DKS_PROFILE=peer' "$tmp/peer-missing.out" || { note "FAIL: dks-native-result.sh peer failure message unclear"; fail=1; }

# Explicit $KUBECTL still wins, even with a peer profile set.
: >"$call_log"
explicit_kc="$tmp/explicit-marker"
( cd "$root" && PATH="$bindir:/usr/bin:/bin" CALL_LOG="$call_log" \
  DKS_PROFILE=peer DKS_PEER_KUBECONFIG="$tmp/still-missing.yaml" \
  KUBECTL="bashy kubectl" NS=default JOB=probe-job \
  scripts/dks-native-result.sh >/dev/null 2>&1 ) || true
case "$(cat "$call_log")" in
  'bashy kubectl get job probe-job -n default'*) ;;
  *) note "FAIL: explicit \$KUBECTL was overridden by the peer profile (log: $(cat "$call_log"))"; fail=1 ;;
esac

# dks-release-gate.sh resolves KUBECTL before its other required-env checks,
# so a peer+missing-kubeconfig failure surfaces even without the release
# gate's other mandatory inputs set.
set +e
gate_err="$( cd "$root" && DKS_PROFILE=peer DKS_PEER_KUBECONFIG="$tmp/still-missing.yaml" \
  scripts/dks-release-gate.sh 2>&1 1>/dev/null )"
rc=$?
set -e
[ "$rc" -ne 0 ] || { note "FAIL: dks-release-gate.sh with peer+missing kubeconfig exited 0"; fail=1; }
case "$gate_err" in
  *"DKS_PROFILE=peer"*"still-missing.yaml"*) ;;
  *) note "FAIL: dks-release-gate.sh peer failure message unclear: $gate_err"; fail=1 ;;
esac

# --- Bashy panic regression -------------------------------------------------
# The nested `KUBECTL="${KUBECTL:-$(dks_resolve_kubectl ...)}"` default and the
# `${BASH_SOURCE[0]:-$0}` self-location idiom both panicked Bashy's
# mvdan-based expander under `set -u` (a nil pointer dereference resolving an
# array-index/command-substitution parameter-expansion default) even though
# both are valid POSIX/bash. Every modified script now uses a plain
# `if [ -z ... ]; then ...; fi` conditional and `$0` instead. Exercise each
# modified script through installed bashy, when available, and require it not
# to panic. Deterministic/offline: a stub PATH shadows outpost/bashy/gh so
# nothing touches a real cluster or network, and no script is expected to
# succeed here — only to fail cleanly instead of crashing.
if command -v bashy >/dev/null 2>&1; then
  bashy_bin="$(command -v bashy)"
  panic_bin="$tmp/panicbin"
  mkdir -p "$panic_bin"
  for stub in outpost bashy gh git; do
    cat >"$panic_bin/$stub" <<EOF
#!/usr/bin/env bash
echo "$stub: stub refuses \$*" >&2
exit 9
EOF
    chmod +x "$panic_bin/$stub"
  done

  no_panic() {
    local desc="$1" script="$2"
    shift 2
    local out rc
    set +e
    out="$(cd "$root" && env -i \
      PATH="$panic_bin:/usr/bin:/bin" HOME="$fake_home" "$@" \
      "$bashy_bin" "$script" 2>&1)"
    rc=$?
    set -e
    case "$out" in
      *'panic:'*|*'SIGSEGV'*|*'goroutine '*)
        note "FAIL: $desc panicked under bashy (exit=$rc)"
        note "$out"
        fail=1
        ;;
    esac
  }

  no_panic "dks-release-gate.sh" scripts/dks-release-gate.sh \
    NS=default JOB=probe-job \
    EXPECTED_SOURCE_REF=deadbeef EXPECTED_SOURCE_SHA256=deadbeef \
    PIPELINE_FILE="$tmp/pipeline.json" NATIVE_JOBS=probe-job CONFORMANCE_JOBS=probe-job

  no_panic "dks-native-result.sh" scripts/dks-native-result.sh \
    NS=default JOB=probe-job

  no_panic "k8s-job-aggregate.sh" scripts/k8s-job-aggregate.sh \
    NS=default JOB=probe-job

  no_panic "dks-author-qa-refs.sh" scripts/dks-author-qa-refs.sh \
    VERSION=v0.0.0 EXPECTED_SOURCE_REF=deadbeef EXPECTED_SOURCE_SHA256=deadbeef \
    PIPELINE_FILE="$tmp/pipeline.json" NATIVE_JOBS=probe-job CONFORMANCE_JOBS=probe-job
else
  note "SKIP: bashy not installed - bashy-panic regression not run"
fi

if [ "$fail" -eq 0 ]; then
  echo "dks profile resolution: PASS"
else
  echo "dks profile resolution: FAIL" >&2
  exit 1
fi
