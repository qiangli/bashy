s="a b  c"; set -- $s; echo "ws=$#:[$1][$2][$3]"
IFS=:; p="x::z"; set -- $p; echo "colon=$#:[$1][$2][$3]"
IFS=' '; v="  trim me  "; set -- $v; echo "trim=$#:[$1][$2]"
unset IFS; w="d e"; set -- $w; echo "unset-ifs=$#:[$1][$2]"
