#!/usr/bin/env bash
# video.sh — turn a storyboard of title cards and recorded terminal steps
# into one MP4. Composes three host tools: asciinema (record), agg (render
# a cast to GIF) and ffmpeg (encode, stack, concatenate, add narration).
#
# usage: video.sh doctor | cameras | build|render|run [--subtitles FILE] PROJECT_DIR | clean PROJECT_DIR
#
# PROJECT_DIR holds:
#   storyboard.txt   one segment per line (# comments allowed):
#                      card NAME [SECONDS]   cards/NAME.txt, first line bold
#                      slide [NAME] IMAGE [SECONDS]
#                                            static PNG/JPEG image or slide (alias: image)
#                      step NAME             steps/NAME.sh, recorded headless
#                      camera NAME [SECONDS] [FIT]
#                                            camera/NAME.mov|.mp4, or record the
#                                            webcam + mic now (macOS); FIT speeds
#                                            a longer take up to FIT seconds;
#                                            then words: enhance (voice cleanup,
#                                            -16 LUFS, light denoise/colour, fades;
#                                            also when camera/NAME.*.json says
#                                            "enhance": true) and reframe (drop
#                                            the top 20%, centre the subject)
#                      pair LEFT RIGHT       two steps side by side; RIGHT
#                                            waits on its first frame until
#                                            LEFT has finished
#   cards/*.txt, steps/*.sh, slides/*.png|*.jpg
#   steps/NAME.m4a|.wav|.mp3, cards/NAME.m4a, slides/NAME.m4a
#                    optional per-segment narration sidecars
#   narration.m4a    optional voice track, mixed with segment/camera audio
#   captions.srt|.vtt (or --subtitles FILE)
#                    optional captions burned into video and muxed as mov_text
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
IDLE=${VIDEO_IDLE:-4}
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

# find_sidecar DIR NAME [EXPLICIT]: locate a per-segment narration file if present.
find_sidecar() {
	local dir=$1 name=$2 explicit=${3:-} c
	if [ -n "$explicit" ]; then
		for c in "$P/$explicit" "$explicit"; do
			[ -f "$c" ] && { printf '%s\n' "$c"; return 0; }
		done
	fi
	for c in \
		"$P/$dir/$name.m4a" "$P/$dir/$name.wav" "$P/$dir/$name.mp3" "$P/$dir/$name.aac" \
		"$P/steps/$name.m4a" "$P/steps/$name.wav" "$P/steps/$name.mp3" \
		"$P/cards/$name.m4a" "$P/cards/$name.wav" "$P/cards/$name.mp3" \
		"$P/slides/$name.m4a" "$P/slides/$name.wav" "$P/slides/$name.mp3" \
		"$P/narration/$name.m4a" "$P/narration/$name.wav" "$P/narration/$name.mp3"; do
		[ -f "$c" ] && { printf '%s\n' "$c"; return 0; }
	done
	return 0
}

