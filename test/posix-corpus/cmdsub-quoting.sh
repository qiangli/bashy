x=$(echo "  spaced  out  ")
echo "[$x]"
echo [$x]
n=$(printf 'a\nb\nc'); echo "lines=$(printf '%s' "$n" | wc -l)"
