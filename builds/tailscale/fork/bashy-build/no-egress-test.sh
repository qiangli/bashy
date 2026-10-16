#!/usr/bin/env bash
# no-egress-test.sh OUTDIR
#
# Release gate, run BEFORE signing: start the built tailscaled from OUTDIR
# (tailscaled-linux-<arch>) with userspace networking, in-memory state and
# the shipped no-tracking.env and otel.env environment, drive it with the
# built CLI (tailscale-linux-<arch>) over its LocalAPI socket, observe it, and
# FAIL on any outbound connection attempt other than loopback to its own
# WireGuard port or 127.0.0.1:31415.
#
# Unconfigured, tailscaled must sit in NeedsLogin and dial nothing: no control
# server, no DERP, no log upload (compiled out), no update check (compiled
# out), no cloud-metadata probe (compiled out — GitHub's Azure runners would
# otherwise trigger it). Contacting a control server happens only after
# `tailscale up --login-server=...`, which the image configures.
#
# Linux only: runs as root in a fresh network namespace (only `lo`) under
# `strace -f -e trace=connect,sendto,sendmsg,sendmmsg`, so every attempt —
# including ones the namespace makes fail (ENETUNREACH) and DNS — is recorded
# and judged. Needs root via sudo (unshare --net), strace, ip.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
# The ONE shared strace verdict (builds/_shared/egress-verdict.sh).
# shellcheck source=/dev/null
. "$here/egress-verdict.sh"
outdir=$(cd "${1:?usage: no-egress-test.sh OUTDIR}" && pwd)
observe=${OBSERVE_SECONDS:-20}

die() { echo "no-egress: FAIL: $*" >&2; exit 1; }
[[ $(uname -s) == Linux ]] || die "unsupported OS $(uname -s) (tailscale is built for linux)"
daemon=$(echo "$outdir"/tailscaled-linux-*)
cli=$(echo "$outdir"/tailscale-linux-*)
[[ -x $daemon && -x $cli ]] || die "no tailscaled-linux-*/tailscale-linux-* executables in $outdir"

work=$(mktemp -d "${TMPDIR:-/tmp}/tailscale-egress.XXXXXX")
chmod 755 "$work"
mkdir -p "$work/home" "$work/state"
envfile=$work/daemon.env
{
	echo "HOME=$work/home"
	echo "PATH=/usr/bin:/bin"
	grep -E '^[A-Z_]+=' "$here/no-tracking.env"
	grep -E '^[A-Z_]+=' "$here/otel.env"
} >"$envfile"
# Its own WireGuard UDP port and bashy's local OTLP collector (yoke
# external/internal/serviceenv LocalCollectorHostPort). Nothing else passes.
wg_port=41641
otlp_port=31415
sock=$work/tailscaled.sock

cat >"$work/run.sh" <<RUN
set -uo pipefail
env -i \$(cat '$envfile') '$daemon' --tun=userspace-networking --state=mem: \\
	--statedir='$work/state' --socket='$sock' --port=$wg_port >'$work/daemon.log' 2>&1 &
d=\$!
for _ in \$(seq 50); do [[ -S '$sock' ]] && break; sleep 0.2; done
env -i \$(cat '$envfile') '$cli' --socket='$sock' version >'$work/cli.log' 2>&1 ||
	echo "cli version failed" >>'$work/cli.log'
env -i \$(cat '$envfile') '$cli' --socket='$sock' status --json >>'$work/cli.log' 2>&1 || true
sleep $observe
kill -0 \$d || { echo "tailscaled exited during the observation window" >>'$work/cli.log'; exit 1; }
kill -TERM \$d
wait \$d || true
RUN

log=$work/strace.log inner=$work/inner.sh
cat >"$inner" <<INNER
set -uo pipefail
ip link set lo up
strace -f -qq -e trace=connect,sendto,sendmsg,sendmmsg -e signal=none -o '$log' bash '$work/run.sh'
INNER
sudo unshare --net -- bash "$inner" ||
	{ cat "$work/cli.log" >&2; tail -n 80 "$work/daemon.log" >&2; die "tailscaled run failed (see above)"; }
[[ -s $log ]] || die "no strace log recorded"
# The gate observed a live, unconfigured daemon through its LocalAPI.
grep -q '"BackendState": "NeedsLogin"' "$work/cli.log" ||
	{ cat "$work/cli.log" >&2; tail -n 80 "$work/daemon.log" >&2; die "tailscaled never answered status in NeedsLogin"; }

# The verdict itself is the ONE shared copy: bashy-build/egress-verdict.sh
# (builds/_shared/egress-verdict.sh). AF_UNIX (the LocalAPI) and AF_NETLINK
# are local IPC.
egress_verdict "$log" "$wg_port" "$otlp_port" ||
	{ tail -n 80 "$work/daemon.log" >&2; die "outbound connection attempt(s) other than loopback:{$wg_port,$otlp_port}"; }
echo "no-egress: tailscaled + tailscale (observed ${observe}s, NeedsLogin) made no outbound connection attempt"
