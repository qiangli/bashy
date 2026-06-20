for s in abc a.c "a c" "" axc; do
  case $s in
    a?c) echo "qmark:$s" ;;
    "a c") echo "lit-space:$s" ;;
    "") echo "empty" ;;
    a*c) echo "star:$s" ;;
    *) echo "none:$s" ;;
  esac
done
