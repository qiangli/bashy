#!/usr/bin/env bash
# no-egress-test.sh RUNC
#
# Release gate, run BEFORE signing: drive the built runc the way containerd's
# shim does, with the shipped no-tracking.env and otel.env environment, and
# FAIL on any outbound connection attempt other than loopback to
# 127.0.0.1:31415.
#
# runc is a one-shot CLI. The gate runs --version, features, spec and list,
# then a real container: `runc run` of a bundle whose rootfs holds only the
# static runc binary itself, as the container process (`/runc --version`).
# That path goes through nsenter (the cgo part this build links against
# musl), namespace and cgroup setup, pivot_root and exec, so a broken static
# link fails here rather than on a host.
#
# Linux only: runs as root in a fresh network namespace (only `lo`) under
# `strace -f -e trace=connect,sendto,sendmsg,sendmmsg`, so every attempt —
# including ones the namespace makes fail (ENETUNREACH) and DNS — is recorded
# and judged. Needs root via sudo (unshare --net), strace, ip, jq.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
# The ONE shared strace verdict (builds/_shared/egress-verdict.sh).
# shellcheck source=/dev/null
. "$here/egress-verdict.sh"
bin=${1:?usage: no-egress-test.sh RUNC}
bin=$(cd "$(dirname "$bin")" && pwd)/$(basename "$bin")

die() { echo "no-egress: FAIL: $*" >&2; exit 1; }
[[ $(uname -s) == Linux ]] || die "unsupported OS $(uname -s) (runc is built for linux)"
[[ -x $bin ]] || die "$bin is not executable"

work=$(mktemp -d "${TMPDIR:-/tmp}/runc-egress.XXXXXX")
chmod 755 "$work"
envfile=$work/runc.env
{
	echo "PATH=/usr/bin:/bin"
	grep -E '^[A-Z_]+=' "$here/no-tracking.env"
	grep -E '^[A-Z_]+=' "$here/otel.env"
} >"$envfile"
otlp_port=31415

# The bundle: rootfs = the static runc alone, no terminal, its own --version
# as the container process.
bundle=$work/bundle
mkdir -p "$bundle/rootfs"
cp "$bin" "$bundle/rootfs/runc"
(cd "$bundle" && "$bin" spec)
jq '.process.terminal = false | .process.args = ["/runc", "--version"]' \
	"$bundle/config.json" >"$bundle/config.json.new"
mv "$bundle/config.json.new" "$bundle/config.json"

log=$work/strace.log inner=$work/inner.sh
cat >"$inner" <<INNER
set -uo pipefail
ip link set lo up
rc=0
r() { env -i \$(cat '$envfile') '$bin' --root '$work/state' "\$@"; }
r --version >>'$work/runc.log' 2>&1 || { echo "--version failed" >&2; rc=1; }
r features >>'$work/runc.log' 2>&1 || { echo "features failed" >&2; rc=1; }
r list >>'$work/runc.log' 2>&1 || { echo "list failed" >&2; rc=1; }
(cd '$bundle' && r run bashy-probe) >'$work/container.log' 2>&1 ||
	{ echo "run failed" >&2; cat '$work/container.log' >&2; rc=1; }
exit \$rc
INNER
sudo unshare --net -- strace -f -qq -e trace=connect,sendto,sendmsg,sendmmsg -e signal=none \
	-o "$log" bash "$inner" || { cat "$work/runc.log" >&2; die "runc run failed (see above)"; }
[[ -f $log ]] || die "no strace log recorded"
# The container really ran: its process is runc itself, reporting the pin.
grep -q '^runc version ' "$work/container.log" ||
	{ cat "$work/container.log" >&2; die "the container process produced no version"; }

# The verdict itself is the ONE shared copy: bashy-build/egress-verdict.sh
# (builds/_shared/egress-verdict.sh). AF_UNIX and AF_NETLINK are local IPC.
egress_verdict "$log" "$otlp_port" ||
	die "outbound connection attempt(s) other than loopback:{$otlp_port}"
echo "no-egress: runc (--version, features, list, a container run) made no outbound connection attempt"
