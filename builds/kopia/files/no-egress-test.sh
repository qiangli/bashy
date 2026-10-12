#!/usr/bin/env bash
# no-egress-test.sh BINARY
#
# Release gate, run BEFORE signing: create a filesystem repository and start
# the built kopia repository server bound to 127.0.0.1 with the shipped
# no-tracking environment (bashy-build/no-tracking.env: proxies cleared) and
# otel.env (exporter pinned at bashy's local collector), exercise the server
# API over loopback, let it idle, and FAIL on any outbound connection attempt
# other than loopback to its own port or to 127.0.0.1:31415.
#
# Both kopia invocations carry --no-check-for-updates, which is kopia's one
# documented outbound call (a GitHub release query); the gate proves it is
# really off. --otlp-trace is kopia's supported OTLP exporter, and otel.env
# points it only at 127.0.0.1:31415 — the single non-service destination the
# verdict tolerates.
#
# Linux (authoritative): the server runs in a fresh network namespace
# (only `lo`) under `strace -f -e trace=connect,sendto,sendmsg,sendmmsg`,
# so every attempt — including ones the namespace makes fail
# (ENETUNREACH) and DNS — is recorded and judged. Needs root via sudo
# (unshare --net), strace, setpriv, ip, curl.
#
# macOS: the server runs under sandbox-exec with all network-outbound
# denied except localhost:PORT and localhost:31415 (and the mDNSResponder
# socket denied, so a DNS lookup is an attempt too); any Sandbox deny for
# the process in the unified log is a failure.
#
# Env: EGRESS_PORT (default 15151), EGRESS_OBSERVE_SECONDS (default 30),
#      EGRESS_READY_SECONDS (default 180).
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
# The ONE shared strace verdict (builds/_shared/egress-verdict.sh).
# shellcheck source=/dev/null
. "$here/egress-verdict.sh"
bin=${1:?usage: no-egress-test.sh BINARY}
bin=$(cd "$(dirname "$bin")" && pwd)/$(basename "$bin")
port=${EGRESS_PORT:-15151}
observe=${EGRESS_OBSERVE_SECONDS:-30}
ready_max=${EGRESS_READY_SECONDS:-180}

die() { echo "no-egress: FAIL: $*" >&2; exit 1; }
[[ -x $bin ]] || die "$bin is not executable"

command -v curl >/dev/null || die "curl is not on PATH (fail-closed)"

work=$(mktemp -d "${TMPDIR:-/tmp}/kopia-egress.XXXXXX")
chmod 755 "$work"
mkdir -p "$work/data" "$work/home"

# The server's whole environment: no inherited proxies, tokens or endpoints.
envfile=$work/server.env
{
	echo "HOME=$work/home"
	echo "PATH=/usr/bin:/bin"
	grep -E '^[A-Z_]+=' "$here/no-tracking.env"
	grep -E '^[A-Z_]+=' "$here/otel.env"
	echo "KOPIA_PASSWORD=bashy-no-egress-probe"
} >"$envfile"

health=http://127.0.0.1:$port/
# bashy's local OTLP collector (yoke external/internal/serviceenv
# LocalCollectorHostPort). Loopback to this port is the ONLY destination
# besides the service's own port that the verdict below tolerates.
otlp_port=31415

cat >"$work/run.sh" <<RUN
set -uo pipefail
# Same argv shape yoke's external/kopia launcher uses (kopiaPrivacyArgs +
# a filesystem repo + an insecure loopback server). Flag reference:
# https://kopia.io/docs/reference/command-line/flags/
cfg='$work/kopia.config'
'$bin' --config-file="\$cfg" --no-check-for-updates \\
	repository create filesystem --path='$work/repo'
exec '$bin' --config-file="\$cfg" --no-check-for-updates --otlp-trace \\
	server start --address=127.0.0.1:$port --insecure --without-password
RUN
chmod +x "$work/run.sh"

probe() {
	# Loopback traffic over the server root and its status API; the answers do
	# not matter, only that the server stays local.
	local base=http://127.0.0.1:$port
	curl -sS -o /dev/null "$base/" || true
	curl -sS -o /dev/null "$base/api/v1/repo/status" || true
	curl -sS -o /dev/null "$base/api/v1/sources" || true
}

wait_ready() { # PID
	local i
	for ((i = 0; i < ready_max; i++)); do
		kill -0 "$1" 2>/dev/null || return 1
		curl -fsS -o /dev/null "$health" && return 0
		sleep 1
	done
	return 1
}

linux() {
	local log=$work/strace.log inner=$work/inner.sh
	local ruid rgid
	ruid=$(id -u) rgid=$(id -g)
	# Everything below runs inside the new network namespace.
	cat >"$inner" <<INNER
set -uo pipefail
ip link set lo up
$(declare -f probe wait_ready)
port=$port ready_max=$ready_max health='$health'
strace -f -qq -e trace=connect,sendto,sendmsg,sendmmsg -e signal=none -o '$log' \\
	setpriv --reuid=$ruid --regid=$rgid --init-groups -- \\
	env -i \$(cat '$envfile') bash '$work/run.sh' >'$work/server.log' 2>&1 &
tracer=\$!
if ! wait_ready \$tracer; then
	echo "server did not become ready on $health" >&2
	tail -n 60 '$work/server.log' >&2 || true
	pkill -KILL -P \$tracer || true
	exit 1
fi
probe
sleep $observe
probe
pkill -TERM -P \$tracer || true
wait \$tracer || true
INNER
	sudo unshare --net -- bash "$inner" || die "server run failed (see above)"
	[[ -s $log ]] || die "no strace log recorded"

	# The verdict itself is the ONE shared copy: bashy-build/egress-verdict.sh
	# (builds/_shared/egress-verdict.sh). AF_UNIX is local IPC, never egress.
	egress_verdict "$log" "$port" "$otlp_port" ||
		die "outbound connection attempt(s) other than loopback:{$port,$otlp_port}"
}

darwin() {
	local profile=$work/no-egress.sb start pid
	cat >"$profile" <<SB
(version 1)
(allow default)
(deny network-outbound (remote ip "*:*"))
(allow network-outbound (remote ip "localhost:$port"))
(allow network-outbound (remote ip "localhost:$otlp_port"))
(deny network-outbound (remote unix-socket (path-literal "/private/var/run/mDNSResponder")))
SB
	start=$(date '+%Y-%m-%d %H:%M:%S')
	# shellcheck disable=SC2046
	env -i $(cat "$envfile") sandbox-exec -f "$profile" bash "$work/run.sh" >"$work/server.log" 2>&1 &
	pid=$!
	if ! wait_ready "$pid"; then
		tail -n 60 "$work/server.log" >&2 || true
		kill -KILL "$pid" 2>/dev/null || true
		die "server did not become ready on $health"
	fi
	probe
	sleep "$observe"
	probe
	kill -TERM "$pid" 2>/dev/null || true
	wait "$pid" || true
	sleep 2 # let the unified log flush

	local denies
	denies=$(log show --style syslog --start "$start" \
		--predicate 'sender == "Sandbox" AND eventMessage CONTAINS "network-outbound"' 2>/dev/null |
		grep -E "kopia\(|$(basename "$bin")\(" || true)
	if [[ -n $denies ]]; then
		echo "$denies" >&2
		die "sandbox denied outbound network attempt(s)"
	fi
	echo "no-egress: darwin: no sandbox network-outbound denies for $(basename "$bin") OK"
}

case $(uname -s) in
Linux) linux ;;
Darwin) darwin ;;
*) die "unsupported OS $(uname -s)" ;;
esac
