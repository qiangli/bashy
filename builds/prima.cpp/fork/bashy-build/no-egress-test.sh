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
  #
  # Fail-safe diagnostics: the namespace exercise runs redirected into
  # out/$asset-egress.txt so the artifact carries the full log, but every
  # failure path below reprints that file to stderr first. A gate failure
  # must leave its cause in the hosted step log even when the evidence
  # artifact cannot be uploaded (candidate -2 run 38411861522 left no cause
  # anywhere: the redirect hid the failure and the step exit skipped upload).
  # Detection is not weakened: the verdict below still fails on any
  # non-loopback attempt, including attempts the namespace itself denied.
  # shellcheck disable=SC2024
  if sudo unshare --net -- bash -c '
    set -euo pipefail
    echo "no-egress: linux: namespace entered; bringing loopback up"
    ip link set lo up
    echo "no-egress: linux: tracing exercise"
    ulimit -f 16384 # bounded trace file; exceeding the cap fails the exercise
    strace -f -qq -tt -T -s 64 -e trace=connect,sendto,sendmsg,sendmmsg,shutdown -o "$1/trace" \
      setpriv --reuid="$2" --regid="$3" --init-groups \
      env -i PATH=/usr/bin:/bin HOME="$1/home" \
      python3 "$4/exercise.py" "$5" "$1/model.gguf" "$6"
    echo "no-egress: linux: exercise finished"
  ' bash "$work" "$(id -u)" "$(id -g)" "$here" "$bin" "$ring" >"out/$asset-egress.txt" 2>&1; then
    : # namespace exercise completed; the verdict below decides
  else
    rc=$?
    echo "no-egress: linux: namespace exercise failed (rc=$rc)" >&2
    if [[ -s $work/trace ]]; then
      echo 'no-egress: linux: syscalls were traced, so unshare/strace worked; suspect the exercise (binary, ring test or model)' >&2
      {
        echo '---- bounded syscall trace (diagnostic only; exercise failed) ----'
        python3 "$here/exercise.py" --trace "$work/trace"
      } >>"out/$asset-egress.txt"
    else
      echo 'no-egress: linux: no syscall observations; suspect unshare/namespace setup, strace, or setpriv' >&2
    fi
    echo "---- out/$asset-egress.txt ----" >&2
    cat "out/$asset-egress.txt" >&2 || true
    exit 1
  fi
  if [[ ! -s $work/trace ]]; then
    echo 'no-egress: linux: missing syscall observations' >&2
    echo "---- out/$asset-egress.txt ----" >&2
    cat "out/$asset-egress.txt" >&2 || true
    exit 1
  fi
  # Loopback allowlist: pinned Prima 0.1.1 shim tests use 5599-5605;
  # rank-offset engine ports use 19000/19001 (data) and 19100/19101
  # (signal). Everything else stays fail-closed (see egress-verdict.sh).
  if ! egress_verdict "$work/trace" 5599 5600 5601 5602 5603 5604 5605 19000 19001 19100 19101; then
    python3 "$here/exercise.py" --trace "$work/trace" >>"out/$asset-egress.txt"
    exit 1
  fi
  python3 "$here/exercise.py" --trace "$work/trace" >>"out/$asset-egress.txt"
  ;;
Darwin)
  # The sandbox profile denies every outbound connection except the explicit
  # loopback ring ports below. Enforcement is proven by a positive control
  # whose denial is observed DIRECTLY in the attempt's own stderr (EPERM):
  # the unified log (`log show`) is not a reliable observer for sandbox
  # denies — hosted macos-15 run 38411861522 recorded no denial there even
  # though the control connection was denied — so the gate never depends on
  # its absence. Any OBSERVED denial evidence still fails the gate.
  cat >"$work/profile.sb" <<'SB'
