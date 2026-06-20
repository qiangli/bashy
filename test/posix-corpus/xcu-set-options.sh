set -- a b c; echo "$# $*"
: > g1.txt; : > g2.txt
echo g*
set -f; echo g*; set +f
echo g*
( set -u; echo "[${UNSET_XYZ:-def}]" )
