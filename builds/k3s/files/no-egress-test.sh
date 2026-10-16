#!/usr/bin/env bash
# no-egress-test.sh BINARY
#
# Release gate, run BEFORE signing: start the built k3s as a single-node
# control plane (`k3s server --cluster-init --disable-agent`, the shape
# outpost's server-entrypoint runs; this build has no sqlite, so the datastore
# is embedded etcd) bound to 127.0.0.1, with the shipped no-tracking.env
# and otel.env environment, exercise the Kubernetes API over loopback, let it
# idle, and FAIL on any outbound connection attempt other than loopback to its
# own ports or to 127.0.0.1:31415.
#
# --flannel-backend=none: a control plane without an agent runs no CNI, but
# k3s still resolves the host-local CNI binary on PATH unless flannel is off.
# k3s has no update checker and no usage reporting; a server without an agent
# pulls no image. This gate is what proves the bare control plane stays local.
#
# Linux only (k3s is a linux-only artifact): the server runs as root (k3s
# server refuses otherwise) in a fresh network namespace (only `lo`) under
# `strace -f -e trace=connect,sendto,sendmsg,sendmmsg`, so every attempt —
# including ones the namespace makes fail (ENETUNREACH) and DNS — is recorded
# and judged. Needs root via sudo (unshare --net), strace, ip.
#
# Env: EGRESS_PORT (default 16443), EGRESS_OBSERVE_SECONDS (default 30),
#      EGRESS_READY_SECONDS (default 180).
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
# The ONE shared strace verdict (builds/_shared/egress-verdict.sh).
# shellcheck source=/dev/null
. "$here/egress-verdict.sh"
bin=${1:?usage: no-egress-test.sh BINARY}
bin=$(cd "$(dirname "$bin")" && pwd)/$(basename "$bin")
port=${EGRESS_PORT:-16443}
observe=${EGRESS_OBSERVE_SECONDS:-30}
ready_max=${EGRESS_READY_SECONDS:-180}

die() { echo "no-egress: FAIL: $*" >&2; exit 1; }
[[ -x $bin ]] || die "$bin is not executable"
[[ $(uname -s) == Linux ]] || die "unsupported OS $(uname -s) (k3s is linux-only)"

work=$(mktemp -d "${TMPDIR:-/tmp}/k3s-egress.XXXXXX")
chmod 755 "$work"
mkdir -p "$work/data" "$work/home"

# The server's whole environment: no inherited proxies, tokens or endpoints.
envfile=$work/server.env
{
	echo "HOME=$work/home"
	echo "PATH=/usr/bin:/bin"
	grep -E '^[A-Z_]+=' "$here/no-tracking.env"
	grep -E '^[A-Z_]+=' "$here/otel.env"
} >"$envfile"

# Loopback ports the control plane talks to itself on: the supervisor
# (--https-listen-port), the internal apiserver (--apiserver-port), and the
# controller-manager / cloud-controller / scheduler secure ports k3s fixes at
# 10257 / 10258 / 10259, and embedded etcd's client / peer / metrics ports
# 2379 / 2380 / 2381. Plus bashy's local OTLP collector (yoke
# external/internal/serviceenv LocalCollectorHostPort). Nothing else passes.
api_port=$((port + 1))
otlp_port=31415
allowed=("$port" "$api_port" 10257 10258 10259 2379 2380 2381 "$otlp_port")
kubeconfig=$work/kubeconfig
# Exists only inside the namespace (RFC 1918, on a dummy link): the apiserver's
# endpoint reconciler refuses a loopback advertise address.
nsaddr=10.255.0.2
export EGRESS_LOCAL_ADDRS=$nsaddr

cat >"$work/run.sh" <<RUN
set -uo pipefail
exec '$bin' server \\
	--data-dir='$work/data' \\
	--cluster-init \\
	--disable-agent \\
	--disable-cloud-controller \\
	--disable-helm-controller \\
	--disable-network-policy \\
	--flannel-backend=none \\
	--disable=traefik,servicelb,metrics-server,coredns,local-storage \\
	--egress-selector-mode=disabled \\
	--advertise-address=$nsaddr \\
	--node-ip=$nsaddr \\
	--https-listen-port=$port \\
	--apiserver-port=$api_port \\
	--write-kubeconfig='$kubeconfig' \\
	--write-kubeconfig-mode=644
RUN
chmod +x "$work/run.sh"

# The admin kubeconfig supplies the credentials; the address is the namespace
# one. With no --bind-address the supervisor is reachable on the advertise
# address (the server's own tunnel reconnects to wss://$nsaddr:$port once it
# is up and drops 127.0.0.1), so the kubeconfig's loopback server URL is not
# the one to probe.
# k3s dispatches its embedded kubectl by argv0 basename (reexec), so call it
# through a symlink named kubectl; `$bin kubectl` would hand the extra word to
# kubectl's own root command ("unknown command kubectl").
ln -sf "$bin" "$work/kubectl"
kubectl() { "$work/kubectl" --kubeconfig="$kubeconfig" --server="https://$nsaddr:$port" --request-timeout=5s "$@"; }

probe() {
	# Loopback traffic over the API: discovery, a list, a write and a read.
	# The answers do not matter, only that the control plane stays local.
	kubectl get --raw /readyz >/dev/null 2>&1 || true
	kubectl api-resources >/dev/null 2>&1 || true
	kubectl get namespaces >/dev/null 2>&1 || true
	kubectl create configmap bashy-probe --from-literal=k=v >/dev/null 2>&1 || true
	kubectl get configmap bashy-probe -o name >/dev/null 2>&1 || true
}

wait_ready() { # PID
	local i
	for ((i = 0; i < ready_max; i++)); do
		kill -0 "$1" 2>/dev/null || return 1
		[[ -s $kubeconfig ]] && kubectl get --raw /readyz >/dev/null 2>"$work/ready.err" && return 0
		sleep 1
	done
	return 1
}

log=$work/strace.log inner=$work/inner.sh
# Everything below runs inside the new network namespace, as root.
cat >"$inner" <<INNER
set -uo pipefail
ip link set lo up
# k3s refuses to start without a default route (it derives the node IP from
# it). Give the namespace one that leads nowhere: a dummy link with an
# unanswerable gateway. Any dial through it is still
# recorded by strace and judged.
ip link add bashy0 type dummy
ip addr add $nsaddr/24 dev bashy0
ip link set bashy0 up
ip route add default via 10.255.0.1 dev bashy0
$(declare -f kubectl probe wait_ready)
bin='$bin' kubeconfig='$kubeconfig' ready_max=$ready_max nsaddr=$nsaddr port=$port work='$work'
strace -f -qq -e trace=connect,sendto,sendmsg,sendmmsg -e signal=none -o '$log' \\
	env -i \$(cat '$envfile') bash '$work/run.sh' >'$work/server.log' 2>&1 &
tracer=\$!
if ! wait_ready \$tracer; then
	echo "k3s server did not become ready on $nsaddr:$port; last probe error:" >&2
	cat '$work/ready.err' >&2 || true
	ss -ltn >&2 || true
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

# The verdict itself is the ONE shared copy: bashy-build/egress-verdict.sh
# (builds/_shared/egress-verdict.sh). AF_UNIX is local IPC.
egress_verdict "$log" "${allowed[@]}" ||
	die "outbound connection attempt(s) other than loopback:{${allowed[*]}}"
