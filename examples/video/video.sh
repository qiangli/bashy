#!/usr/bin/env bash
# video.sh — turn a storyboard of title cards and recorded terminal steps
# into one MP4. Composes three host tools: asciinema (record), agg (render
# a cast to GIF) and ffmpeg (encode, stack, concatenate, add narration).
#
# usage: video.sh doctor | cameras | build PROJECT_DIR | clean PROJECT_DIR
#
# PROJECT_DIR holds:
#   storyboard.txt   one segment per line (# comments allowed):
#                      card NAME [SECONDS]   cards/NAME.txt, first line bold
#                      step NAME             steps/NAME.sh, recorded headless
#                      camera NAME [SECONDS] [FIT]
#                                            camera/NAME.mov|.mp4, or record the
#                                            webcam + mic now (macOS); FIT speeds
#                                            a longer take up to FIT seconds
#                      pair LEFT RIGHT       two steps side by side; RIGHT
#                                            waits on its first frame until
#                                            LEFT has finished
#   cards/*.txt, steps/*.sh
#   narration.m4a    optional voice track, muxed over the whole video
# Output: PROJECT_DIR/build/demo.mp4 (intermediates in build/).
set -euo pipefail

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
COLS=${VIDEO_COLS:-100}
ROWS=${VIDEO_ROWS:-24}
# Pairs use a narrower terminal so text stays readable at half width.
PAIR_COLS=${VIDEO_PAIR_COLS:-64}
PAIR_ROWS=${VIDEO_PAIR_ROWS:-30}
# Steps run under bashy (dogfood); VIDEO_SHELL=bash to compare.
SHELL_CMD=${VIDEO_SHELL:-bashy}
FONT_SIZE=${VIDEO_FONT_SIZE:-28}
THEME=${VIDEO_THEME:-monokai}
IDLE=${VIDEO_IDLE:-1.5}
W=${VIDEO_WIDTH:-1920}
H=${VIDEO_HEIGHT:-1080}
FPS=${VIDEO_FPS:-30}
# One encoding for every segment so concatenation can copy streams:
# H.264 video plus a 48 kHz stereo AAC track (silence when a segment has none).
VENC=(-c:v libx264 -preset medium -crf 20 -pix_fmt yuv420p -r "$FPS")
AENC=(-c:a aac -b:a 160k -ar 48000 -ac 2)
# Intermediate recordings are video only.
ENC=("${VENC[@]}" -an)
CAMERA=${VIDEO_CAMERA:-0:0} # avfoundation "video:audio" device indexes (macOS)

# Recordings show only what the step prints: no startup notices or hints.
export BASHY_TELEMETRY_QUIET=1 BASHY_HINTS=off

die() { echo "video: $*" >&2; exit 1; }

doctor() {
	local ok=0 t
	for t in asciinema agg ffmpeg ffprobe "${VIDEO_SHELL:-bashy}"; do
		if command -v "$t" >/dev/null; then
			echo "ok   $t: $(command -v "$t")"
		else
			echo "MISSING $t"; ok=1
		fi
	done
	return $ok
}

# fit SRC DST: scale into WxH, pad to exactly WxH, normalize fps and codecs;
# keep SRC's audio or add a silent track.
fit() {
	local vf="scale=${W}:${H}:force_original_aspect_ratio=decrease:flags=lanczos,pad=${W}:${H}:(ow-iw)/2:(oh-ih)/2:color=black,fps=${FPS}"
	if [ -n "$(ffprobe -v error -select_streams a -show_entries stream=index -of csv=p=0 "$1")" ]; then
		ffmpeg -nostdin -hide_banner -loglevel error -y -i "$1" -vf "$vf" "${VENC[@]}" "${AENC[@]}" "$2"
	else
		ffmpeg -nostdin -hide_banner -loglevel error -y -i "$1" -f lavfi -i anullsrc=r=48000:cl=stereo \
			-map 0:v -map 1:a -shortest -vf "$vf" "${VENC[@]}" "${AENC[@]}" "$2"
	fi
}

