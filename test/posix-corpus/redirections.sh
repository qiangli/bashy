echo out1 > r.txt; echo out2 >> r.txt; cat r.txt
echo "to stderr then merged" 2>&1
{ echo grouped1; echo grouped2; } | wc -l
exec 3>&1; echo "via fd3" >&3; exec 3>&-
printf 'x\ny\nz\n' | { read a; read b; echo "a=$a b=$b"; }
