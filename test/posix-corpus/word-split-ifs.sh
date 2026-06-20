IFS=:
s=a:b::c
set -- $s
echo "$# [$1][$2][$3][$4]"
IFS=' '
