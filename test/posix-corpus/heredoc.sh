cat <<END
plain $HOME-less line
END
v=world
cat <<-'Q'
	no $v expansion (quoted)
Q
cat <<U
yes $v expansion
U