# fit SRC DST [SIDECAR]: scale into WxH, pad to exactly WxH, normalize fps and codecs;
# attach SIDECAR audio if given, else keep SRC's audio, else add a silent track.
fit() {
	local src=$1 dst=$2 sidecar=${3:-}
	local vf="scale=${W}:${H}:force_original_aspect_ratio=decrease:flags=lanczos,pad=${W}:${H}:(ow-iw)/2:(oh-ih)/2:color=black,fps=${FPS},format=yuv420p"
	if [ -n "$sidecar" ] && [ -f "$sidecar" ]; then
		local vd ad extra
		vd=$(dur "$src" 2>/dev/null || echo 0)
		ad=$(dur "$sidecar" 2>/dev/null || echo 0)
		extra=$(awk -v a="${ad:-0}" -v v="${vd:-0}" 'BEGIN{d=a-v; if(d>0.05) printf "%.3f", d}')
		[ -n "$extra" ] && vf+=",tpad=stop_mode=clone:stop_duration=$extra"
		ffmpeg -nostdin -hide_banner -loglevel error -y -i "$src" -i "$sidecar" \
			-map 0:v:0 -map 1:a:0 -vf "$vf" \
			-af "aformat=sample_rates=48000:channel_layouts=stereo,apad" \
			"${VENC[@]}" "${AENC[@]}" -shortest "$dst"
	elif [ -n "$(ffprobe -v error -select_streams a -show_entries stream=index -of csv=p=0 "$src")" ]; then
		ffmpeg -nostdin -hide_banner -loglevel error -y -i "$src" -vf "$vf" \
			-af "aformat=sample_rates=48000:channel_layouts=stereo" \
			"${VENC[@]}" "${AENC[@]}" "$dst"
	else
		ffmpeg -nostdin -hide_banner -loglevel error -y -i "$src" -f lavfi -i anullsrc=r=48000:cl=stereo \
			-map 0:v -map 1:a -shortest -vf "$vf" "${VENC[@]}" "${AENC[@]}" "$dst"
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
	local target=${3:-0} d r take=$src
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
	local opts=" ${4:-} " vf="" af=""
	# The choice file sits next to the original take, not the fitted copy.
	grep -qs '"enhance": *true' "$take.json" && opts+=" enhance "
	case $opts in *" reframe "*) vf="crop=iw*0.8:ih*0.8:iw*0.1:ih*0.2," ;; esac
	case $opts in *" enhance "*)
		vf+="hqdn3d=1.5:1.5:4:4,eq=brightness=0.03:contrast=1.06:saturation=1.05,unsharp=5:5:0.6,"
		af="highpass=f=80,afftdn=nf=-25,acompressor=threshold=-20dB:ratio=3:attack=5:release=80,loudnorm=I=-16:TP=-1.5:LRA=11"
		;;
	esac
	if [ -n "$vf$af" ]; then
		echo "video: camera $name:${opts// /,}" | tr -s , >&2
		ffmpeg -nostdin -hide_banner -loglevel error -y -i "$src" \
			-vf "${vf}null" ${af:+-af "$af"} "${VENC[@]}" "${AENC[@]}" "$B/camera-$name-touch.mp4"
		src="$B/camera-$name-touch.mp4"
	fi
	fit "$src" "$B/seg-$SEQ.mp4"
}

# card NAME [SECONDS] [AUDIO]: a one-frame cast with the card's text, centred.
card() {
	local name=$1 secs=${2:-4} audio=${3:-} txt="$P/cards/$1.txt" cast="$B/card-$1.cast" sidecar
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
	sidecar=$(find_sidecar cards "$name" "$audio")
	fit "$B/card-$name.gif" "$B/seg-$SEQ.mp4" "$sidecar"
}

