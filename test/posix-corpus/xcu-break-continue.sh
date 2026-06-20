for i in 1 2 3; do [ "$i" = 2 ] && continue; echo "c$i"; done
for i in 1 2 3 4; do [ "$i" = 3 ] && break; echo "b$i"; done
for i in 1 2; do for j in a b c; do [ "$j" = b ] && continue 2; echo "cc$i$j"; done; done
for i in 1 2; do for j in a b; do [ "$i$j" = 2a ] && break 2; echo "bb$i$j"; done; done
