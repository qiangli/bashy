# POSIX parameter expansion forms
a=hello
echo "${a%l*}"      # remove shortest suffix -> hel
echo "${a%%l*}"     # longest -> he
echo "${a#he}"      # prefix -> llo
echo "${#a}"        # length -> 5
unset b; echo "${b:-def}/${b:=now}/${b}"
echo "${a:1:3}"     # substring (POSIX? bash-ism) -> ell
