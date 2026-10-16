#!/usr/bin/env bash
# no-egress-test.sh BINARY
#
# Release gate, run BEFORE signing: start the built metrics-server bound to
# 127.0.0.1 with the shipped no-tracking.env and otel.env environment and a
# kubeconfig whose only cluster is a loopback apiserver address, let it run,
# and FAIL on any outbound connection attempt other than loopback to that
# apiserver port, its own port, or 127.0.0.1:31415.
#
# metrics-server has no update checker and no usage reporting. Everything it
# talks to comes from the apiserver it is pointed at (node list -> kubelet
# addresses), so with a loopback apiserver every dial it makes must be
# loopback. No apiserver answers here: metrics-server waits for its informer
# caches before serving, so the gate judges its dials over the whole window
# and requires the process to still be running at the end (it neither crashed
# nor fell back to a non-loopback destination).
#
# Linux only (the image is linux): runs in a fresh network namespace (only
# `lo`) under `strace -f -e trace=connect,sendto,sendmsg,sendmmsg`, so every
# attempt — including ones the namespace makes fail (ENETUNREACH) and DNS — is
# recorded and judged. Needs root via sudo (unshare --net), strace, ip.
#
# Env: EGRESS_PORT (default 16443), EGRESS_OBSERVE_SECONDS (default 30).
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
# The ONE shared strace verdict (builds/_shared/egress-verdict.sh).
# shellcheck source=/dev/null
. "$here/egress-verdict.sh"
bin=${1:?usage: no-egress-test.sh BINARY}
bin=$(cd "$(dirname "$bin")" && pwd)/$(basename "$bin")
port=${EGRESS_PORT:-16443}
observe=${EGRESS_OBSERVE_SECONDS:-30}

die() { echo "no-egress: FAIL: $*" >&2; exit 1; }
[[ -x $bin ]] || die "$bin is not executable"
[[ $(uname -s) == Linux ]] || die "unsupported OS $(uname -s) (metrics-server is built for linux)"

work=$(mktemp -d "${TMPDIR:-/tmp}/metrics-server-egress.XXXXXX")
chmod 755 "$work"
mkdir -p "$work/home" "$work/certs"
envfile=$work/server.env
{
	echo "HOME=$work/home"
	echo "PATH=/usr/bin:/bin"
	grep -E '^[A-Z_]+=' "$here/no-tracking.env"
	grep -E '^[A-Z_]+=' "$here/otel.env"
} >"$envfile"

# Its own secure port, the loopback apiserver it is pointed at, and bashy's
# local OTLP collector (yoke external/internal/serviceenv
# LocalCollectorHostPort). Nothing else passes.
api_port=$((port + 1))
otlp_port=31415
kubeconfig=$work/kubeconfig
cat >"$kubeconfig" <<KC
apiVersion: v1
kind: Config
clusters:
- name: probe
  cluster: {server: "https://127.0.0.1:$api_port", insecure-skip-tls-verify: true}
contexts:
- name: probe
  context: {cluster: probe, user: probe}
current-context: probe
users:
- name: probe
  user: {token: bashy-no-egress-probe}
KC

cat >"$work/run.sh" <<RUN
set -uo pipefail
exec '$bin' \\
	--kubeconfig='$kubeconfig' \\
	--authentication-kubeconfig='$kubeconfig' \\
	--authorization-kubeconfig='$kubeconfig' \\
	--authentication-skip-lookup \\
	--bind-address=127.0.0.1 \\
	--secure-port=$port \\
	--cert-dir='$work/certs' \\
	--kubelet-insecure-tls
RUN
chmod +x "$work/run.sh"

log=$work/strace.log inner=$work/inner.sh
cat >"$inner" <<INNER
set -uo pipefail
ip link set lo up
strace -f -qq -e trace=connect,sendto,sendmsg,sendmmsg -e signal=none -o '$log' \\
	env -i \$(cat '$envfile') bash '$work/run.sh' >'$work/server.log' 2>&1 &
tracer=\$!
sleep $observe
if ! pgrep -P \$tracer >/dev/null; then
	echo "metrics-server exited during the observation window" >&2
	tail -n 80 '$work/server.log' >&2 || true
	exit 1
fi
pkill -TERM -P \$tracer || true
wait \$tracer || true
INNER
sudo unshare --net -- bash "$inner" || die "server run failed (see above)"
[[ -s $log ]] || die "no strace log recorded"
grep -q "127.0.0.1:$api_port" "$work/server.log" ||
	{ tail -n 40 "$work/server.log" >&2; die "metrics-server never dialled its apiserver (the gate observed nothing)"; }

# The verdict itself is the ONE shared copy: bashy-build/egress-verdict.sh
# (builds/_shared/egress-verdict.sh). AF_UNIX is local IPC.
egress_verdict "$log" "$port" "$api_port" "$otlp_port" ||
	die "outbound connection attempt(s) other than loopback:{$port,$api_port,$otlp_port}"
