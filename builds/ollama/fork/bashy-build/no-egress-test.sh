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
  cat >"$work/profile.sb" <<SB
(version 1)
(allow default)
(deny network-outbound)
(allow network-outbound (remote ip "127.0.0.1:$port"))
SB
  # Includes wrong-loopback-port, IPv4, IPv6 and UDP denial controls.
  sandbox-exec -f "$work/profile.sb" python3 "$here/native-gate.py" --deny-controls "$port" >>"$out/$asset-egress.txt" 2>&1
  env -i PATH="$PATH" HOME="$work/home" sandbox-exec -f "$work/profile.sb" \
    python3 "$here/native-gate.py" "${args[@]}" >>"$out/$asset-egress.txt" 2>&1 || {
    cat "$out/$asset-egress.txt" >&2; exit 1;
  }
  ;;
MINGW*|MSYS*)
  command -v gcc >/dev/null
  # Compile only on the hosted Windows image, using the same MinGW provision.
  gcc -std=c11 -Wall -Wextra -Werror -municode "$here/windows-net-guard.c" \
    -o "$work/net-guard.exe" -lfwpuclnt -lws2_32 -lrpcrt4
  python3 "$here/native-gate.py" "${args[@]}" "$work/net-guard.exe" >>"$out/$asset-egress.txt" 2>&1 || {
    cat "$out/$asset-egress.txt" >&2; exit 1;
  }
  ;;
*) echo 'unsupported native gate platform' >&2; exit 1;;
esac
# Only publish PASS after both protocol and platform enforcement have passed.
python3 "$here/native-gate.py" --finalize "$out/$asset-native.json" "$out/$asset-egress.txt"
cat "$out/$asset-native.json"
