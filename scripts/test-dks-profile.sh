#!/usr/bin/env bash
# Deterministic, offline tests for scripts/dks-profile.sh: the argv-safe
# cloudbox/peer KUBECTL-resolution profile used by the DKS emitter/result/gate
# scripts. No cluster, no real kubectl execution, no network.
#
# Covers, beyond the original resolution matrix:
#   - TOKEN NON-LEAK: a credential-shaped override value never reaches any
#     output stream on any consumer or failure path (the headline test).
#   - WHITESPACE: a kubeconfig path containing a space resolves and is invoked
#     as one argv element (not refused, not split).
#   - GLOB: a path containing *, ?, and [abc] is passed literally and never
#     expanded against the filesystem, even with a matching decoy file nearby.
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

# Run the resolver in a fresh shell and print the resolved argv, one '|'-joined
# line (announcement on stderr is discarded). Extra shell code (env, asserts)
# is passed as $2.
resolve_argv() {
  local default="$1" pre="${2:-}"
  bash -c '
    . "'"$root"'/scripts/dks-profile.sh"
    '"$pre"'
    dks_kubectl_init "'"$default"'" 2>/dev/null
    IFS="|"; printf "%s\n" "${DKS_KUBECTL_CMD[*]}"
  '
}

# --- dks_kubectl_init unit coverage ----------------------------------------

# 1. Default (no DKS_PROFILE) resolves to the caller's existing default -
#    today's cloudbox behavior, unchanged, now as an argv (space-joined here).
check "unset profile leaves the cloudbox default untouched" \
  "$(resolve_argv 'outpost kubectl')" "outpost|kubectl"
check "explicit cloudbox profile leaves the default untouched" \
  "$(resolve_argv 'bashy kubectl' 'export DKS_PROFILE=cloudbox')" "bashy|kubectl"

# Redo the peer-path checks under an isolated HOME so we never touch the
# operator's real ~/.kube.
fake_home="$tmp/home"
mkdir -p "$fake_home/.kube/outpost-control-plane"
: >"$fake_home/.kube/outpost-control-plane/k3s.yaml"

check "peer profile resolves default kubeconfig path" \
  "$(resolve_argv 'outpost kubectl' 'export HOME="'"$fake_home"'" DKS_PROFILE=peer')" \
  "kubectl|--kubeconfig|$fake_home/.kube/outpost-control-plane/k3s.yaml"

# 2b. profile=peer honors an overriding DKS_PEER_KUBECONFIG.
custom_kc="$tmp/custom-k3s.yaml"
: >"$custom_kc"
check "peer profile honors DKS_PEER_KUBECONFIG override" \
  "$(resolve_argv 'outpost kubectl' 'export DKS_PROFILE=peer DKS_PEER_KUBECONFIG="'"$custom_kc"'"')" \
  "kubectl|--kubeconfig|$custom_kc"

# 2c. An explicit KUBECTL string override wins over any profile, split argv-safe.
check "explicit KUBECTL string wins over peer profile" \
  "$(resolve_argv 'outpost kubectl' 'export DKS_PROFILE=peer KUBECTL="bashy kubectl"')" \
  "bashy|kubectl"

# 2d. An inherited DKS_KUBECTL_ARGV (argv-safe, one element per line) wins and
#     is reconstructed exactly, preserving spaces and glob metacharacters.
inherit=$'kubectl\n--kubeconfig\n/weird path/k*?[x].yaml'
check "inherited DKS_KUBECTL_ARGV reconstructs the argv verbatim" \
  "$(resolve_argv 'outpost kubectl' 'export DKS_KUBECTL_ARGV="'"$inherit"'"')" \
  "kubectl|--kubeconfig|/weird path/k*?[x].yaml"

# 2e. A trailing newline in the serialized argv (the natural output of any
#     `printf '%s\n'` serializer, e.g. a hand-exported DKS_KUBECTL_ARGV) must
#     not reconstruct as a spurious empty argv element.
inherit_nl=$'kubectl\n--kubeconfig\n/space path/k3s.yaml\n'
check "inherited DKS_KUBECTL_ARGV trailing newline yields no empty element" \
  "$(resolve_argv 'outpost kubectl' 'export DKS_KUBECTL_ARGV="'"$inherit_nl"'"')" \
  "kubectl|--kubeconfig|/space path/k3s.yaml"

