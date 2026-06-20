printf 'a b c\n' | { read x y z; echo "[$x][$y][$z]"; }
printf 'a b c d\n' | { read x y; echo "[$x][$y]"; }
printf 'hello\n' | { read v; echo "len=${#v}"; }
printf 'a\tb\n' | { read p q; echo "[$p][$q]"; }
