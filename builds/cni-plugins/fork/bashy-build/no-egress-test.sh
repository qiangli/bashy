#!/usr/bin/env bash
# no-egress-test.sh OUTDIR
#
# Release gate, run BEFORE signing: invoke every built CNI plugin in OUTDIR
# (cni-<name>-linux-<arch>) the way a container runtime does, with the shipped
# no-tracking.env and otel.env environment, and FAIL on any outbound connection
# attempt other than loopback to 127.0.0.1:31415.
#
# A CNI plugin is a one-shot CLI driven by CNI_COMMAND + a JSON config on
# stdin. Every plugin runs its about string (no CNI_COMMAND) and VERSION;
# loopback and host-local also run a real ADD and DEL (loopback against the
# test's own network namespace, host-local against a file-backed IPAM store).
# bridge/portmap/bandwidth/firewall ADD need iptables/tc and a veth peer,
# which a bare runner namespace does not provide; their code paths make no
# network call of their own.
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

die() { echo "no-egress: FAIL: $*" >&2; exit 1; }
[[ $(uname -s) == Linux ]] || die "unsupported OS $(uname -s) (the CNI plugins are linux-only)"
bins=("$outdir"/cni-*-linux-*)
[[ -x ${bins[0]} ]] || die "no cni-*-linux-* executables in $outdir"

work=$(mktemp -d "${TMPDIR:-/tmp}/cni-egress.XXXXXX")
chmod 755 "$work"
envfile=$work/plugin.env
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
rc=0
run() { # BIN CNI_COMMAND CONFIG
	env -i \$(cat '$envfile') CNI_COMMAND="\$2" CNI_CONTAINERID=bashy-probe \\
		CNI_NETNS=/proc/\$\$/ns/net CNI_IFNAME=lo CNI_PATH='$outdir' \\
		"\$1" <<<"\$3" >>'$work/plugins.log' 2>&1
}
for b in '$outdir'/cni-*-linux-*; do
	env -i \$(cat '$envfile') "\$b" >>'$work/plugins.log' 2>&1 || { echo "\$b: about failed" >&2; rc=1; }
	run "\$b" VERSION '{"cniVersion":"1.0.0"}' || { echo "\$b: VERSION failed" >&2; rc=1; }
done
lo=\$(echo '$outdir'/cni-loopback-linux-*)
lo_conf='{"cniVersion":"1.0.0","name":"probe","type":"loopback"}'
run "\$lo" ADD "\$lo_conf" || { echo "loopback ADD failed" >&2; rc=1; }
run "\$lo" DEL "\$lo_conf" || { echo "loopback DEL failed" >&2; rc=1; }
hl=\$(echo '$outdir'/cni-host-local-linux-*)
hl_conf='{"cniVersion":"1.0.0","name":"probe","type":"host-local","ipam":{"type":"host-local","dataDir":"$work/ipam","ranges":[[{"subnet":"10.250.0.0/24"}]]}}'
run "\$hl" ADD "\$hl_conf" || { echo "host-local ADD failed" >&2; rc=1; }
run "\$hl" DEL "\$hl_conf" || { echo "host-local DEL failed" >&2; rc=1; }
exit \$rc
INNER
sudo unshare --net -- strace -f -qq -e trace=connect,sendto,sendmsg,sendmmsg -e signal=none \
	-o "$log" bash "$inner" || { cat "$work/plugins.log" >&2; die "plugin run failed (see above)"; }
[[ -f $log ]] || die "no strace log recorded"

# The verdict itself is the ONE shared copy: bashy-build/egress-verdict.sh
# (builds/_shared/egress-verdict.sh). AF_UNIX and AF_NETLINK are local IPC.
egress_verdict "$log" "$otlp_port" ||
	die "outbound connection attempt(s) other than loopback:{$otlp_port}"
echo "no-egress: ${#bins[@]} CNI plugins made no outbound connection attempt"
