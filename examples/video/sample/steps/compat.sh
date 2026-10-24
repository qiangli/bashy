say an ordinary Bash script, unchanged
show "declare -A count=([apples]=3 [pears]=5); for k in \"\${!count[@]}\"; do echo \"\$k=\${count[\$k]}\"; done | sort"
show "[[ 'bashy' =~ ^ba(.*)$ ]] && echo \"matched: \${BASH_REMATCH[1]}\""
pause 1
