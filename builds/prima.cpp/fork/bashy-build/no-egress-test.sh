#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$here/egress-verdict.sh"
bin=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
asset=$(basename "$bin")
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
python3 "$here/tiny-model.py" "$work/model.gguf"
ring=$(pwd)/build/bin/prima-ring-test
chmod 755 "$work"
mkdir -p "$work/home"
case $(uname -s) in
Linux)
  # Trace the entire process tree, including denied attempts, in a loopback-only namespace.
  # Output intentionally belongs to the unprivileged runner.
  # shellcheck disable=SC2024
  sudo unshare --net -- bash -c '
    set -euo pipefail
    ip link set lo up
    strace -f -qq -s 256 -e trace=connect,sendto,sendmsg,sendmmsg -o "$1/trace" \
      setpriv --reuid="$2" --regid="$3" --init-groups \
      env -i PATH=/usr/bin:/bin HOME="$1/home" \
      python3 "$4/exercise.py" "$5" "$1/model.gguf" "$6"
  ' bash "$work" "$(id -u)" "$(id -g)" "$here" "$bin" "$ring" >"out/$asset-egress.txt" 2>&1
  [[ -s $work/trace ]] || { echo "missing syscall observations"; exit 1; }
  egress_verdict "$work/trace" 5599 5600 19000 19001 19100 19101
  cat "$work/trace" >>"out/$asset-egress.txt"
  ;;
Darwin)
  # A positive-control denial must be visible; otherwise the observer is unproven.
  cat >"$work/profile.sb" <<'SB'
(version 1)
(allow default)
(deny network-outbound)
(allow network-outbound (remote ip "localhost:5599") (remote ip "localhost:5600") (remote ip "localhost:19000") (remote ip "localhost:19001") (remote ip "localhost:19100") (remote ip "localhost:19101"))
SB
  start=$(date '+%Y-%m-%d %H:%M:%S')
  sandbox-exec -f "$work/profile.sb" python3 -c 'import socket; socket.create_connection(("192.0.2.1", 443), 2)' 2>"$work/control" && exit 1
  sleep 2
  /usr/bin/log show --style syslog --start "$start" --predicate 'eventMessage CONTAINS "deny" AND eventMessage CONTAINS "network"' >"$work/control-log"
  grep -qi python "$work/control-log" || { echo 'sandbox observer cannot see control denial'; exit 1; }
  start=$(date '+%Y-%m-%d %H:%M:%S')
  env -i PATH="$PATH" HOME="$work/home" sandbox-exec -f "$work/profile.sb" \
    python3 "$here/exercise.py" "$bin" "$work/model.gguf" "$ring" >"out/$asset-egress.txt" 2>&1
  sleep 2
  /usr/bin/log show --style syslog --start "$start" --predicate 'eventMessage CONTAINS "deny" AND eventMessage CONTAINS "network"' >"$work/denies"
  if grep -E 'llama-cli|prima-|Python|python' "$work/denies"; then exit 1; fi
  cat "$work/control-log" >>"out/$asset-egress.txt"
  ;;
*) exit 1 ;;
esac
