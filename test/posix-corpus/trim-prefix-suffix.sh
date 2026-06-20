p=/a/b/c.tar.gz
echo "${p##*/}"
echo "${p%%.*}"
echo "${p%.*}"
echo "${p#*/}"
f=file.txt
echo "${f%.txt}.bak"
