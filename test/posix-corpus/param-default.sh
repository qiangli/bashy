unset u
echo "a:[${u:-d1}] still:[${u-d2}]"
e=
echo "b:[${e:-d3}] c:[${e-d4}]"
echo "d:[${u:+set}] e:[${e:+set}]"
v=val
echo "f:[${v:-x}] g:[${v:+y}]"
echo "len:${#v}"
