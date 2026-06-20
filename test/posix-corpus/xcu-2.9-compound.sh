if true; then echo if; elif false; then echo elif; else echo else; fi
if false; then echo a; else echo else2; fi
i=0; while [ $i -lt 3 ]; do echo "w$i"; i=$((i+1)); done
i=0; until [ $i -ge 2 ]; do echo "u$i"; i=$((i+1)); done
for x in p q; do echo "f$x"; done
{ echo group1; echo group2; }
(echo subshell)
