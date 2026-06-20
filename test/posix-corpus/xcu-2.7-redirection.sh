echo one > f; echo two >> f; cat f
cat < f
exec 3> g; echo via3 >&3; exec 3>&-; cat g
cat <<END
heredoc $v body
END
cat <<-TAB
	stripped
	TAB
