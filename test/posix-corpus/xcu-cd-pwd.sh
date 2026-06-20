mkdir -p d/e
cd d; echo "1:${PWD##*/}"
cd e; echo "2:${PWD##*/}"
cd ..; echo "3:${PWD##*/}"
cd - >/dev/null; echo "4:${PWD##*/}"
