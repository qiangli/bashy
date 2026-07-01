set -e --
setup() { :; }
cleanup() { :; }
__finished() { cleanup; echo $(($__count-1)) >&3 2>/dev/null; }
__count=0
trap : PIPE
if [ "${ZSH_VERSION:-}" ]; then
  trap "__finished; __finished() { :; }; exit 1" TERM
else
  trap "exit 1" TERM
fi
trap "__finished" EXIT
#!/bin/sh

setup() { i=1; }
cleanup() { :; }

setup
__ready=
trap __ready=1 HUP
kill -HUP "$MAIN_PID"
until [ "$__ready" ]; do __dummy=; done
while __count=$(($__count+1)); do
i=$((i+1))
done