# slide [NAME] IMAGE [SECONDS] [AUDIO]: static PNG/JPEG image scaled to 1080p30.
slide() {
	local a=${1:?slide image or name} b=${2:-} c=${3:-} d=${4:-}
	local name ref secs audio img="" cand sidecar
	if [ -z "$b" ] || [[ $b =~ ^[0-9]+(\.[0-9]+)?$ ]]; then
		ref=$a
		name=$(basename "$a"); name=${name%.*}
		secs=${b:-3}
		audio=$c
	else
		name=$a
		ref=$b
		secs=${c:-3}
		audio=$d
	fi
	for cand in "$P/$ref" "$ref" "$P/slides/$ref" \
		"$P/slides/$ref.png" "$P/slides/$ref.jpg" "$P/slides/$ref.jpeg" \
		"$P/cards/$ref.png" "$P/cards/$ref.jpg" "$P/cards/$ref.jpeg" \
		"$P/$ref.png" "$P/$ref.jpg" "$P/$ref.jpeg"; do
		if [ -f "$cand" ]; then img=$cand; break; fi
	done
	[ -n "$img" ] || die "missing slide image: $ref"
	sidecar=$(find_sidecar slides "$name" "$audio")
	ffmpeg -nostdin -hide_banner -loglevel error -y -loop 1 -framerate "$FPS" -t "$secs" -i "$img" \
		-vf "scale=${W}:${H}:force_original_aspect_ratio=decrease:flags=lanczos,pad=${W}:${H}:(ow-iw)/2:(oh-ih)/2:color=black,fps=${FPS},format=yuv420p" \
		"${ENC[@]}" "$B/slide-$name.mp4"
	fit "$B/slide-$name.mp4" "$B/seg-$SEQ.mp4" "$sidecar"
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

step() {
	local sidecar
	record "$1"
	sidecar=$(find_sidecar steps "$1" "${2:-}")
	fit "$B/$1.mp4" "$B/seg-$SEQ.mp4" "$sidecar"
}

pair() {
	local l=$1 r=$2 audio=${3:-} dl sidecar
	record "$l" "$PAIR_COLS" "$PAIR_ROWS"; record "$r" "$PAIR_COLS" "$PAIR_ROWS"
	dl=$(dur "$B/$l.mp4")
	# Each half fills half the frame; the right half holds its first frame
	# while the left runs, the left holds its last frame afterwards.
	ffmpeg -nostdin -hide_banner -loglevel error -y -i "$B/$l.mp4" -i "$B/$r.mp4" -filter_complex \
		"[0:v]scale=$((W / 2 - 8)):-2,tpad=stop_mode=clone:stop_duration=600[a];
		 [1:v]scale=$((W / 2 - 8)):-2,tpad=start_mode=clone:start_duration=${dl}[b];
		 [a][b]hstack=inputs=2:shortest=0,trim=duration=$(awk -v a="$dl" -v b="$(dur "$B/$r.mp4")" 'BEGIN{print a+b}')[v]" \
		-map "[v]" "${ENC[@]}" "$B/pair-$l-$r.mp4"
	sidecar=$(find_sidecar steps "$l-$r" "$audio")
	fit "$B/pair-$l-$r.mp4" "$B/seg-$SEQ.mp4" "$sidecar"
}

# parse_subtitles FILE: emit "start_sec<TAB>end_sec<TAB>text" for SRT or WebVTT cues.
parse_subtitles() {
	awk '
	function ts(s,   a, n) {
		gsub(/,/, ".", s)
		n = split(s, a, ":")
		if (n == 3) return a[1]*3600 + a[2]*60 + a[3]
		if (n == 2) return a[1]*60 + a[2]
		return s + 0
	}
	function flush() {
		if (in_cue && txt != "") {
			gsub(/<[^>]+>/, "", txt)
			printf "%.3f\t%.3f\t%s\n", s_sec, e_sec, txt
		}
		in_cue = 0; txt = ""
	}
	{
		sub(/\r$/, "")
		if ($0 ~ /^[ \t]*$/) { flush(); next }
		if ($0 ~ /-->/) {
			flush()
			split($0, p, /[ \t]*-->[ \t]*/)
			split(p[1], s_tok, /[ \t]+/)
			split(p[2], e_tok, /[ \t]+/)
			s_sec = ts(s_tok[1]); e_sec = ts(e_tok[1])
			in_cue = (e_sec > s_sec)
			next
		}
		if (in_cue) txt = (txt == "" ? $0 : txt " " $0)
	}
	END { flush() }
	' "$1"
}

# apply_subtitles SRC_MP4 SUB_FILE DST_MP4: burn caption banners into the video
# stream and mux a mov_text soft subtitle track into the MP4 container.
apply_subtitles() {
	local src=$1 sub=$2 dst=$3 idx=0 s e txt cols pad line cast gif
	local inputs=(-i "$src") fc=() prev="[0:v]"
	while IFS=$'\t' read -r s e txt; do
		[ -n "${txt:-}" ] || continue
		idx=$((idx + 1))
		cast="$B/sub-$idx.cast"; gif="$B/sub-$idx.gif"
		cols=$(( ${#txt} + 4 )); ((cols < 36)) && cols=36
		pad=$(((cols - ${#txt}) / 2)); ((pad < 0)) && pad=0
		line=$(printf '%*s\e[1m%s\e[0m\e[?25l' "$pad" '' "$txt")
		{
			printf '{"version": 2, "width": %d, "height": 1}\n' "$cols"
			printf '[0.0, "o", %s]\n' "$(printf '%s' "$line" | json_string)"
		} >"$cast"
		agg --font-size 28 --theme "$THEME" --last-frame-duration 1 --no-loop "$cast" "$gif" >/dev/null
		inputs+=(-i "$gif")
		fc+=("${prev}[${idx}:v]overlay=(W-w)/2:H-h-40:enable='between(t,${s},${e})'[v${idx}]")
		prev="[v${idx}]"
	done < <(parse_subtitles "$sub")
	if ((idx > 0)); then
		local sub_in=$((idx + 1)) IFS_SAVE=$IFS filter_str
		IFS=';'; filter_str="${fc[*]}"; IFS=$IFS_SAVE
		inputs+=(-i "$sub")
		ffmpeg -nostdin -hide_banner -loglevel error -y "${inputs[@]}" \
			-filter_complex "$filter_str" \
			-map "$prev" -map 0:a:0 -map "${sub_in}:s:0" \
			"${VENC[@]}" -c:a copy -c:s mov_text -metadata:s:s:0 language=eng "$dst"
	else
		ffmpeg -nostdin -hide_banner -loglevel error -y -i "$src" -i "$sub" \
			-map 0:v:0 -map 0:a:0 -map 1:s:0 \
			-c:v copy -c:a copy -c:s mov_text -metadata:s:s:0 language=eng "$dst"
	fi
}

build() {
	local proj="" sub_arg="${VIDEO_SUBTITLES:-}"
	while (($# > 0)); do
		case $1 in
		--subtitles) sub_arg=${2:?missing --subtitles file}; shift 2 ;;
		--subtitles=*) sub_arg=${1#--subtitles=}; shift ;;
		*) proj=$1; shift ;;
		esac
	done
	P=$(cd "${proj:?project dir}" && pwd); B="$P/build"
	[ -f "$P/storyboard.txt" ] || die "missing $P/storyboard.txt"
	doctor >/dev/null || { doctor; die "install the missing tools first"; }
	mkdir -p "$B"; rm -f "$B"/seg-*.mp4 "$B"/sub-*.cast "$B"/sub-*.gif "$B/segments.txt"
	SEQ=100
	local kind a b c d
	while read -r kind a b c d; do
		case $kind in '' | '#'*) continue ;; esac
		case $kind in
		subtitles | captions)
			[ -z "$sub_arg" ] && sub_arg=$a
			continue
			;;
		esac
		SEQ=$((SEQ + 1))
		echo "video: [$SEQ] $kind $a ${b:-}"
		case $kind in
		card) card "$a" "${b:-4}" "${c:-}" ;;
		slide | image) slide "$a" "${b:-}" "${c:-}" "${d:-}" ;;
		step) step "$a" "${b:-}" ;;
		camera) camera "$a" "${b:-75}" "${c:-0}" "${d:-}" ;;
		pair) pair "$a" "$b" "${c:-}" ;;
		*) die "unknown segment kind: $kind" ;;
		esac
		echo "file 'seg-$SEQ.mp4'" >>"$B/segments.txt"
	done <"$P/storyboard.txt"
	(cd "$B" && ffmpeg -nostdin -hide_banner -loglevel error -y -f concat -safe 0 -i segments.txt -c copy video.mp4)
	local staged="$B/video.mp4"
	if [ -f "$P/narration.m4a" ]; then
		staged="$B/narrated.mp4"
		if [ "${VIDEO_NARRATION_MODE:-mix}" = replace ]; then
			ffmpeg -nostdin -hide_banner -loglevel error -y -i "$B/video.mp4" -i "$P/narration.m4a" \
				-map 0:v:0 -map 1:a:0 -c:v copy \
				-af "aformat=sample_rates=48000:channel_layouts=stereo,apad" \
				"${AENC[@]}" -shortest "$staged"
		else
			# Mix global narration with segment audio so camera presenter speech
			# and per-segment sidecar voice-overs are preserved alongside narration.
			ffmpeg -nostdin -hide_banner -loglevel error -y -i "$B/video.mp4" -i "$P/narration.m4a" \
				-filter_complex "[0:a][1:a]amix=inputs=2:duration=first:dropout_transition=0:normalize=0,aformat=sample_rates=48000:channel_layouts=stereo[a]" \
				-map 0:v:0 -map "[a]" -c:v copy "${AENC[@]}" "$staged"
		fi
	fi
	local sub_file="" cand
	if [ -n "$sub_arg" ]; then
		for cand in "$sub_arg" "$P/$sub_arg"; do
			[ -f "$cand" ] && { sub_file=$cand; break; }
		done
		[ -n "$sub_file" ] || die "missing subtitles file: $sub_arg"
	else
		for cand in "$P/captions.srt" "$P/captions.vtt" "$P/subtitles.srt" "$P/subtitles.vtt"; do
			[ -f "$cand" ] && { sub_file=$cand; break; }
		done
	fi
	if [ -n "$sub_file" ]; then
		apply_subtitles "$staged" "$sub_file" "$B/demo.mp4"
	else
		cp "$staged" "$B/demo.mp4"
	fi
	echo "video: $B/demo.mp4 ($(dur "$B/demo.mp4")s)"
}

case ${1:-} in
doctor) doctor ;;
cameras) ffmpeg -hide_banner -f avfoundation -list_devices true -i "" 2>&1 | grep -E "AVFoundation|\\[[0-9]+\\]" ;;
build | render | run) shift; build "$@" ;;
clean) rm -rf "${2:?project dir}/build" ;;
*) echo "usage: video.sh doctor | cameras | build|render|run [--subtitles FILE] PROJECT_DIR | clean PROJECT_DIR" >&2; exit 2 ;;
esac
