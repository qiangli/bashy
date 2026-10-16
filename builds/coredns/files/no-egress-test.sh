#!/usr/bin/env bash
# no-egress-test.sh BINARY
#
# Release gate, run BEFORE signing: start the built CoreDNS on 127.0.0.1 with
# the shipped no-tracking.env and otel.env environment and a Corefile that
# answers from itself only (whoami, health, log; no forward, no kubernetes),
# send it one real DNS query, and FAIL on any outbound connection attempt
# other than loopback to its own DNS and health ports, the probe's fixed
# source port (the reply), or 127.0.0.1:31415.
#
# CoreDNS has no update checker and no usage reporting: it dials only what
# its Corefile names. With a self-answering Corefile every packet it sends
# must be the reply to the probe.
#
# Linux only (the image is linux): runs in a fresh network namespace (only
# `lo`) under `strace -f -e trace=connect,sendto,sendmsg,sendmmsg`, so every
# attempt — including ones the namespace makes fail (ENETUNREACH) and DNS — is
# recorded and judged. Needs root via sudo (unshare --net), strace, ip, curl,
# python3.
#
# Env: EGRESS_PORT (default 10053), EGRESS_OBSERVE_SECONDS (default 30).
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
# The ONE shared strace verdict (builds/_shared/egress-verdict.sh).
# shellcheck source=/dev/null
. "$here/egress-verdict.sh"
bin=${1:?usage: no-egress-test.sh BINARY}
bin=$(cd "$(dirname "$bin")" && pwd)/$(basename "$bin")
port=${EGRESS_PORT:-10053}
observe=${EGRESS_OBSERVE_SECONDS:-30}

die() { echo "no-egress: FAIL: $*" >&2; exit 1; }
[[ -x $bin ]] || die "$bin is not executable"
[[ $(uname -s) == Linux ]] || die "unsupported OS $(uname -s) (coredns is built for linux)"

work=$(mktemp -d "${TMPDIR:-/tmp}/coredns-egress.XXXXXX")
chmod 755 "$work"
mkdir -p "$work/home"
envfile=$work/server.env
{
	echo "HOME=$work/home"
	echo "PATH=/usr/bin:/bin"
	grep -E '^[A-Z_]+=' "$here/no-tracking.env"
	grep -E '^[A-Z_]+=' "$here/otel.env"
} >"$envfile"

# Its DNS port, its health port, the probe's fixed source port (CoreDNS
# replies to it), and bashy's local OTLP collector (yoke
# external/internal/serviceenv LocalCollectorHostPort). Nothing else passes.
health_port=$((port + 1))
probe_port=$((port + 2))
otlp_port=31415
cat >"$work/Corefile" <<CF
.:$port {
	bind 127.0.0.1
	whoami
	health 127.0.0.1:$health_port
	log
}
CF

cat >"$work/probe.py" <<'PY'
import socket, struct, sys
port, src = int(sys.argv[1]), int(sys.argv[2])
q = struct.pack(">HHHHHH", 0x6273, 0x0100, 1, 0, 0, 0)
q += b"".join(bytes([len(l)]) + l for l in (b"bashy", b"probe")) + b"\0"
q += struct.pack(">HH", 1, 1)
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.bind(("127.0.0.1", src))
s.settimeout(5)
s.sendto(q, ("127.0.0.1", port))
r = s.recv(512)
ident, flags = struct.unpack(">HH", r[:4])
ok = ident == 0x6273 and flags & 0x8000 and flags & 0xF == 0
print("probe: DNS reply id=%#x rcode=%d" % (ident, flags & 0xF))
sys.exit(0 if ok else 1)
PY

log=$work/strace.log inner=$work/inner.sh
cat >"$inner" <<INNER
set -uo pipefail
ip link set lo up
strace -f -qq -e trace=connect,sendto,sendmsg,sendmmsg -e signal=none -o '$log' \\
	env -i \$(cat '$envfile') '$bin' -conf '$work/Corefile' >'$work/server.log' 2>&1 &
tracer=\$!
for _ in \$(seq 1 50); do
	curl -fsS 'http://127.0.0.1:$health_port/health' >/dev/null 2>&1 && break
	sleep 0.2
done
curl -fsS 'http://127.0.0.1:$health_port/health' || { echo "coredns never became healthy" >&2; tail -n 80 '$work/server.log' >&2; exit 1; }
echo
python3 '$work/probe.py' $port $probe_port || { echo "coredns did not answer the DNS probe" >&2; tail -n 80 '$work/server.log' >&2; exit 1; }
sleep $observe
if ! pgrep -P \$tracer >/dev/null; then
	echo "coredns exited during the observation window" >&2
	tail -n 80 '$work/server.log' >&2 || true
	exit 1
fi
pkill -TERM -P \$tracer || true
wait \$tracer || true
INNER
sudo unshare --net -- bash "$inner" || die "server run failed (see above)"
[[ -s $log ]] || die "no strace log recorded"
grep -q 'bashy.probe.' "$work/server.log" ||
	{ tail -n 40 "$work/server.log" >&2; die "coredns never logged the probe query (the gate observed nothing)"; }

# The verdict itself is the ONE shared copy: bashy-build/egress-verdict.sh
# (builds/_shared/egress-verdict.sh). AF_UNIX is local IPC.
egress_verdict "$log" "$port" "$health_port" "$probe_port" "$otlp_port" ||
	die "outbound connection attempt(s) other than loopback:{$port,$health_port,$probe_port,$otlp_port}"
