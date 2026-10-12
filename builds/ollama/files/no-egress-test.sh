#!/usr/bin/env bash
# no-egress-test.sh ARCHIVE.tgz
#
# Extract the exact release archive, start `ollama serve` on loopback with
# cloud features disabled, exercise /api/version and /api/tags, then idle.
# Linux traces the whole process tree in a network namespace and fails only on
# AF_INET/AF_INET6 attempts whose destination is not loopback. AF_UNIX,
# AF_NETLINK and AF_UNSPEC are deliberately outside the verdict. macOS denies
# non-loopback outbound traffic with sandbox-exec and fails on any matching
# sandbox denial.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
archive=${1:?usage: no-egress-test.sh ARCHIVE.tgz}
archive=$(cd "$(dirname "$archive")" && pwd)/$(basename "$archive")
port=${EGRESS_PORT:-18449}
observe=${EGRESS_OBSERVE_SECONDS:-30}
ready_max=${EGRESS_READY_SECONDS:-180}

die() {
	echo "no-egress: FAIL: $*" >&2
	if [[ -n ${work:-} && -s $work/server.log ]]; then
		echo "---- server.log (last 80 lines) ----" >&2
		tail -n 80 "$work/server.log" >&2 || true
	fi
	exit 1
}
[[ -f $archive ]] || die "$archive is not a file"

work=$(mktemp -d "${TMPDIR:-/tmp}/ollama-egress.XXXXXX")
chmod 755 "$work"
mkdir -p "$work/root" "$work/home" "$work/models"
tar -xzf "$archive" -C "$work/root"
bin=$work/root/bin/ollama
[[ -x $bin ]] || die "archive has no executable bin/ollama"
[[ -x $work/root/lib/ollama/llama-server ]] || die "archive has no lib/ollama/llama-server"

envfile=$work/server.env
{
	echo "HOME=$work/home"
	echo "PATH=/usr/bin:/bin"
	echo "OLLAMA_HOST=127.0.0.1:$port"
	echo "OLLAMA_MODELS=$work/models"
	grep -E '^[A-Z_]+=' "$here/no-tracking.env"
	grep -E '^[A-Z_]+=' "$here/otel.env"
} >"$envfile"

probe() {
	local base=http://127.0.0.1:$port body
	body=$(curl -fsS "$base/api/version")
	[[ $body == *'"version"'* ]] || return 1
	body=$(curl -fsS "$base/api/tags")
	[[ $body == *'"models"'* ]] || return 1
}

wait_ready() { # PID
	local i
	for ((i = 0; i < ready_max; i++)); do
		kill -0 "$1" 2>/dev/null || return 1
		curl -fsS -o /dev/null "http://127.0.0.1:$port/api/version" && return 0
		sleep 1
	done
	return 1
}

linux() {
	local log=$work/strace.log inner=$work/inner.sh
	local ruid rgid
	ruid=$(id -u) rgid=$(id -g)
	cat >"$inner" <<INNER
set -euo pipefail
ip link set lo up
$(declare -f probe wait_ready)
port=$port ready_max=$ready_max
strace -f -qq -e trace=connect,sendto,sendmsg,sendmmsg -e signal=none -o '$log' \\
	setpriv --reuid=$ruid --regid=$rgid --init-groups -- \\
	env -i \$(cat '$envfile') '$bin' serve >'$work/server.log' 2>&1 &
tracer=\$!
if ! wait_ready \$tracer; then
	echo "server did not become ready on 127.0.0.1:$port" >&2
	tail -n 80 '$work/server.log' >&2 || true
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
		*) continue ;; # AF_UNIX, AF_UNSPEC, AF_NETLINK, or no address
		esac
		case $host in
		127.* | ::1 | ::ffff:127.*) continue ;;
		esac
		bad+=("$fam $host:$p :: $line")
	done <"$log"

	if ((${#bad[@]})); then
		printf 'no-egress: outbound attempt: %s\n' "${bad[@]}" >&2
		die "${#bad[@]} non-loopback AF_INET/AF_INET6 connection attempt(s)"
	fi
	echo "no-egress: linux: $(grep -c . "$log") traced socket syscalls; no non-loopback AF_INET/AF_INET6 attempts OK"
}

darwin() {
	local profile=$work/no-egress.sb start pid
	cat >"$profile" <<SB
(version 1)
(allow default)
(deny network-outbound (remote ip "*:*"))
(allow network-outbound (remote ip "localhost:*"))
(deny network-outbound (remote unix-socket (path-literal "/private/var/run/mDNSResponder")))
SB
	start=$(date '+%Y-%m-%d %H:%M:%S')
	# shellcheck disable=SC2046
	env -i $(cat "$envfile") sandbox-exec -f "$profile" "$bin" serve >"$work/server.log" 2>&1 &
	pid=$!
	if ! wait_ready "$pid"; then
		kill -KILL "$pid" 2>/dev/null || true
		die "server did not become ready on 127.0.0.1:$port"
	fi
	probe
	sleep "$observe"
	probe
	kill -TERM "$pid" 2>/dev/null || true
	wait "$pid" || true
	sleep 2

	local denies
	denies=$(log show --style syslog --start "$start" \
		--predicate 'sender == "Sandbox" AND eventMessage CONTAINS "network-outbound"' 2>/dev/null |
		grep -E 'ollama\(|llama-server\(' || true)
	if [[ -n $denies ]]; then
		echo "$denies" >&2
		die "sandbox denied non-loopback outbound network attempt(s)"
	fi
	echo "no-egress: darwin: no non-loopback sandbox network-outbound denies for ollama OK"
}

case $(uname -s) in
Linux) linux ;;
Darwin) darwin ;;
*) die "unsupported OS $(uname -s)" ;;
esac
