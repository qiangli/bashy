#!/usr/bin/env bash
# egress-verdict-test.sh — self-test for the shared strace verdict.
#
# builds/check.sh runs this once per invocation. egress-verdict.sh is the one
# copy seven forks' release gates depend on, and the AF_UNIX case in particular
# is a reviewed decision that a well-meaning edit could quietly reverse (doing
# so failed the Linux leg on tuwunel and on all three Victoria forks), so the
# classification is pinned by example here.
set -uo pipefail

here=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$here/egress-verdict.sh"

log=$(mktemp "${TMPDIR:-/tmp}/egress-verdict-test.XXXXXX")
trap 'rm -f "$log"' EXIT
pass=0 fail=0

t() { # NAME WANT_RC LINE ALLOWED_PORT...
	local name=$1 want=$2 line=$3 out rc
	shift 3
	printf '%s\n' "$line" >"$log"
	out=$(egress_verdict "$log" "$@" 2>&1)
	rc=$?
	if [[ $rc == "$want" ]]; then
		pass=$((pass + 1))
	else
		fail=$((fail + 1))
		echo "egress-verdict-test: FAIL: $name: rc=$rc want=$want :: $out" >&2
	fi
}

# Not egress: local IPC, loopback on an allowed port, and syscalls with no
# address to judge.
t 'AF_UNIX nscd' 0 \
	'1 connect(7, {sa_family=AF_UNIX, sun_path="/var/run/nscd/socket"}, 110) = -1 ENOENT' 18428
t 'AF_UNIX systemd-resolved' 0 \
	'1 connect(9, {sa_family=AF_UNIX, sun_path="/run/systemd/resolve/io.systemd.Resolve"}, 110) = -1 ECONNREFUSED' 18428
t 'loopback v4 on the only allowed port' 0 \
	'1 connect(3, {sa_family=AF_INET, sin_port=htons(18428), sin_addr=inet_addr("127.0.0.1")}, 16) = 0' 18428
t 'loopback v4 on the second allowed port' 0 \
	'1 connect(3, {sa_family=AF_INET, sin_port=htons(31415), sin_addr=inet_addr("127.0.0.1")}, 16) = 0' 18428 31415
t 'loopback v6 on an allowed port' 0 \
	'1 connect(3, {sa_family=AF_INET6, sin6_port=htons(18428), inet_pton(AF_INET6, "::1", &sin6_addr), sin6_flowinfo=0}, 28) = 0' 18428
t 'v4-mapped loopback on an allowed port' 0 \
	'1 connect(3, {sa_family=AF_INET6, sin6_port=htons(18428), inet_pton(AF_INET6, "::ffff:127.0.0.1", &sin6_addr), sin6_flowinfo=0}, 28) = 0' 18428
t 'AF_NETLINK has no address' 0 \
	'1 connect(4, {sa_family=AF_NETLINK, nl_pid=0, nl_groups=00000000}, 12) = 0' 18428
t 'AF_UNSPEC (UDP disconnect)' 0 \
	'1 connect(5, {sa_family=AF_UNSPEC, sa_data="\000\000"}, 16) = 0' 18428

# Egress: anything else, including attempts the namespace itself made fail.
t 'loopback on a port that is not allowed' 1 \
	'1 connect(3, {sa_family=AF_INET, sin_port=htons(31415), sin_addr=inet_addr("127.0.0.1")}, 16) = 0' 18428
t 'DNS to a non-loopback resolver' 1 \
	'1 sendto(6, "\0\1", 2, MSG_NOSIGNAL, {sa_family=AF_INET, sin_port=htons(53), sin_addr=inet_addr("8.8.8.8")}, 16) = -1 ENETUNREACH' 18428 31415
t 'v6 egress' 1 \
	'1 connect(3, {sa_family=AF_INET6, sin6_port=htons(443), inet_pton(AF_INET6, "2606:4700::1", &sin6_addr), sin6_flowinfo=0}, 28) = -1 ENETUNREACH' 18428 31415

if ((fail)); then
	echo "egress-verdict-test: $pass passed, $fail FAILED" >&2
	exit 1
fi
echo "egress-verdict-test: $pass cases, the shared strace verdict holds"
