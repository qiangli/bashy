set -- -a -b val -c
while getopts "abc:" opt; do
  case $opt in
    a) echo "flag a" ;;
    b) echo "flag b" ;;
    c) echo "c=$OPTARG" ;;
  esac
done
echo "shift=$((OPTIND-1))"
