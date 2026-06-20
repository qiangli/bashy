true && echo and1
false || echo or1
true && false || echo chain
! false && echo neg
echo a; echo b
echo x | cat | cat
false | true; echo "pipe=$?"
(exit 5) & wait $!; echo "bgwait=$?"