# 3. peer profile with a missing kubeconfig fails loudly, never falls back.
missing_kc="$tmp/does-not-exist.yaml"
set +e
err="$(bash -c '. "'"$root"'/scripts/dks-profile.sh"; DKS_PROFILE=peer DKS_PEER_KUBECONFIG="'"$missing_kc"'" dks_kubectl_init "outpost kubectl"' 2>&1 1>/dev/null)"
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

# 3b. peer profile with an UNREADABLE (mode-000) kubeconfig also fails closed:
#     resolution checks `-r`, not just `-f`. Skipped for root, which bypasses
#     file permission checks.
if [ "$(id -u)" -ne 0 ]; then
  unreadable_kc="$tmp/unreadable-k3s.yaml"
  : >"$unreadable_kc"
  chmod 000 "$unreadable_kc"
  set +e
  err="$(bash -c '. "'"$root"'/scripts/dks-profile.sh"; DKS_PROFILE=peer DKS_PEER_KUBECONFIG="'"$unreadable_kc"'" dks_kubectl_init "outpost kubectl"' 2>&1 1>/dev/null)"
  rc=$?
  set -e
  [ "$rc" -ne 0 ] || { note "FAIL: peer profile with unreadable kubeconfig exited 0"; fail=1; }
  case "$err" in
    *"unreadable"*"$unreadable_kc"*) ;;
    *) note "FAIL: unreadable-kubeconfig error is unclear: $err"; fail=1 ;;
  esac
fi

# 4. Unknown profile fails loudly too (not a silent cloudbox default).
set +e
err="$(bash -c '. "'"$root"'/scripts/dks-profile.sh"; DKS_PROFILE=bogus dks_kubectl_init "outpost kubectl"' 2>&1 1>/dev/null)"
rc=$?
set -e
[ "$rc" -ne 0 ] || { note "FAIL: unknown profile exited 0"; fail=1; }
case "$err" in
  *"unknown DKS_PROFILE"*bogus*) ;;
  *) note "FAIL: unknown-profile error is unclear: $err"; fail=1 ;;
esac

# 4b. Every near-miss typo of "peer" fails closed - none may route to cloudbox.
#     An explicitly-EMPTY DKS_PROFILE is a typo'd variable, not "unset", and
#     must fail closed too rather than silently defaulting to cloudbox.
for typo in Peer PEER 'peer ' ' peer' peeer ''; do
  set +e
  out="$(bash -c '. "'"$root"'/scripts/dks-profile.sh"; DKS_PROFILE="'"$typo"'" dks_kubectl_init "outpost kubectl" 2>/dev/null; IFS="|"; printf "%s" "${DKS_KUBECTL_CMD[*]}"')"
  rc=$?
  set -e
  [ "$rc" -ne 0 ] || { note "FAIL: DKS_PROFILE='$typo' did not fail closed (rc=$rc)"; fail=1; }
  case "$out" in
    *outpost*|*cloudbox*) note "FAIL: DKS_PROFILE='$typo' routed to a cloudbox default"; fail=1 ;;
  esac
done

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

# The gate hands its resolved argv to the child scripts via DKS_KUBECTL_ARGV.
# Prove the child re-resolves to the gate's default (bashy kubectl), not its own,
# under cloudbox - i.e. propagation still works after the KUBECTL string channel
# was retired.
: >"$call_log"
( cd "$root" && PATH="$bindir:/usr/bin:/bin" CALL_LOG="$call_log" NS=default JOB=probe-job \
  DKS_KUBECTL_ARGV=$'bashy\nkubectl' \
  scripts/k8s-job-aggregate.sh >/dev/null 2>&1 ) || true
case "$(cat "$call_log")" in
  'bashy kubectl get job probe-job -n default'*) ;;
  *) note "FAIL: DKS_KUBECTL_ARGV not honored by k8s-job-aggregate.sh (log: $(cat "$call_log"))"; fail=1 ;;
esac

