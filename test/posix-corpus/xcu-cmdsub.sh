echo "$(echo nested)"
x=$(printf 'a b'); echo "[$x]"
echo "outer $(echo "inner $(echo deep)")"
echo `echo backtick`
