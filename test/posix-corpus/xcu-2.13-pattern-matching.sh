for s in cat car cup zzz; do
  case $s in
    ca[tr]) echo "set:$s" ;;
    c?p) echo "qmark:$s" ;;
    c*) echo "star:$s" ;;
    *) echo "none:$s" ;;
  esac
done
case b in [!x]) echo neg ;; esac
case m in [a-z]) echo range ;; esac
