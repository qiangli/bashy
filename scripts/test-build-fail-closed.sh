#!/bin/sh
# Hermetic contract test for the compound Makefile build recipes. A fake Go
# compiler succeeds for cmd/bash but fails for cmd/bashy; the native launcher
# and installer leave markers if make incorrectly continues past that failure.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd -P)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/bashy-build-fail-closed.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

fakebin=$tmp/fakebin
bindir=$tmp/bin
mkdir -p "$fakebin" "$bindir"

export FAKE_GO_LOG=$tmp/go.log
export FAKE_CC_LOG=$tmp/cc.log
export FAKE_INSTALL_MARKER=$tmp/install-called

cat >"$fakebin/go" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >>"$FAKE_GO_LOG"

if [ "${1-}" = env ]; then
	case "${2-}" in
	GOOS|GOHOSTOS) printf '%s\n' linux; exit 0 ;;
	GOARCH|GOHOSTARCH) printf '%s\n' amd64; exit 0 ;;
	esac
fi

case " $* " in
	*' run ./tools/installbashy '*)
		: >"$FAKE_INSTALL_MARKER"
		exit 0
		;;
	*' ./cmd/bashy '*)
		echo 'fake go: intentional cmd/bashy compile failure' >&2
		exit 73
		;;
	*' ./cmd/bash '*)
		out=
		want_out=false
		for arg do
			if [ "$want_out" = true ]; then
				out=$arg
				break
			fi
			[ "$arg" = -o ] && want_out=true
		done
		[ -n "$out" ] || exit 74
		: >"$out"
		chmod +x "$out"
		exit 0
		;;
esac

echo "fake go: unexpected invocation: $*" >&2
exit 75
EOF

cat >"$fakebin/cc" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >>"$FAKE_CC_LOG"
out=
want_out=false
for arg do
	if [ "$want_out" = true ]; then
		out=$arg
		break
	fi
	[ "$arg" = -o ] && want_out=true
done
[ -n "$out" ] || exit 76
: >"$out"
chmod +x "$out"
EOF

real_bashy=${BASHY:-$(command -v bashy 2>/dev/null || echo "$root/bin/bashy")}

cat >"$fakebin/bashy" <<EOF
#!/bin/sh
if [ "\${1-}" = go ]; then
	shift
	exec "$fakebin/go" "\$@"
fi
export GO="$fakebin/go"
exec "$real_bashy" "\$@"
EOF

chmod +x "$fakebin/go" "$fakebin/cc" "$fakebin/bashy"

# Exclude Homebrew and other optional tool locations so build-meet-spa.sh takes
# its documented no-node/no-UI path and cannot install packages or use network.
test_path=$fakebin:/usr/bin:/bin
export GO="$fakebin/go"

# A direct build-bashy failure must not compile its native launcher.
if (cd "$root" && PATH=$test_path "$fakebin/bashy" dag build-bashy BIN_DIR="$bindir") >/dev/null 2>"$tmp/build.err"; then
	echo 'test-build-fail-closed: build-bashy unexpectedly succeeded' >&2
	exit 1
fi
[ ! -s "$FAKE_CC_LOG" ] || {
	echo 'test-build-fail-closed: launcher compiler ran after failed bashy build' >&2
	exit 1
}

# Even with stale outputs present, install must stop at the failed dependency.
printf '%s\n' stale-bashy >"$bindir/bashy"
printf '%s\n' stale-bashy-real >"$bindir/bashy.real"
stale_sum=$(cksum "$bindir/bashy" "$bindir/bashy.real")
: >"$FAKE_CC_LOG"
: >"$FAKE_GO_LOG"

if (cd "$root" && PATH=$test_path DHNT_BIN_DIR="$tmp/install" \
	"$fakebin/bashy" dag install BIN_DIR="$bindir") >/dev/null 2>"$tmp/install.err"; then
	echo 'test-build-fail-closed: install unexpectedly succeeded' >&2
	exit 1
fi

[ ! -e "$FAKE_INSTALL_MARKER" ] || {
	echo 'test-build-fail-closed: installer ran after failed bashy build' >&2
	exit 1
}
[ "$(cksum "$bindir/bashy" "$bindir/bashy.real")" = "$stale_sum" ] || {
	echo 'test-build-fail-closed: stale bashy outputs changed after failed build' >&2
	exit 1
}

# build-bash legitimately reaches its launcher; build-bashy must not reach a
# second launcher invocation after the intentional compile failure.
[ "$(wc -l <"$FAKE_CC_LOG" | tr -d ' ')" = 1 ] || {
	echo 'test-build-fail-closed: unexpected launcher compiler invocation count' >&2
	exit 1
}
grep -q -e "-o $bindir/bash " "$FAKE_CC_LOG" || {
	echo 'test-build-fail-closed: the sole launcher invocation was not cmd/bash' >&2
	exit 1
}
grep -q './cmd/bashy' "$FAKE_GO_LOG" || {
	echo 'test-build-fail-closed: install did not reach the intended failing build' >&2
	exit 1
}

echo 'test-build-fail-closed: PASS'
