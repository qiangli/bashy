#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
. "$here/egress-verdict.sh"
bin=$(cd "$(dirname "${1:?usage: no-egress-test.sh BINARY}")" && pwd)/$(basename "$1")
port=${EGRESS_PORT:-14466}
work=$(mktemp -d "${TMPDIR:-/tmp}/headlamp-egress.XXXXXX")
trap 'rm -rf "$work"' EXIT
envfile=$work/env
{ echo "HOME=$work"; echo 'PATH=/usr/bin:/bin'; grep -E '^[A-Z_]+=' "$here/no-tracking.env"; grep -E '^[A-Z_]+=' "$here/otel.env"; } > "$envfile"
cat > "$work/run.sh" <<EOF
exec '$bin' -no-browser -listen-addr=127.0.0.1 -port=$port -tracing-enabled=false -metrics-enabled=false
EOF
chmod +x "$work/run.sh"
case $(uname -s) in
Linux)
  log=$work/strace.log
  uid=$(id -u); gid=$(id -g)
  sudo unshare --net -- bash -c "ip link set lo up; strace -f -qq -e trace=connect,sendto,sendmsg,sendmmsg -e signal=none -o '$log' setpriv --reuid=$uid --regid=$gid --init-groups -- env -i \$(cat '$envfile') bash '$work/run.sh' >'$work/server.log' 2>&1 & p=\$!; for i in \$(seq 1 120); do curl -fsS http://127.0.0.1:$port/ >/dev/null && break; kill -0 \$p || exit 1; sleep 1; done; curl -fsS http://127.0.0.1:$port/ >/dev/null; sleep 15; kill -TERM \$p; wait \$p || true"
  egress_verdict "$log" "$port" 31415
  ;;
Darwin)
  profile=$work/no-egress.sb
  printf '%s\n' '(version 1)' '(allow default)' '(deny network-outbound (remote ip "*:*"))' "(allow network-outbound (remote ip \"localhost:$port\"))" > "$profile"
  # shellcheck disable=SC2046
  env -i $(cat "$envfile") sandbox-exec -f "$profile" bash "$work/run.sh" > "$work/server.log" 2>&1 & p=$!
  for _ in {1..120}; do curl -fsS "http://127.0.0.1:$port/" >/dev/null && break; kill -0 "$p" || exit 1; sleep 1; done
  curl -fsS "http://127.0.0.1:$port/" >/dev/null; sleep 5; kill -TERM "$p"; wait "$p" || true
  ;;
*) echo 'unsupported OS' >&2; exit 1;;
esac
