[ -n x ] && echo n
[ -z "" ] && echo z
[ a = a ] && echo eq
[ a != b ] && echo ne
[ 5 -eq 5 ] && echo numeq
[ 5 -gt 3 ] && [ 3 -lt 5 ] && echo cmp
[ -e /dev/null ] && [ -d / ] && echo files
[ ! -f /no-such-xyz ] && echo notfile
if [ 1 = 1 ] && [ 2 = 2 ]; then echo and; fi
