#!/usr/bin/env bash
# no-egress-test.sh BINARY
#
# Release gate, run BEFORE signing: exercise the built act_runner, then start
# its cache server bound to 127.0.0.1 with the shipped no-tracking baseline
# (bashy-build/config.yaml) and the no-tracking.env / otel.env environment,
# drive it over loopback, let it idle, and FAIL on any outbound connection
# attempt other than loopback to its own port or to 127.0.0.1:31415.
#
# act_runner has no usage reporting and no update check: everything it talks to
# is configured — the Gitea instance it registers with, the Docker daemon it
# runs jobs on. That makes `daemon` and `register` the wrong subjects for this
# gate (both are SUPPOSED to reach their configured instance, and neither
# starts without one), so the gate runs the subcommands that need nothing
# external: --version, `config generate`, and `cache-server`, which is a real
# long-running HTTP listener and the one part of the runner with an
# UNCONFIGURED outbound call to remove.
#
# That call is why cache.host is pinned in config.yaml. Left empty, the cache
# server asks the kernel which local address would reach the internet by
# dialling 8.8.8.8:80 (act/common/outbound_ip.go:24, reached from
# act/artifactcache/handler.go:159 when Options.OutboundIP is unset). It is a
# real UDP connect to a third party on every start, it is not a lookup the
# verdict would forgive as local IPC, and it is exactly what this gate catches
# if a future pin or config drift reintroduces it.
#
# Linux (authoritative): everything runs in a fresh network namespace
# (only `lo`) under `strace -f -e trace=connect,sendto,sendmsg,sendmmsg`,
# so every attempt — including ones the namespace makes fail
# (ENETUNREACH) and DNS — is recorded and judged. Needs root via sudo
# (unshare --net), strace, setpriv, ip, curl.
#
# macOS: the same run happens under sandbox-exec with all network-outbound
# denied except localhost:PORT and localhost:31415 (and the mDNSResponder
# socket denied, so a DNS lookup is an attempt too); any Sandbox deny for
# the process in the unified log is a failure.
#
# Env: EGRESS_PORT (default 18088), EGRESS_OBSERVE_SECONDS (default 30),
#      EGRESS_READY_SECONDS (default 180).
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
# The ONE shared strace verdict (builds/_shared/egress-verdict.sh).
# shellcheck source=/dev/null
. "$here/egress-verdict.sh"
bin=${1:?usage: no-egress-test.sh BINARY}
bin=$(cd "$(dirname "$bin")" && pwd)/$(basename "$bin")
port=${EGRESS_PORT:-18088}
observe=${EGRESS_OBSERVE_SECONDS:-30}
ready_max=${EGRESS_READY_SECONDS:-180}

die() { echo "no-egress: FAIL: $*" >&2; exit 1; }
[[ -x $bin ]] || die "$bin is not executable"

command -v curl >/dev/null || die "curl is not on PATH (fail-closed)"

work=$(mktemp -d "${TMPDIR:-/tmp}/act_runner-egress.XXXXXX")
chmod 755 "$work"
mkdir -p "$work/cache" "$work/home"

# The runner's whole environment: no inherited proxies, tokens or endpoints.
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
# besides the cache server's own port that the verdict below tolerates.
otlp_port=31415

# Seed the no-tracking config baseline (loopback host + fixed port + cache
# dir). The cache server refuses to start without cache.external_secret, so
# the probe gets a throwaway one rather than a shipped constant.
secret=$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')
sed -e "s|{{DATA_DIR}}|$work/cache|g" \
	-e "s|{{ADDR}}|127.0.0.1|g" \
	-e "s|{{PORT}}|$port|g" \
	-e "s|{{SECRET}}|$secret|g" \
	"$here/config.yaml" >"$work/config.yaml"

cat >"$work/run.sh" <<RUN
set -uo pipefail
# Representative invocations first: the ones that need nothing external. Each
# is traced like the server, so a phone-home in any of them fails the gate.
'$bin' --version
'$bin' config generate >'$work/generated-config.yaml'
'$bin' --config '$work/config.yaml' config get cache.host
# Then the long-running listener the readiness probe and the idle window watch.
exec '$bin' --config '$work/config.yaml' cache-server
RUN
chmod +x "$work/run.sh"

probe() {
	# Loopback traffic over the cache API and the internal registration
	# endpoint; the answers do not matter (both reject an unauthorized probe),
	# only that the runner stays local.
	local base=http://127.0.0.1:$port
	curl -sS -o /dev/null "$base/" || true
	curl -sS -o /dev/null "$base/_apis/artifactcache/cache?keys=bashy-probe&version=v1" || true
	curl -sS -o /dev/null -X POST "$base/_apis/artifactcache/caches" || true
	curl -sS -o /dev/null -X POST "$base/_internal/register" || true
}

wait_ready() { # PID
	local i
	for ((i = 0; i < ready_max; i++)); do
		kill -0 "$1" 2>/dev/null || return 1
		# The cache server answers 404 at / and 401 on its API, so any HTTP
		# status means it is listening; a readiness probe must not need
		# authorization.
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
port=$port ready_max=$ready_max health='$health'
strace -f -qq -e trace=connect,sendto,sendmsg,sendmmsg -e signal=none -o '$log' \\
	setpriv --reuid=$ruid --regid=$rgid --init-groups -- \\
	env -i \$(cat '$envfile') bash '$work/run.sh' >'$work/server.log' 2>&1 &
tracer=\$!
if ! wait_ready \$tracer; then
	echo "cache server did not become ready on $health" >&2
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
	sudo unshare --net -- bash "$inner" || die "runner run failed (see above)"
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
		die "cache server did not become ready on $health"
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
		grep -E "act_runner\(|gitea-runner\(|$(basename "$bin")\(" || true)
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