# camera NAME [SECONDS]: use camera/NAME.mov|.mp4 if it exists (e.g. recorded
# with QuickTime); otherwise record webcam + microphone now for SECONDS
# (default 75) after a 3-second count-in, and keep the take as camera/NAME.mov.
camera() {
	local name=$1 secs=${2:-75} src=""
	for src in "$P/camera/$name.mov" "$P/camera/$name.mp4" ""; do [ -f "$src" ] && break; done
	if [ -z "$src" ]; then
		[ "$(uname)" = Darwin ] || die "live camera capture is wired for macOS (avfoundation); put the take at camera/$name.mov"
		mkdir -p "$P/camera"; src="$P/camera/$name.mov"
		echo "video: recording camera $CAMERA for ${secs}s into $src (q stops early)"
		for i in 3 2 1; do echo "  $i"; sleep 1; done
		ffmpeg -hide_banner -loglevel error -y -f avfoundation -framerate 30 -video_size 1280x720 \
			-i "$CAMERA" -t "$secs" -c:v libx264 -preset veryfast -crf 18 -pix_fmt yuv420p -c:a aac -b:a 192k "$src"
	fi
	local target=${3:-0} d r
	d=$(dur "$src")
	if [ "${target%.*}" -gt 0 ] 2>/dev/null && awk -v d="$d" -v t="$target" 'BEGIN{exit !(d>t)}'; then
		# Speed picture and voice up together; atempo keeps the voice's pitch.
		r=$(awk -v d="$d" -v t="$target" 'BEGIN{r=d/t; if(r>2)r=2; printf "%.4f", r}')
		echo "video: camera $name ${d%.*}s -> ${target}s (x$r)" >&2
		awk -v r="$r" 'BEGIN{exit !(r>1.35)}' && echo "video: warning: x$r is above 1.35; speech may sound rushed" >&2
		ffmpeg -nostdin -hide_banner -loglevel error -y -i "$src" -filter_complex \
			"[0:v]setpts=PTS/$r[v];[0:a]atempo=$r[a]" -map "[v]" -map "[a]" "${VENC[@]}" "${AENC[@]}" "$B/camera-$name-fit.mp4"
		src="$B/camera-$name-fit.mp4"
	fi
	fit "$src" "$B/seg-$SEQ.mp4"
}

