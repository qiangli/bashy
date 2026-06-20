set -- a "b c" d
echo "count=$#"
for x in "$@"; do echo "[$x]"; done
echo "---"
for x in $*; do echo "<$x>"; done
