#!/bin/sh
# Run as a bashy binary invoked through a symlink named sh on Linux.
# /usr/bin/time -f 'elapsed_sec=%e' env GOGC=off ./sh scripts/s407-carrier-fd-bench.sh

count_fds() { /bin/ls "/proc/$$/fd" | /usr/bin/wc -l; }
before=$(count_fds)
i=0
limit=${1:-2000}
while [ "$i" -lt "$limit" ]; do
    /usr/bin/awk 'BEGIN { x = 1 + 1 }' &
    wait "$!" || exit
    i=$((i + 1))
done
after=$(count_fds)
printf 'fd_before=%s fd_after=%s commands=%s\n' "$before" "$after" "$limit"
