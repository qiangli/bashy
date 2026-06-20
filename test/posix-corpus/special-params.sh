set -- one two three
echo "$#"
echo "$*"
shift 2
echo "$1 $#"
f() { echo "args=$#: $@"; }
f a b c
