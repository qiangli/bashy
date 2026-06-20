trap 'echo EXIT-fired' EXIT
trap 'echo INT-handler' INT
echo before
(trap 'echo sub-exit' EXIT; echo in-subshell)
echo after
