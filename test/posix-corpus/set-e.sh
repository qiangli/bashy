set -e
echo before
( false; echo "not reached in subshell" ) || echo "subshell failed, caught"
echo after-caught
true
echo done
