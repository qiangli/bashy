echo() { printf 'FUNC:%s\n' "$*"; }
echo hi
unset -f echo
echo plain
x=$(command -v test); [ -n "$x" ] && echo "test resolves"
