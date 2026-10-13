#!/usr/bin/env bash
# no-egress-test.sh BINARY
#
# Release gate, run BEFORE signing: start the built `weed server` stack
# (master + volume + filer + S3 gateway) bound to 127.0.0.1 with usage
# reporting explicitly disabled and the shipped no-tracking.env / otel.env
# environment, exercise the S3 gateway and the master UI over loopback, let it
# idle, and FAIL on any outbound connection attempt other than loopback to its
# own ports or to 127.0.0.1:31415.
#
# seaweedfs is the one tool in builds/ whose tracking defaults to ON:
# `weed server` defines -master.telemetry with a default of true and
# -master.telemetry.url defaulting to https://telemetry.seaweedfs.com/api/collect
# (weed/command/server.go:112-113), and `weed master` defines the same pair as
# -telemetry / -telemetry.url (weed/command/master.go:114-115). The collector
# reports once 1 GiB is stored and then on an interval
# (weed/telemetry/collector.go:84), so an idle probe would not catch it by
# waiting — which is why the flag, not the observation, is the control. This
# gate proves the flag is honoured: the stack runs with -master.telemetry=false
# and nothing leaves the host.
#
# Linux (authoritative): the server runs in a fresh network namespace
# (only `lo`) under `strace -f -e trace=connect,sendto,sendmsg,sendmmsg`,
# so every attempt — including ones the namespace makes fail
# (ENETUNREACH) and DNS — is recorded and judged. Needs root via sudo
# (unshare --net), strace, setpriv, ip, curl.
#
# macOS: the server runs under sandbox-exec with all network-outbound
# denied except the loopback ports it needs (and the mDNSResponder socket
# denied, so a DNS lookup is an attempt too); any Sandbox deny for the
# process in the unified log is a failure.
#
# Env: EGRESS_PORT (default 15333, the S3 gateway; master/volume/filer take
#      the four ports above it), EGRESS_OBSERVE_SECONDS (default 30),
#      EGRESS_READY_SECONDS (default 180).
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
# The ONE shared strace verdict (builds/_shared/egress-verdict.sh).
# shellcheck source=/dev/null
. "$here/egress-verdict.sh"
bin=${1:?usage: no-egress-test.sh BINARY}
bin=$(cd "$(dirname "$bin")" && pwd)/$(basename "$bin")
port=${EGRESS_PORT:-15333}
observe=${EGRESS_OBSERVE_SECONDS:-30}
ready_max=${EGRESS_READY_SECONDS:-180}

die() { echo "no-egress: FAIL: $*" >&2; exit 1; }
[[ -x $bin ]] || die "$bin is not executable"

command -v curl >/dev/null || die "curl is not on PATH (fail-closed)"

work=$(mktemp -d "${TMPDIR:-/tmp}/seaweedfs-egress.XXXXXX")
chmod 755 "$work"
mkdir -p "$work/data" "$work/home"

# `weed server` runs four services, each on its own port; they talk to each
# other over loopback, so every one of these is an allowed destination.
master_port=$((port + 1))
volume_port=$((port + 2))
filer_port=$((port + 3))
# Each service's gRPC listener defaults to its HTTP port + 10000 (seaweedfs's
# fixed convention, weed/pb/grpc_client_server.go:558), and the services dial
# each other there, so those are loopback destinations too.
s3_grpc_port=$((port + 10000))
master_grpc_port=$((master_port + 10000))
volume_grpc_port=$((volume_port + 10000))
filer_grpc_port=$((filer_port + 10000))

# The server's whole environment: no inherited proxies, tokens or endpoints.
envfile=$work/server.env
{
	echo "HOME=$work/home"
	echo "PATH=/usr/bin:/bin"
	grep -E '^[A-Z_]+=' "$here/no-tracking.env"
	grep -E '^[A-Z_]+=' "$here/otel.env"
} >"$envfile"

health=http://127.0.0.1:$port/
# bashy's local OTLP collector (yoke external/internal/serviceenv
# LocalCollectorHostPort). Loopback to this port is the ONLY destination
# besides the service's own ports that the verdict below tolerates.
otlp_port=31415

cat >"$work/run.sh" <<RUN
set -uo pipefail
# Same argv shape yoke's external/seaweedfs launcher uses (a loopback-bound
# server with the S3 gateway), plus the no-tracking flag this gate exists for.
# -ip is pinned to 127.0.0.1 as well: left to its default it is
# util.DetectedHostAddress(), which picks whatever interface address it finds
# and would make the services address each other off-loopback.
exec '$bin' server \\
	-dir='$work/data' \\
	-ip=127.0.0.1 \\
	-ip.bind=127.0.0.1 \\
	-master.port=$master_port \\
	-volume.port=$volume_port \\
	-filer -filer.port=$filer_port \\
	-s3 -s3.port=$port \\
	-volume.max=1 \\
	-master.telemetry=false
RUN
chmod +x "$work/run.sh"

probe() {
	# Loopback traffic over the S3 gateway, the master's cluster API and the
	# filer; the answers do not matter, only that weed stays local.
	curl -sS -o /dev/null "http://127.0.0.1:$port/" || true
	curl -sS -o /dev/null "http://127.0.0.1:$port/bashy-probe/" || true
	curl -sS -o /dev/null "http://127.0.0.1:$master_port/cluster/status" || true
	curl -sS -o /dev/null "http://127.0.0.1:$master_port/dir/status" || true
	curl -sS -o /dev/null "http://127.0.0.1:$filer_port/" || true
}

wait_ready() { # PID
	local i
	for ((i = 0; i < ready_max; i++)); do
		kill -0 "$1" 2>/dev/null || return 1
		# The S3 gateway answers once the stack is up. It replies 403 to an
		# unsigned request rather than 200, so any HTTP status counts as ready
		# (a readiness probe must not require authorization).
		[[ $(curl -sS -o /dev/null -w '%{http_code}' "$health" || true) != 000 ]] && return 0
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
port=$port master_port=$master_port filer_port=$filer_port
ready_max=$ready_max health='$health'
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
	egress_verdict "$log" "$port" "$master_port" "$volume_port" "$filer_port" \
		"$s3_grpc_port" "$master_grpc_port" "$volume_grpc_port" "$filer_grpc_port" "$otlp_port" ||
		die "outbound connection attempt(s) other than the loopback service ports and $otlp_port"
}

darwin() {
	local profile=$work/no-egress.sb start pid p
	{
		echo '(version 1)'
		echo '(allow default)'
		echo '(deny network-outbound (remote ip "*:*"))'
		for p in "$port" "$master_port" "$volume_port" "$filer_port" \
			"$s3_grpc_port" "$master_grpc_port" "$volume_grpc_port" "$filer_grpc_port" "$otlp_port"; do
			echo "(allow network-outbound (remote ip \"localhost:$p\"))"
		done
		echo '(deny network-outbound (remote unix-socket (path-literal "/private/var/run/mDNSResponder")))'
	} >"$profile"
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
		grep -E "weed\(|$(basename "$bin")\(" || true)
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
