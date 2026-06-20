true; echo $?
false; echo $?
false || echo "or-ran $?"
true && echo "and-ran"
(exit 7); echo "subshell=$?"
! true; echo "negate=$?"
false | true; echo "pipe-last=$?"
