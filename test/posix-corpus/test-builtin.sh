[ -z "" ] && echo empty
[ "abc" = "abc" ] && echo eq
[ 5 -gt 3 ] && echo gt
if [ -n "x" ] && [ 1 -eq 1 ]; then echo both; fi