(version 1)
(allow default)
(deny network-outbound)
(allow network-outbound (remote ip "localhost:5599") (remote ip "localhost:5600") (remote ip "localhost:5601") (remote ip "localhost:5602") (remote ip "localhost:5603") (remote ip "localhost:5604") (remote ip "localhost:5605") (remote ip "localhost:19000") (remote ip "localhost:19001") (remote ip "localhost:19100") (remote ip "localhost:19101"))
SB
  if sandbox-exec -f "$work/profile.sb" \
      python3 -c 'import socket; socket.create_connection(("192.0.2.1", 443), 2)' 2>"$work/control"; then
    echo 'no-egress: control connection succeeded under the deny profile' >&2
    cat "$work/control" >&2 || true
    exit 1
  fi
  grep -qi 'operation not permitted\|EPERM' "$work/control" || {
    echo 'no-egress: control failure was not a sandbox denial:' >&2
    cat "$work/control" >&2 || true
    exit 1
  }
  echo 'no-egress: darwin: positive control denied (EPERM) OK'
  start=$(date '+%Y-%m-%d %H:%M:%S')
  if env -i PATH="$PATH" HOME="$work/home" sandbox-exec -f "$work/profile.sb" \
      python3 "$here/exercise.py" "$bin" "$work/model.gguf" "$ring" >"$work/engine.log" 2>&1; then
    echo 'no-egress: darwin: engine + ring inference succeeded under the deny profile'
  else
    rc=$?
    cat "$work/engine.log" >&2 || true
    echo "no-egress: engine failed under the deny profile (rc=$rc)" >&2
    cp "$work/engine.log" "out/$asset-egress.txt"
    exit 1
  fi
  # A denied attempt the engine survived would still be egress: the sandbox
  # reports EPERM to the attempting process, so fail on that signature here.
  if grep -qi 'operation not permitted\|EPERM' "$work/engine.log"; then
    echo 'no-egress: engine log shows a denied network attempt' >&2
    cp "$work/engine.log" "out/$asset-egress.txt"
    exit 1
  fi
  cp "$work/engine.log" "out/$asset-egress.txt"
  sleep 2 # let the unified log flush
  # Opportunistic sweep only: findings fail the gate, absence proves nothing.
  /usr/bin/log show --style syslog --start "$start" --predicate 'eventMessage CONTAINS "deny" AND eventMessage CONTAINS "network"' >"$work/denies" 2>/dev/null || true
  if grep -E 'llama-cli|prima-|Python|python' "$work/denies"; then
    echo 'no-egress: unified log shows an engine network denial' >&2
    exit 1
  fi
  {
    echo '---- positive-control denial (stderr) ----'
    cat "$work/control"
  } >>"out/$asset-egress.txt"
  ;;