# --- TOKEN NON-LEAK (headline) ---------------------------------------------
# A credential-shaped override value must appear in NO output stream on ANY
# consumer or failure path. The resolver announces the executable BASENAME, so
# the token in --token=... is never echoed; the argv is exec'd, never printed.
secret='SUPERSECRETtok3n'
# A silent fake kubectl: never echoes argv, so any appearance of the secret in
# output is the SCRIPTS leaking it, not the fake.
cat >"$bindir/kubectl-silent" <<'EOF'
#!/usr/bin/env bash
exit 7
EOF
chmod +x "$bindir/kubectl-silent"
silent="$bindir/kubectl-silent"

leak_scan() { # desc, output
  case "$2" in
    *"$secret"*) note "FAIL: $1 leaked the override secret to an output stream"; fail=1 ;;
  esac
}

# Carrier A: the legacy KUBECTL string override, secret in a --token= flag.
# Carrier B: an inherited DKS_KUBECTL_ARGV, secret as its own element.
for carrier in KUBECTL ARGV; do
  if [ "$carrier" = KUBECTL ]; then
    creds=(KUBECTL="$silent --token=$secret")
  else
    creds=(DKS_KUBECTL_ARGV="$silent"$'\n'"--token=$secret")
  fi

  # Every direct consumer + its failure path (the silent fake fails every call).
  out="$( cd "$root" && env PATH="$bindir:/usr/bin:/bin" "${creds[@]}" \
    NS=default JOB=probe-job scripts/k8s-job-aggregate.sh 2>&1 || true )"
  leak_scan "k8s-job-aggregate.sh ($carrier)" "$out"

  out="$( cd "$root" && env PATH="$bindir:/usr/bin:/bin" "${creds[@]}" \
    NS=default JOB=probe-job scripts/dks-native-result.sh 2>&1 || true )"
  leak_scan "dks-native-result.sh ($carrier)" "$out"

  # The gate: full required env so it reaches the kubectl call path, then fails.
  out="$( cd "$root" && env PATH="$bindir:/usr/bin:/bin" "${creds[@]}" \
    NS=default \
    EXPECTED_SOURCE_REF=deadbeef EXPECTED_SOURCE_SHA256=deadbeef \
    PIPELINE_FILE="$tmp/pipeline.json" \
    NATIVE_JOBS=probe-job CONFORMANCE_JOBS=probe-job \
    scripts/dks-release-gate.sh 2>&1 || true )"
  leak_scan "dks-release-gate.sh ($carrier)" "$out"

  # The fourth consumer: dks-author-qa-refs.sh resolves the profile up front and
  # serializes the argv into DKS_KUBECTL_ARGV for its child gate. It fails here
  # (no gh stub) AFTER that resolution, so both its resolution announcement and
  # its failure path are exercised. gh is absent from the stub PATH, so the
  # failure message is the shell's own "command not found" - never the argv.
  out="$( cd "$root" && env PATH="$bindir:/usr/bin:/bin" "${creds[@]}" \
    VERSION=v0.0.0 EXPECTED_SOURCE_REF=deadbeef EXPECTED_SOURCE_SHA256=deadbeef \
    PIPELINE_FILE="$tmp/pipeline.json" \
    NATIVE_JOBS=probe-job CONFORMANCE_JOBS=probe-job \
    scripts/dks-author-qa-refs.sh 2>&1 || true )"
  leak_scan "dks-author-qa-refs.sh ($carrier)" "$out"

  # The resolver's own announcement, captured directly.
  out="$( cd "$root" && env PATH="$bindir:/usr/bin:/bin" "${creds[@]}" \
    bash -c '. scripts/dks-profile.sh; dks_kubectl_init "bashy kubectl"' 2>&1 || true )"
  leak_scan "resolver announcement ($carrier)" "$out"
done

# --- WHITESPACE + GLOB kubeconfig paths ------------------------------------
# A fake kubectl that records each argv element on its own line, so we can prove
# the kubeconfig path arrived as ONE element, verbatim, unexpanded.
cat >"$bindir/kubectl" <<'EOF'
#!/usr/bin/env bash
for a in "$@"; do printf 'ARG<%s>\n' "$a" >>"$ARGV_LOG"; done
exit 0
EOF
chmod +x "$bindir/kubectl"
argv_log="$tmp/argv.log"

