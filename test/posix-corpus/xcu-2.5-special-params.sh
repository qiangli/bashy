set -- a b c
echo "num=$# star=$* first=$1 third=$3"
for w in "$@"; do printf '<%s>' "$w"; done; echo
shift 2; echo "after=$# rest=$*"
true; echo "ok=$?"; false; echo "fail=$?"
[ -n "$0" ] && echo "argv0-set"
