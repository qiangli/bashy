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
# Kopia's one documented outbound call is its periodic GitHub release query,
# and no-tracking.env turns it off with KOPIA_CHECK_FOR_UPDATES=false rather
# than a flag: `--[no-]check-for-updates` is not a global flag at this tag, so
# passing it before the subcommand is an "unknown long flag" and both
# invocations below simply fail. The envar reaches them through the server
# environment assembled above, and the gate proves the query is really off.
# Both invocations also carry --no-persist-credentials: at v0.23.1 credential
# persistence is on by default, and macOS uses Keychain by default. The
# repository password comes only from KOPIA_PASSWORD, so this gate stays
# headless. --otlp-trace IS a global flag and remains before the server
# subcommand; otel.env points it only at 127.0.0.1:31415 — the single
# non-service destination the verdict tolerates.
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
repo_create_log=
server_log=

dump_log() { # LABEL PATH
	local label=$1 path=$2
	echo "----- no-egress: $label -----" >&2
	if [[ -n $path && -f $path ]]; then
		if [[ -s $path ]]; then
			cat "$path" >&2
		else
			echo "(empty)" >&2
		fi
	else
		echo "(not created)" >&2
	fi
	echo "----- no-egress: end $label -----" >&2
}

dump_failure_logs() {
	dump_log "repository create output" "$repo_create_log"
	dump_log "server stdout/stderr" "$server_log"
}

die() {
	trap - ERR
	echo "no-egress: FAIL: $*" >&2
	dump_failure_logs
	exit 1
}

on_error() {
	local status=$?
	trap - ERR
	echo "no-egress: FAIL: unexpected command failure (exit $status)" >&2
	dump_failure_logs
	exit "$status"
}
trap on_error ERR
[[ -x $bin ]] || die "$bin is not executable"

command -v curl >/dev/null || die "curl is not on PATH (fail-closed)"

work=$(mktemp -d "${TMPDIR:-/tmp}/kopia-egress.XXXXXX")
chmod 755 "$work"
mkdir -p "$work/data" "$work/home"
repo_create_log=$work/repository-create.log
server_log=$work/server.log

# The server's whole environment: no inherited proxies, tokens or endpoints.
envfile=$work/server.env
{
	echo "HOME=$work/home"
	echo "PATH=/usr/bin:/bin"
	grep -E '^[A-Z_]+=' "$here/no-tracking.env"
	grep -E '^[A-Z_]+=' "$here/otel.env"
	echo "KOPIA_PASSWORD=bashy-no-egress-probe"
	echo "KOPIA_CHECK_FOR_UPDATES=false"
} >"$envfile"

health=http://127.0.0.1:$port/
# bashy's local OTLP collector (yoke external/internal/serviceenv
# LocalCollectorHostPort). Loopback to this port is the ONLY destination
# besides the service's own port that the verdict below tolerates.
otlp_port=31415

cat >"$work/run.sh" <<RUN
set -euo pipefail
# Keep the repository password in the environment and explicitly disable
# persistence on both invocations. On macOS this avoids Keychain access.
cfg='$work/kopia.config'
'$bin' --config-file="\$cfg" --no-persist-credentials \\
	repository create filesystem --path='$work/repo' \\
	>'$repo_create_log' 2>&1
exec '$bin' --config-file="\$cfg" --no-persist-credentials --otlp-trace \\
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
	env -i \$(cat '$envfile') bash '$work/run.sh' >'$server_log' 2>&1 &
tracer=\$!
if ! wait_ready \$tracer; then
	echo "server did not become ready on $health" >&2
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
	env -i $(cat "$envfile") sandbox-exec -f "$profile" bash "$work/run.sh" >"$server_log" 2>&1 &
	pid=$!
	if ! wait_ready "$pid"; then
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
