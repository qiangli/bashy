# typed.sh — sourced by every recorded step. `show CMD` prints a prompt,
# types the command line at a human pace, then runs it for real; `say TEXT`
# prints a dim caption line; `pause SECS` holds the frame.
TYPE_DELAY=${TYPE_DELAY:-0.012}

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

# say holds the caption for its reading time: SAY_WPM words a minute (default
# 240, about 4 a second) plus a short beat, so a viewer can finish reading.
# Keep VIDEO_IDLE above the longest hold, or the recorder cuts it short.
say() {
	printf '\e[2m# %s\e[0m\n' "$*"
	local words=$# wpm=${SAY_WPM:-240}
	delay "$(awk -v w="$words" -v r="$wpm" 'BEGIN { printf "%.2f", 0.6 + w * 60 / r }')"
}

pause() { delay "${1:-1}"; }
