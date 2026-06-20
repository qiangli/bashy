printf 'one two three\n' | { read a b c; echo "[$a][$b][$c]"; }
printf 'a\\ b\n' | { read x; echo "noraw:[$x]"; }
printf 'a\\ b\n' | { read -r x; echo "raw:[$x]"; }
printf '  lead trail  \n' | { read v; echo "[$v]"; }
