#!/usr/bin/env bash
# no-egress-test.sh BINARY
#
# Release gate, run BEFORE signing: start the built tuwunel bound to
# 127.0.0.1 with the shipped no-tracking config (bashy-build/tuwunel.toml:
# registration and federation off, sentry/jaeger off) and the otel.env
# environment, exercise it over loopback, let it idle, and FAIL on any
# outbound connection attempt other than loopback to its own port.
#
# Linux (authoritative): the server runs in a fresh network namespace (only
# `lo`) under `strace -f -e trace=connect,sendto,sendmsg,sendmmsg`, so every
# attempt — including ones the namespace makes fail (ENETUNREACH) and DNS —
# is recorded and judged. Needs root via sudo (unshare --net), strace,
# setpriv, ip, curl.
#
# macOS: the server runs under sandbox-exec with all network-outbound denied
# except localhost:PORT (and the mDNSResponder socket denied, so a DNS lookup
# is an attempt too); any Sandbox deny for the process in the unified log is
# a failure.
#
# Env: EGRESS_PORT (default 18448), EGRESS_OBSERVE_SECONDS (default 30),
#      EGRESS_READY_SECONDS (default 180).
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
bin=${1:?usage: no-egress-test.sh BINARY}
bin=$(cd "$(dirname "$bin")" && pwd)/$(basename "$bin")
port=${EGRESS_PORT:-18448}
observe=${EGRESS_OBSERVE_SECONDS:-30}
ready_max=${EGRESS_READY_SECONDS:-180}

die() {
	echo "no-egress: FAIL: $*" >&2
	# Always show the server's own output on failure (startup errors live here).
	if [[ -n ${work:-} && -s $work/server.log ]]; then
		echo "---- server.log (last 80 lines) ----" >&2
		tail -n 80 "$work/server.log" >&2 || true
	fi
	exit 1
}
[[ -x $bin ]] || die "$bin is not executable"

work=$(mktemp -d "${TMPDIR:-/tmp}/tuwunel-egress.XXXXXX")
chmod 755 "$work"
mkdir -p "$work/db" "$work/home"

# The server's whole environment: no inherited proxies or tokens.
envfile=$work/server.env
{
	echo "HOME=$work/home"
	echo "PATH=/usr/bin:/bin"
	echo "TUWUNEL_CONFIG=$here/tuwunel.toml"
	echo "TUWUNEL_SERVER_NAME=egress-test.invalid"
	echo "TUWUNEL_PORT=$port"
	echo "TUWUNEL_DATABASE_PATH=$work/db"
	grep -E '^[A-Z_]+=' "$here/otel.env"
} >"$envfile"

probe() {
	# Loopback traffic that exercises the client, federation and well-known
	# surfaces; the answers do not matter, only that the server stays local.
	local base=http://127.0.0.1:$port
	curl -sS -o /dev/null "$base/_matrix/client/versions" || true
	curl -sS -o /dev/null "$base/_matrix/federation/v1/version" || true
	curl -sS -o /dev/null "$base/.well-known/matrix/client" || true
	curl -sS -o /dev/null -X POST -H 'Content-Type: application/json' \
		-d '{"username":"probe","password":"probe-password","auth":{"type":"m.login.dummy"}}' \
		"$base/_matrix/client/v3/register" || true
	curl -sS -o /dev/null "$base/_matrix/client/v3/publicRooms" || true
}

wait_ready() { # PID
	local i
	for ((i = 0; i < ready_max; i++)); do
		kill -0 "$1" 2>/dev/null || return 1
		curl -fsS -o /dev/null "http://127.0.0.1:$port/_matrix/client/versions" && return 0
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
set -euo pipefail
ip link set lo up
$(declare -f probe wait_ready)
port=$port ready_max=$ready_max
strace -f -qq -e trace=connect,sendto,sendmsg,sendmmsg -e signal=none -o '$log' \\
	setpriv --reuid=$ruid --regid=$rgid --init-groups -- \\
	env -i \$(cat '$envfile') '$bin' >'$work/server.log' 2>&1 &
tracer=\$!
if ! wait_ready \$tracer; then
	echo "server did not become ready on 127.0.0.1:$port" >&2
	tail -n 50 '$work/server.log' >&2 || true
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

	local bad=() line fam host p
	while IFS= read -r line; do
		case $line in
		*sa_family=AF_INET6*)
			fam=inet6
			host=$(sed -n 's/.*inet_pton(AF_INET6, "\([^"]*\)".*/\1/p' <<<"$line")
			p=$(sed -n 's/.*sin6_port=htons(\([0-9]*\)).*/\1/p' <<<"$line")
			;;
		*sa_family=AF_INET,*)
			fam=inet
			host=$(sed -n 's/.*sin_addr=inet_addr("\([^"]*\)").*/\1/p' <<<"$line")
			p=$(sed -n 's/.*sin_port=htons(\([0-9]*\)).*/\1/p' <<<"$line")
			;;
		*sa_family=AF_UNIX*)
			# Local IPC (glibc nscd, resolver sockets) is not network egress;
			# only AF_INET/AF_INET6 connects to non-loopback addresses count.
			continue
			;;
		*) continue ;; # AF_UNSPEC (UDP disconnect), AF_NETLINK, no address
		esac
		case $host in
		127.0.0.1 | ::1 | ::ffff:127.0.0.1) [[ $p == "$port" ]] && continue ;;
		esac
		bad+=("$fam $host:$p :: $line")
	done <"$log"

	if ((${#bad[@]})); then
		printf 'no-egress: outbound attempt: %s\n' "${bad[@]}" >&2
		die "${#bad[@]} outbound connection attempt(s) other than 127.0.0.1:$port"
	fi
	echo "no-egress: linux: $(grep -c . "$log") traced socket syscalls, none leave loopback:$port OK"
}

darwin() {
	local profile=$work/no-egress.sb start pid
	cat >"$profile" <<SB
(version 1)
(allow default)
(deny network-outbound (remote ip "*:*"))
(allow network-outbound (remote ip "localhost:$port"))
(deny network-outbound (remote unix-socket (path-literal "/private/var/run/mDNSResponder")))
SB
	start=$(date '+%Y-%m-%d %H:%M:%S')
	# shellcheck disable=SC2046
	env -i $(cat "$envfile") sandbox-exec -f "$profile" "$bin" >"$work/server.log" 2>&1 &
	pid=$!
	if ! wait_ready "$pid"; then
		tail -n 50 "$work/server.log" >&2 || true
		kill -KILL "$pid" 2>/dev/null || true
		die "server did not become ready on 127.0.0.1:$port"
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
		grep -E "tuwunel\(|$(basename "$bin")\(" || true)
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
