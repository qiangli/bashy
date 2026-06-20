v=value; unset u; e=
echo "${v} ${v:-d} ${u:-d} ${e:-d} ${e-keep}"
echo "${u:=now}/$u"
echo "${v:+set}/${e:+set}/${u:+set}"
echo "len=${#v}"
p=a.b.c.txt
echo "${p%.*} ${p%%.*} ${p#*.} ${p##*.}"
