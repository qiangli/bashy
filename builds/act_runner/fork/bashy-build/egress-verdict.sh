#!/usr/bin/env bash
# egress-verdict.sh — the ONE strace verdict shared by every tool's
# bashy-build/no-egress-test.sh (builds/README.md convention 2).
#
# Sourced, not executed: it defines egress_verdict, which reads an strace log
# written with `-e trace=connect,sendto,sendmsg,sendmmsg` and decides whether
# the traced process tried to talk to anything but the destinations the caller
# allows.
#
#   . "$here/egress-verdict.sh"
#   egress_verdict "$log" "$port" [MORE_PORTS...] || die "..."
#
# The verdict (operator decision from the tuwunel fork CI, bashy fb22eaf9):
#
#   AF_INET / AF_INET6   judged. Only a loopback address (127.0.0.1, ::1,
#                        ::ffff:127.0.0.1) on one of the allowed ports passes;
#                        anything else is egress, including attempts the
#                        network namespace itself made fail (ENETUNREACH) and
#                        DNS packets.
#   AF_UNIX              local IPC, never egress. A connect to the glibc nscd
#                        or systemd-resolved socket is a filesystem socket on
#                        this host, not a packet leaving it; counting it as
#                        egress failed the gate on three Victoria forks
#                        (qiangli/victoria-{metrics,logs,traces} CI runs
#                        38311645575 / 38311666036 / 38311680149) for a
#                        resolver handshake that cannot reach the network.
#                        A lookup that DOES leave the host still shows up as
#                        the AF_INET/AF_INET6 sendto/sendmsg to port 53 that
#                        follows, which this verdict catches.
#   everything else      no address to judge: AF_UNSPEC (UDP disconnect),
#                        AF_NETLINK (route/interface queries), truncated lines.
#
# On failure every offending line is printed and the function returns 1; the
# caller renders the fatal message (and, for a server harness, the server log).
# On success it prints the one-line summary.

# egress_verdict LOG ALLOWED_PORT...
egress_verdict() {
	local log=$1
	shift
	local allowed=("$@") desc
	if (($# == 1)); then
		desc="loopback:$1"
	else
		desc="loopback:{$(
			IFS=,
			echo "$*"
		)}"
	fi

	local bad=() line fam host p ok allow
	while IFS= read -r line; do
		case $line in
		*sa_family=AF_INET6*)
			fam=inet6
			host=$(sed -n 's/.*inet_pton(AF_INET6, "\([^"]*\)".*/\1/p' <<<"$line")
			p=$(sed -n 's/.*sin6_port=htons(\([0-9]*\)).*/\1/p' <<<"$line")
			;;
		*sa_family=AF_INET,*)
			fam=inet
			host=$(sed -n 's/.*sin_addr=inet_addr("\([^"]*\)").*/\1/p' <<<"$line")
			p=$(sed -n 's/.*sin_port=htons(\([0-9]*\)).*/\1/p' <<<"$line")
			;;
		*sa_family=AF_UNIX*) continue ;; # local IPC, not egress (see above)
		*) continue ;;                   # AF_UNSPEC, AF_NETLINK, no address
		esac
		ok=0
		case $host in
		127.0.0.1 | ::1 | ::ffff:127.0.0.1)
			for allow in "${allowed[@]}"; do
				[[ $p == "$allow" ]] && ok=1 && break
			done
			;;
		esac
		((ok)) || bad+=("$fam $host:$p :: $line")
	done <"$log"

	if ((${#bad[@]})); then
		printf 'no-egress: outbound attempt: %s\n' "${bad[@]}" >&2
		echo "no-egress: ${#bad[@]} outbound connection attempt(s) other than $desc" >&2
		return 1
	fi
	echo "no-egress: linux: $(grep -c . "$log") traced socket syscalls, none leave $desc OK"
}