# card NAME SECONDS: a one-frame cast with the card's text, centred.
card() {
	local name=$1 secs=${2:-4} txt="$P/cards/$1.txt" cast="$B/card-$1.cast"
	[ -f "$txt" ] || die "missing $txt"
	local cols=60 rows=14 lines=() i=0 l pad top out=""
	mapfile -t lines <"$txt"
	top=$(((rows - ${#lines[@]}) / 2))
	for ((i = 0; i < top; i++)); do out+=$'\r\n'; done
	for i in "${!lines[@]}"; do
		l=${lines[$i]}
		pad=$(((cols - ${#l}) / 2)); ((pad < 0)) && pad=0
		out+=$(printf '%*s' "$pad" '')
		if ((i == 0)); then out+=$'\e[1m'"$l"$'\e[0m'; else out+="$l"; fi
		out+=$'\r\n'
	done
	out+=$'\e[?25l' # hide the cursor
	{
		printf '{"version": 2, "width": %d, "height": %d}\n' "$cols" "$rows"
		printf '[0.0, "o", %s]\n' "$(printf '%s' "$out" | json_string)"
	} >"$cast"
	agg --font-size 48 --theme "$THEME" --last-frame-duration "$secs" --no-loop "$cast" "$B/card-$name.gif" >/dev/null
	fit "$B/card-$name.gif" "$B/seg-$SEQ.mp4"
}

json_string() {
	# stdin -> JSON string literal (no jq dependency beyond python3 fallback).
	if command -v jq >/dev/null; then jq -Rs .; else python3 -c 'import json,sys;print(json.dumps(sys.stdin.read()))'; fi
}

# record NAME [COLS ROWS] -> build/NAME.mp4 (the step's own video, before fitting)
record() {
	local name=$1 cols=${2:-$COLS} rows=${3:-$ROWS} script="$P/steps/$1.sh"
	[ -f "$script" ] || die "missing $script"
	(cd "$P" && asciinema rec --headless --overwrite --quiet \
		--window-size "${cols}x${rows}" --idle-time-limit "$IDLE" \
		--command "$SHELL_CMD -c '. \"$HERE/typed.sh\"; . \"$script\"'" "$B/$name.cast" </dev/null)
	agg --font-size "$FONT_SIZE" --theme "$THEME" --idle-time-limit "$IDLE" \
		--last-frame-duration 2 --no-loop "$B/$name.cast" "$B/$name.gif" >/dev/null
	ffmpeg -nostdin -hide_banner -loglevel error -y -i "$B/$name.gif" -vf "pad=ceil(iw/2)*2:ceil(ih/2)*2" "${ENC[@]}" "$B/$name.mp4"
}

dur() { ffprobe -v error -show_entries format=duration -of csv=p=0 "$1"; }

step() { record "$1"; fit "$B/$1.mp4" "$B/seg-$SEQ.mp4"; }

pair() {
	local l=$1 r=$2 dl
	record "$l" "$PAIR_COLS" "$PAIR_ROWS"; record "$r" "$PAIR_COLS" "$PAIR_ROWS"
	dl=$(dur "$B/$l.mp4")
	# Each half fills half the frame; the right half holds its first frame
	# while the left runs, the left holds its last frame afterwards.
	ffmpeg -nostdin -hide_banner -loglevel error -y -i "$B/$l.mp4" -i "$B/$r.mp4" -filter_complex \
		"[0:v]scale=$((W / 2 - 8)):-2,tpad=stop_mode=clone:stop_duration=600[a];
		 [1:v]scale=$((W / 2 - 8)):-2,tpad=start_mode=clone:start_duration=${dl}[b];
		 [a][b]hstack=inputs=2:shortest=0,trim=duration=$(awk -v a="$dl" -v b="$(dur "$B/$r.mp4")" 'BEGIN{print a+b}')[v]" \
		-map "[v]" "${ENC[@]}" "$B/pair-$l-$r.mp4"
	fit "$B/pair-$l-$r.mp4" "$B/seg-$SEQ.mp4"
}

build() {
	P=$(cd "${1:?project dir}" && pwd); B="$P/build"
	[ -f "$P/storyboard.txt" ] || die "missing $P/storyboard.txt"
	doctor >/dev/null || { doctor; die "install the missing tools first"; }
	mkdir -p "$B"; rm -f "$B"/seg-*.mp4 "$B/segments.txt"
	SEQ=100
	local kind a b c
	while read -r kind a b c; do
		case $kind in '' | '#'*) continue ;; esac
		SEQ=$((SEQ + 1))
		echo "video: [$SEQ] $kind $a ${b:-}"
		case $kind in
		card) card "$a" "${b:-4}" ;;
		step) step "$a" ;;
		camera) camera "$a" "${b:-75}" "${c:-0}" ;;
		pair) pair "$a" "$b" ;;
		*) die "unknown segment kind: $kind" ;;
		esac
		echo "file 'seg-$SEQ.mp4'" >>"$B/segments.txt"
	done <"$P/storyboard.txt"
	(cd "$B" && ffmpeg -nostdin -hide_banner -loglevel error -y -f concat -safe 0 -i segments.txt -c copy video.mp4)
	if [ -f "$P/narration.m4a" ]; then
		ffmpeg -nostdin -hide_banner -loglevel error -y -i "$B/video.mp4" -i "$P/narration.m4a" \
			-map 0:v -map 1:a -c:v copy "${AENC[@]}" -shortest "$B/demo.mp4"
	else
		cp "$B/video.mp4" "$B/demo.mp4"
	fi
	echo "video: $B/demo.mp4 ($(dur "$B/demo.mp4")s)"
}

case ${1:-} in
doctor) doctor ;;
cameras) ffmpeg -hide_banner -f avfoundation -list_devices true -i "" 2>&1 | grep -E "AVFoundation|\\[[0-9]+\\]" ;;
build) build "${2:-}" ;;
clean) rm -rf "${2:?project dir}/build" ;;
*) echo "usage: video.sh doctor | cameras | build PROJECT_DIR | clean PROJECT_DIR" >&2; exit 2 ;;
esac
