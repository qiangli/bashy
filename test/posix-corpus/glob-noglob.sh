# runs in a fresh empty cwd (provided by the harness)
: > a.txt; : > b.txt; : > c.log
set -- *.txt
echo "txt count=$#"
echo *.log
echo "nomatch:" *.nomatch
