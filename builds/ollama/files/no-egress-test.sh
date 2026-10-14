#!/usr/bin/env bash
# Hosted native inference only. Never execute this workload on Dragon.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
[[ $# == 1 ]] || { echo 'usage: no-egress-test.sh out/runner-OS-ARCH' >&2; exit 2; }
[[ ${GITHUB_ACTIONS:-} == true && ${RUNNER_ENVIRONMENT:-} == github-hosted ]] || {
  echo 'native gate requires a GitHub-hosted runner' >&2; exit 1;
}
case $(hostname | tr '[:upper:]' '[:lower:]') in *dragon*) echo 'Dragon native gate prohibited' >&2; exit 1;; esac
asset=$(basename "$1")
bin=$1
[[ $asset != runner-windows-amd64 ]] || bin=$1.exe
[[ -f $bin && -f $1.json ]] || { echo 'missing gated executable/metadata' >&2; exit 1; }
bin=$(cd "$(dirname "$bin")" && pwd)/$(basename "$bin")
out=$(dirname "$bin")
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/home"
# Invalidate stale evidence before any fixture/tool/runtime operation.
python3 -c 'import json,sys; open(sys.argv[1], "w").write(json.dumps({"status":"FAIL", "error":"native gate incomplete"})+"\n")' "$out/$asset-native.json"
: >"$out/$asset-egress.txt"
python3 "$here/fetch-fixture.py" "$work/model.gguf" >>"$out/$asset-egress.txt" 2>&1 || {
  cat "$out/$asset-egress.txt" >&2; exit 1;
}
# Fixed configured endpoint, never silently retry a collision on another port.
port=18997
args=("$bin" "$work/model.gguf" "$out/$asset-native.json" "$port")
case $(uname -s) in
Linux)
  # shellcheck source=/dev/null
  . "$here/egress-verdict.sh"
  for tool in sudo unshare ip strace setpriv python3; do command -v "$tool" >/dev/null; done
  chmod 755 "$work"
  # Entire exercise and child tree is traced; network namespace has only lo.
  # shellcheck disable=SC2024
  sudo -n unshare --net -- bash -c '
    set -euo pipefail
    ip link set lo up
    ulimit -f 32768
    exec strace -f -qq -s 256 -e trace=connect,sendto,sendmsg,sendmmsg -o "$1/trace" \
      setpriv --reuid="$2" --regid="$3" --init-groups \
      env -i PATH=/usr/bin:/bin HOME="$1/home" \
      python3 "$4/native-gate.py" "${@:5}"
  ' bash "$work" "$(id -u)" "$(id -g)" "$here" "${args[@]}" >>"$out/$asset-egress.txt" 2>&1 || {
    cat "$out/$asset-egress.txt" >&2; exit 1;
  }
  [[ -s $work/trace ]] || { echo 'missing network trace' >&2; exit 1; }
  cat "$work/trace" >>"$out/$asset-egress.txt"
  egress_verdict "$work/trace" "$port" >>"$out/$asset-egress.txt"
  ;;
Darwin)
  command -v sandbox-exec >/dev/null
  # Proven Seatbelt form (prima.cpp tag bashy-bashy-prima-0.1.2-13, run
  # 38486571631, green on the hosted macos-15 runners): (remote ip
  # "localhost:PORT"). The ip4 variant failed CI runs 38448516095 and
  # later on those runners.
  cat >"$work/profile.sb" <<SB
(version 1)
(allow default)
(deny network-outbound)
(allow network-outbound (remote ip "localhost:$port"))
SB
  # Includes wrong-loopback-port, IPv4, IPv6 and UDP denial controls.
  sandbox-exec -f "$work/profile.sb" python3 "$here/native-gate.py" --deny-controls "$port" >>"$out/$asset-egress.txt" 2>&1
  env -i PATH="$PATH" HOME="$work/home" sandbox-exec -f "$work/profile.sb" \
    python3 "$here/native-gate.py" "${args[@]}" >>"$out/$asset-egress.txt" 2>&1 || {
    cat "$out/$asset-egress.txt" >&2; exit 1;
  }
  ;;
