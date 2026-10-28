#!/usr/bin/env bash
# no-egress-test.sh ASSET_TARBALL
#
# Release gate, run BEFORE signing, once per MATRIX leg. ASSET_TARBALL is
# out/<matrix.asset> (builds/klatch/tool.env EGRESS_TARGET) — either a
# server tarball (klatch + net/transform scripts) or the web tarball
# (klatch-web-*, static vite output, no executable code at all).
#
# Web leg: nothing in the tarball executes during this build, so there is no
# process to trace — the gate is a sanity check that the archive is a
# non-empty, path-safe tree of plain files, not a strace exercise.
#
# Server leg: extract the tarball, start the built klatch binary bound to
# 127.0.0.1 with the shipped no-tracking.env/otel.env environment, exercise
# a handful of unauthenticated HTTP routes over loopback, let it idle, and
# FAIL on any outbound connection attempt other than loopback to its own
# port or to 127.0.0.1:31415.
#
# klatch has no update checker and no usage reporting (tool.env
# RELEASE_NOTE); its only outbound-capable dependency is webpush-go, which
# this gate never exercises (it requires a configured account and a real
# push endpoint) — see files/no-tracking.env.
#
# Linux (authoritative, and the only platform this gate runs the runtime
# check on — see #Darwin below): the server runs in a fresh network
# namespace (only `lo`) under
# `strace -f -e trace=connect,sendto,sendmsg,sendmmsg`, so every attempt —
# including ones the namespace makes fail (ENETUNREACH) and DNS — is
# recorded and judged. Needs root via sudo (unshare --net), strace, setpriv,
# ip, curl, tar.
#
# Darwin (klatch-$UPSTREAM_TAG-darwin-{arm64,amd64}.tar.gz server legs): this
# gate is SKIPPED, not failed. The verdict function it shares with every
# other tool (builds/_shared/egress-verdict.sh, egress_verdict) parses
# strace(1)'s syscall log line shapes (sa_family=AF_INET6, sin_port=htons(),
# ...), which has no macOS equivalent here — unlike zot's no-egress-test.sh,
# which implements a SEPARATE sandbox-exec-based darwin() check, klatch's
# darwin legs have not had that written yet (builds/README.md #No-egress
# gate / builds/klatch/README.md #No-egress gate records this as a known
# gap, not a silent one). The darwin tarballs still go through every OTHER
# gate (pin, build, govulncheck/yarn-audit, sign+attest).
#
# Env: EGRESS_PORT (default 15000), EGRESS_OBSERVE_SECONDS (default 30),
#      EGRESS_READY_SECONDS (default 180).
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
# The ONE shared strace verdict (builds/_shared/egress-verdict.sh).
# shellcheck source=/dev/null
. "$here/egress-verdict.sh"
asset=${1:?usage: no-egress-test.sh ASSET_TARBALL}

die() { echo "no-egress: FAIL: $*" >&2; exit 1; }
[[ -f $asset ]] || die "$asset does not exist"
asset=$(cd "$(dirname "$asset")" && pwd)/$(basename "$asset")

if [[ $(basename "$asset") != klatch-web-* ]]; then
	case $(uname -s) in
	Linux) ;;
	Darwin)
		echo "no-egress: darwin: SKIPPED for $(basename "$asset") — the shared strace verdict has no macOS equivalent wired up yet (see files/no-egress-test.sh header and README.md #No-egress gate)"
		exit 0
		;;
	*) die "unsupported OS $(uname -s)" ;;
	esac
fi

if [[ $(basename "$asset") == klatch-web-* ]]; then
	# Static web bundle: nothing executes, so there is nothing to trace.
	# Just prove the archive is a sane, non-empty tree of plain files.
	work=$(mktemp -d "${TMPDIR:-/tmp}/klatch-web-check.XXXXXX")
	tar -C "$work" -xzf "$asset"
	files=$(find "$work" -type f | wc -l)
	((files > 0)) || die "$asset contains no files"
	if find "$work" -type f \( -name '*.sh' -o -perm -u+x \) | grep -q .; then
		die "$asset contains an executable or shell script — the web bundle must be static assets only"
	fi
	echo "no-egress: web bundle: $files static file(s), none executable OK"
	exit 0
fi

work=$(mktemp -d "${TMPDIR:-/tmp}/klatch-egress.XXXXXX")
chmod 755 "$work"
tar -C "$work" -xzf "$asset"
bin=$work/klatch
[[ -x $bin ]] || die "$asset did not extract an executable klatch binary"
mkdir -p "$work/store" "$work/web" "$work/home"

port=${EGRESS_PORT:-15000}
observe=${EGRESS_OBSERVE_SECONDS:-30}
ready_max=${EGRESS_READY_SECONDS:-180}
# GetNodeStatus (net/server/internal/api_getNodeStatus.go): unauthenticated
# GET that queries the (fresh, empty) sqlite store and answers plain JSON —
# proves the server, its sqlite store and its HTTP plumbing are all live.
health=http://127.0.0.1:$port/admin/status

command -v curl >/dev/null || die "curl is not on PATH (fail-closed)"

# The server's whole environment: no inherited proxies, tokens or endpoints.
envfile=$work/server.env
{
	echo "HOME=$work/home"
	echo "PATH=/usr/bin:/bin"
	grep -E '^[A-Z_]+=' "$here/no-tracking.env"
	grep -E '^[A-Z_]+=' "$here/otel.env"
} >"$envfile"

cat >"$work/run.sh" <<RUN
set -uo pipefail
exec '$bin' -p '$port' -s '$work/store' -w '$work/web' -t '$work/transform'
RUN
chmod +x "$work/run.sh"

probe() {
	# Loopback traffic over a handful of unauthenticated routes
	# (net/server/internal/routers.go); the answers do not matter, only
	# that klatch stays local while answering them.
	local base=http://127.0.0.1:$port
	curl -sS -o /dev/null "$base/admin/status" || true
	curl -sS -o /dev/null "$base/account/status" || true
	curl -sS -o /dev/null "$base/account/available" || true
	curl -sS -o /dev/null "$base/" || true
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

otlp_port=31415
log=$work/strace.log
inner=$work/inner.sh
ruid=$(id -u)
rgid=$(id -g)
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