MINGW*|MSYS*)
  # Hosted Windows gate. A disposable dynamic WFP policy explicitly permits
  # exactly the pinned prima loopback ports (ring 5599-5605, engine data
  # 19000/19001, engine signal 19100/19101) for the exercise processes — WFP
  # filters loopback, Windows Firewall would not. A BLOCK veto wins over any
  # PERMIT, so the explicit loopback permits cannot weaken the denies below.
  # Controls retry a cold-start-tolerant deadline and, like an exercise
  # failure, dump the bound TCP table so the next failure names what the
  # exercise actually bound (IPv4 loopback vs IPv6). Enforcement is proven
  # by positive
  # controls whose verdicts are observed DIRECTLY in the guard's own stdout
  # (WSAEACCES / delivery proof) before the marker line; anything after the
  # marker is exercise output and must show no denial signature. The WFP
  # filters are keyed on exact executable paths, which scopes policy more
  # strongly than a post-hoc netstat owner check, so no netstat step follows.
  [[ ${GITHUB_ACTIONS:-} == true && ${RUNNER_ENVIRONMENT:-} == github-hosted ]] || {
    echo 'native gate requires a GitHub-hosted runner' >&2; exit 1;
  }
  case $(hostname | tr '[:upper:]' '[:lower:]') in *dragon*) echo 'Dragon native gate prohibited' >&2; exit 1;; esac
  command -v python3 >/dev/null || { echo 'missing python3' >&2; exit 1; }
  [[ $asset == prima-windows-amd64 ]] || { echo "unsupported Windows asset: $asset" >&2; exit 1; }
  binexe=$bin.exe
  [[ -f $binexe ]] || { echo "missing Windows CLI executable: $binexe" >&2; exit 1; }
  ringexe=$(pwd)/build/bin/prima-ring-test.exe
  [[ -f $ringexe ]] || { echo "missing Windows ring test: $ringexe" >&2; exit 1; }
  # Compile only on the hosted Windows image, using the verified MinGW clang
  # from the build leg (windows-compilers.py): no bare gcc, which names no
  # pinned package in the CLANG64 provision.
  cc=${BASHY_WINDOWS_CC:?verified Windows C compiler missing (windows-compilers.py)}
  "$cc" -std=c11 -Wall -Wextra -Werror -municode "$here/windows-net-guard.c" \
    -o "$work/net-guard.exe" -lfwpuclnt -lws2_32 -lrpcrt4 -liphlpapi
  # Native paths for CreateProcess and WFP app-id filtering.
  pyexe=$(python3 -c 'import sys; print(sys.executable)')
  pyexe=$(cygpath -m "$pyexe")
  binexe=$(cygpath -m "$binexe")
  ringexe=$(cygpath -m "$ringexe")
  model_win=$(cygpath -m "$work/model.gguf")
  exercise_win=$(cygpath -m "$here/exercise.py")
  # The guard is a native executable: every path it opens (pidfile) or
  # filters/spawns (app ids, command line) must be a Windows path.
  pid_win=$(cygpath -m "$work/pid")
  if "$work/net-guard.exe" --app "$pyexe" --app "$binexe" --app "$ringexe" -- \
      "$pid_win" "$pyexe" "$exercise_win" "$binexe" "$model_win" "$ringexe" \
      >"out/$asset-egress.txt" 2>&1; then
    echo 'no-egress: windows: exercise succeeded under the WFP deny policy'
  else
    rc=$?
    echo "no-egress: windows: exercise failed under the WFP deny policy (rc=$rc)" >&2
    cat "out/$asset-egress.txt" >&2 || true
    exit 1
  fi
  marker='WFP exercise filters installed; multi-process job active'
  controls=$(sed "/$marker/q" "out/$asset-egress.txt")
  exercise=$(sed -n "/$marker/,\$p" "out/$asset-egress.txt")
  # Positive controls directly observed: two TCP denials plus the IPv6 denial,
  # three UDP verdicts, and one allowed-loopback proof.
  for proof in 'WFP control denied family=2 type=1 port=443 WSAEACCES' \
               'WFP control denied family=2 type=1 port=5606 WSAEACCES' \
               'WFP control denied family=23 type=1 port=5599 WSAEACCES' \
               'WFP UDP control delivered family=2 port=5599' \
               'WFP UDP control denied family=2 port=5606' \
               'WFP UDP control denied family=23 port=5599' \
               'WFP configured loopback control allowed'; do
    grep -qF "$proof" <<<"$controls" || {
      echo "no-egress: windows: missing positive-control proof: $proof" >&2; exit 1; }
  done
  # All three exercise stages passed with no denial signature past the marker.
  [[ $(grep -c '^PASS:' <<<"$exercise") == 3 ]] || {
    echo 'no-egress: windows: exercise stages did not all PASS' >&2; exit 1; }
  grep -q 'exercise reaped; job closed; dynamic WFP policy removed' "out/$asset-egress.txt" || {
    echo 'no-egress: windows: guard did not confirm policy removal' >&2; exit 1; }
  if grep -Ei 'WSAEACCES|WFP gate failure|operation not permitted|EPERM' <<<"$exercise"; then
    echo 'no-egress: windows: exercise output shows a denied network attempt' >&2; exit 1
  fi
  ;;
*) exit 1 ;;
esac
