for w in apple banana cherry; do
  case $w in
    a*) echo "A:$w" ;;
    *na*) echo "N:$w" ;;
    *) echo "?:$w" ;;
  esac
done
