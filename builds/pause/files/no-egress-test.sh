#!/usr/bin/env bash
# no-egress-test.sh BINARY
#
# Release gate, run BEFORE signing: run the built pause with the shipped
# no-tracking.env and otel.env environment, let it sit, then send SIGTERM, and
# FAIL on any outbound connection attempt at all (pause opens no socket; only
# bashy's local collector port is even admitted), or if pause does not keep
# pause.c's signal contract: alive for the whole window, then on SIGTERM
# "Shutting down, got signal: Terminated" on stderr and exit status 0.
#
# Linux only (the image is linux): runs in a fresh network namespace (only
# `lo`) under `strace -f -e trace=connect,sendto,sendmsg,sendmmsg`, so every
# attempt — including ones the namespace makes fail (ENETUNREACH) and DNS — is
# recorded and judged. Needs root via sudo (unshare --net), strace, ip.
#
# Env: EGRESS_OBSERVE_SECONDS (default 15).
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
# The ONE shared strace verdict (builds/_shared/egress-verdict.sh).
# shellcheck source=/dev/null
. "$here/egress-verdict.sh"
bin=${1:?usage: no-egress-test.sh BINARY}
bin=$(cd "$(dirname "$bin")" && pwd)/$(basename "$bin")
observe=${EGRESS_OBSERVE_SECONDS:-15}

die() { echo "no-egress: FAIL: $*" >&2; exit 1; }
[[ -x $bin ]] || die "$bin is not executable"
[[ $(uname -s) == Linux ]] || die "unsupported OS $(uname -s) (pause is built for linux)"

work=$(mktemp -d "${TMPDIR:-/tmp}/pause-egress.XXXXXX")
chmod 755 "$work"
envfile=$work/pause.env
{
	echo "PATH=/usr/bin:/bin"
	grep -E '^[A-Z_]+=' "$here/no-tracking.env"
	grep -E '^[A-Z_]+=' "$here/otel.env"
} >"$envfile"
otlp_port=31415

log=$work/strace.log inner=$work/inner.sh
cat >"$inner" <<INNER
set -uo pipefail
ip link set lo up
strace -f -qq -e trace=connect,sendto,sendmsg,sendmmsg -e signal=none -o '$log' \\
	env -i \$(cat '$envfile') '$bin' >'$work/pause.log' 2>&1 &
tracer=\$!
sleep $observe
pid=\$(pgrep -P \$tracer) || { echo "pause exited during the observation window" >&2; cat '$work/pause.log' >&2; exit 1; }
kill -TERM \$pid
wait \$tracer
rc=\$?
[[ \$rc == 0 ]] || { echo "pause exited \$rc on SIGTERM, pause.c exits 0" >&2; cat '$work/pause.log' >&2; exit 1; }
INNER
sudo unshare --net -- bash "$inner" || die "pause run failed (see above)"
cat "$work/pause.log"
grep -qx 'Shutting down, got signal: Terminated' "$work/pause.log" ||
	die "pause did not report SIGTERM as pause.c does"
touch "$log"

# The verdict itself is the ONE shared copy: bashy-build/egress-verdict.sh
# (builds/_shared/egress-verdict.sh). AF_UNIX is local IPC.
egress_verdict "$log" "$otlp_port" ||
	die "outbound connection attempt(s) other than loopback:$otlp_port"
