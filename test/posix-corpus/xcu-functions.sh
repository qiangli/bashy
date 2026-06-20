greet() { echo "hi $1"; }
greet world
f() { echo "n=$#"; for a in "$@"; do printf '<%s>' "$a"; done; echo; }
f x y z
ispos() { [ "$1" -gt 0 ]; }
ispos 5 && echo pos
ispos -1 || echo nonpos