run_peer_kubectl() { # kubeconfig-path
  : >"$argv_log"
  ( cd "$root" && PATH="$bindir:/usr/bin:/bin" ARGV_LOG="$argv_log" \
    DKS_PROFILE=peer DKS_PEER_KUBECONFIG="$1" NS=default JOB=probe-job \
    scripts/k8s-job-aggregate.sh >/dev/null 2>&1 ) || true
}

# WHITESPACE: a kubeconfig under a path with a space resolves and is invoked as
# a single argv element - not refused, not split.
ws_dir="$tmp/My Kube Configs"
mkdir -p "$ws_dir"
ws_kc="$ws_dir/k3s.yaml"
: >"$ws_kc"
run_peer_kubectl "$ws_kc"
grep -qxF "ARG<--kubeconfig>" "$argv_log" || { note "FAIL: whitespace path: --kubeconfig flag missing"; fail=1; }
grep -qxF "ARG<$ws_kc>" "$argv_log" || {
  note "FAIL: whitespace kubeconfig path was split or refused (argv log:)"; note "$(cat "$argv_log")"; fail=1;
}

# GLOB: a path with *, ?, and [abc] is passed literally and never expanded, even
# though a DECOY file that the glob WOULD match exists right beside it.
glob_dir="$tmp/globdir"
mkdir -p "$glob_dir"
glob_kc="$glob_dir/kube*?[abc].yaml"   # literal filename containing glob chars
: >"$glob_kc"
: >"$glob_dir/kubeXYa.yaml"            # decoy: matches kube*?[abc].yaml if expanded
: >"$glob_dir/kubeZZb.yaml"            # second decoy
run_peer_kubectl "$glob_kc"
grep -qxF "ARG<$glob_kc>" "$argv_log" || {
  note "FAIL: glob kubeconfig path not passed literally (argv log:)"; note "$(cat "$argv_log")"; fail=1;
}
if grep -qF 'kubeXYa.yaml' "$argv_log" || grep -qF 'kubeZZb.yaml' "$argv_log"; then
  note "FAIL: glob kubeconfig path was expanded against the filesystem (argv log:)"; note "$(cat "$argv_log")"; fail=1
fi
# The literal glob path must appear exactly once (no split into multiple args).
n=$(grep -cxF "ARG<$glob_kc>" "$argv_log" || true)
[ "$n" = 1 ] || { note "FAIL: glob path appeared $n times, expected exactly 1"; fail=1; }

# --- Bashy panic regression -------------------------------------------------
# The nested `KUBECTL="${KUBECTL:-$(dks_resolve_kubectl ...)}"` default and the
# `${BASH_SOURCE[0]:-$0}` self-location idiom both panicked Bashy's
# mvdan-based expander under `set -u` (a nil pointer dereference resolving an
# array-index/command-substitution parameter-expansion default) even though
# both are valid POSIX/bash. Every modified script now uses a plain
# `dks_kubectl_init` call and `$0` self-location. Exercise each modified script
# through installed bashy, when available, and require it not to panic.
# Deterministic/offline: a stub PATH shadows outpost/bashy/gh so nothing touches
# a real cluster or network, and no script is expected to succeed here - only to
# fail cleanly instead of crashing.
if command -v bashy >/dev/null 2>&1; then
  bashy_bin="$(command -v bashy)"
  panic_bin="$tmp/panicbin"
  mkdir -p "$panic_bin"
  for stub in outpost bashy gh git kubectl; do
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

  # Peer profile under bashy with whitespace + glob kubeconfig paths must not
  # panic (exercises the argv-array path through the mvdan expander).
  no_panic "dks-native-result.sh peer+whitespace" scripts/dks-native-result.sh \
    NS=default JOB=probe-job DKS_PROFILE=peer DKS_PEER_KUBECONFIG="$ws_kc"
  no_panic "dks-native-result.sh peer+glob" scripts/dks-native-result.sh \
    NS=default JOB=probe-job DKS_PROFILE=peer DKS_PEER_KUBECONFIG="$glob_kc"
else
  note "SKIP: bashy not installed - bashy-panic regression not run"
fi

if [ "$fail" -eq 0 ]; then
  echo "dks profile resolution: PASS"
else
  echo "dks profile resolution: FAIL" >&2
  exit 1
fi
