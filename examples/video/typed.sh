# typed.sh — sourced by every recorded step. `show CMD` prints a prompt,
# types the command line at a human pace, then runs it for real; `say TEXT`
# prints a dim caption line; `pause SECS` holds the frame.
TYPE_DELAY=${TYPE_DELAY:-0.035}

# delay SECS waits in-shell: a read with a timeout on a pipe nobody writes.
# One external `sleep` per typed character costs a process spawn each, which
# is far slower than the delay itself.
exec 9<> <(:)
delay() { read -r -t "$1" -u 9 _ || true; }

show() {
	local line="$*" i
	printf '\e[1;32m$\e[0m '
	for ((i = 0; i < ${#line}; i++)); do
		printf '%s' "${line:i:1}"
		delay "$TYPE_DELAY"
	done
	printf '\n'
	delay 0.4
	eval "$line"
}

say() { printf '\e[2m# %s\e[0m\n' "$*"; delay 0.8; }

pause() { delay "${1:-1}"; }
