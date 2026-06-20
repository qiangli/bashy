: ; echo "colon=$?"
eval 'x=2; echo "eval=$x"'
v=v1; export v; echo "export=$v"
readonly r=ro; echo "ro=$r"
unset v; echo "unset=[${v:-gone}]"
f() { return 4; }; f; echo "return=$?"
set -- one two three; echo "set=$# $2"
