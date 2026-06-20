: > a.txt; : > b.txt; : > .hidden
echo *.txt
echo *
echo .h*
echo "*.txt"
echo nomatch_*
case a.txt in *.txt) echo case-glob ;; esac