MINGW*|MSYS*)
  # Hosted Windows gate on the built-in Windows Defender Firewall: an
  # elevated PowerShell step installs disposable outbound BLOCK rules on
  # the exact exercise programs (no custom native code — CI runs
  # 38448516095, 38450982320, 38452584047 failed in the previous custom
  # WFP-API harness, now deleted). Loopback is not
  # filtered by Windows Firewall, so the runner's 127.0.0.1 exercise works
  # unimpeded and needs no allowlist. Enforcement is proven by a negative
  # control (an outbound TCP 443 connect from the blocked interpreter)
  # that must fail AND appear as DROP in the firewall dropped-packet log
  # (logging is enabled by the gate and restored afterwards). The gate
  # passes only if the protocol exercise records PROTOCOL_PASS AND the
  # control is blocked and logged; any observed allowed non-loopback
  # connection fails it. (GitHub-hosted and non-Dragon are already
  # enforced at the top of this script.)
  command -v python3 >/dev/null || { echo 'missing python3' >&2; exit 1; }
  command -v powershell.exe >/dev/null || { echo 'missing powershell.exe' >&2; exit 1; }
  [[ $asset == runner-windows-amd64 ]] || { echo "unsupported Windows asset: $asset" >&2; exit 1; }
  [[ -f $bin ]] || { echo "missing Windows runner executable: $bin" >&2; exit 1; }
  # Native paths: the gate keys rules on exact executable paths, and the
  # exercise interpreter below is the blocked program, so the control and
  # the exercise run under this same python. The firewall API rejects
  # forward slashes in -Program paths ("invalid characters"), so rule
  # programs use backslashes; PowerShell accepts either form everywhere
  # else, so only the rule programs are converted.
  winpath() { cygpath -m "$1" | tr '/' '\\'; }
  # realpath, not sys.executable verbatim: the invoking python3 may be a
  # hostedtoolcache launcher or shim whose path differs from the running
  # image the firewall matches -Program rules against (prima run
  # 38481498544: TimeoutError with no DROP). The gate re-canonicalizes and
  # also blocks the interpreter's self-reported image, so this is the
  # first of two resolutions, not the only one.
  pyexe=$(python3 -c 'import sys, os; print(os.path.realpath(sys.executable))')
  pyexe=$(winpath "$pyexe")
  runner_win=$(winpath "$bin")
  gate_win=$(cygpath -m "$here/native-gate.py")
  model_win=$(cygpath -m "$work/model.gguf")
  evidence_win=$(cygpath -m "$out/$asset-native.json")
  out_win=$(cygpath -m "$out/$asset-egress.txt")
  gatework_win=$(cygpath -m "$work")
  gate_args=(-PythonExe "$pyexe" -RunnerExe "$runner_win"
             -GatePy "$gate_win" -ModelFile "$model_win"
             -EvidenceFile "$evidence_win" -Port "$port"
             -OutFile "$out_win" -WorkDir "$gatework_win")
  gate_ps1=$(cygpath -m "$here/windows-defender-egress.ps1")
  # The Defender script owns the evidence file and rewrites it wholesale
  # (prima-proven behavior); stash the fixture-fetch log above so its
  # hash verification stays in the evidence, then prepend it back below.
  cp "$out/$asset-egress.txt" "$work/fetch-log.txt"
  if powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$gate_ps1" "${gate_args[@]}" \
      >"$work/gate-console.txt" 2>&1; then
    echo 'no-egress: windows: protocol exercise succeeded under the Defender deny policy'
  else
    rc=$?
    echo "no-egress: windows: Defender gate failed (rc=$rc)" >&2
    cat "$work/gate-console.txt" >&2 || true
    exit 1
  fi
  # The evidence file decides: the blocked-and-logged control proof, the
  # PROTOCOL_PASS protocol verdict, the no-egress sampler verdict, and the
  # cleanup proof must all be present. A connected control or an allowed
  # non-loopback connection fails here even if the gate step above exited 0.
  grep -qF 'Defender control blocked' "$out/$asset-egress.txt" || {
    echo 'no-egress: windows: missing blocked-control proof' >&2; exit 1; }
  grep -qF 'pfirewall.log' "$out/$asset-egress.txt" || {
    echo 'no-egress: windows: missing firewall-log proof' >&2; exit 1; }
  grep -qF 'protocol gate PROTOCOL_PASS' "$out/$asset-egress.txt" || {
    echo 'no-egress: windows: protocol exercise did not reach PROTOCOL_PASS' >&2; exit 1; }
  grep -qF 'no non-loopback connections observed' "$out/$asset-egress.txt" || {
    echo 'no-egress: windows: missing no-egress sampler verdict' >&2; exit 1; }
  grep -qF 'Defender block rules removed; firewall logging restored' "$out/$asset-egress.txt" || {
    echo 'no-egress: windows: gate did not confirm policy removal' >&2; exit 1; }
  if grep -Ei 'CONTROL-CONNECTED|allowed non-loopback connection' "$out/$asset-egress.txt"; then
    echo 'no-egress: windows: evidence shows a connected control or an allowed connection' >&2; exit 1
  fi
  { cat "$work/fetch-log.txt"; cat "$out/$asset-egress.txt"; } >"$work/egress-merged.txt"
  cat "$work/egress-merged.txt" >"$out/$asset-egress.txt"
  ;;
*) echo 'unsupported native gate platform' >&2; exit 1;;
esac
# Only publish PASS after both protocol and platform enforcement have passed.
python3 "$here/native-gate.py" --finalize "$out/$asset-native.json" "$out/$asset-egress.txt"
cat "$out/$asset-native.json"
