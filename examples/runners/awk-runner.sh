#!/bin/sh
# awk-runner — a fence runner for a language bashy has no fence for: awk.
# Register it once, use it from any script:
#
#   bashy commands add awk-runner --set exec.0=$PWD/examples/runners/awk-runner.sh --set effects.0=exec
#   ~~~awk as aw !awk-runner ... ~~~      aw.run(args...)  aw.check()
#
# A runner is called as `runner <verb> <file> [args…]`: `methods` answers the
# verb table (one JSON object per line: name, optional signature, optional
# effects), every other verb serves the fence. stdout is the value.
case "$1" in
methods)
	printf '%s\n' '{"name":"run","effects":["exec"]}' '{"name":"check"}'
	;;
run)
	file=$2
	shift 2
	awk -f "$file" "$@"
	;;
check)
	awk -f "$2" </dev/null >/dev/null 2>&1 && echo "awk: ok" || { echo "awk: syntax error in $2" >&2; exit 1; }
	;;
*)
	echo "awk-runner: unknown verb $1" >&2
	exit 2
	;;
esac
