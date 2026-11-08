#!/usr/bin/env bash
# test.sh — end-to-end verification for examples/video/video.sh in an isolated sandbox.
set -euo pipefail

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
TMP=$(mktemp -d "${TMPDIR:-/tmp}/bashy-video-test.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

export HOME="$TMP/home"
export BASHY_HOME="$TMP/bashy"
export XDG_CONFIG_HOME="$TMP/xdg/config"
export XDG_DATA_HOME="$TMP/xdg/data"
export XDG_STATE_HOME="$TMP/xdg/state"
export XDG_CACHE_HOME="$TMP/xdg/cache"
export GIT_AUTHOR_NAME="Demo Presenter"
export GIT_AUTHOR_EMAIL="presenter@example.test"
export GIT_COMMITTER_NAME="Demo Presenter"
export GIT_COMMITTER_EMAIL="presenter@example.test"
mkdir -p "$HOME" "$BASHY_HOME" "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$XDG_STATE_HOME" "$XDG_CACHE_HOME"

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "ok   $*"; }

# 1. Doctor check
"$HERE/video.sh" doctor >/dev/null
pass "doctor check passed"

# 2. Build a full sandbox project testing card, camera (with presenter audio),
#    slide (PNG diagram), step (with per-step steps/<name>.m4a sidecar),
#    global narration.m4a mixing, and SRT subtitle burn-in + mov_text track.
PROJ="$TMP/proj"
mkdir -p "$PROJ/cards" "$PROJ/steps" "$PROJ/camera" "$PROJ/slides"

cat >"$PROJ/cards/title.txt" <<'EOF'
Bashy Video Demo
Sandbox End-to-End Verification
EOF

cat >"$PROJ/cards/close.txt" <<'EOF'
Done
All segments verified
EOF

# Generate a synthetic camera take (2s) with 440 Hz presenter voice audio
ffmpeg -nostdin -hide_banner -loglevel error -y \
	-f lavfi -i "color=c=navy:s=1280x720:r=30:d=2" \
	-f lavfi -i "sine=frequency=440:sample_rate=48000:duration=2" \
	-c:v libx264 -preset ultrafast -pix_fmt yuv420p \
	-c:a aac -b:a 160k -ar 48000 -ac 2 \
	"$PROJ/camera/intro.mp4"

# Generate a static PNG slide (640x360) to verify 1080p30 scaling + padding
ffmpeg -nostdin -hide_banner -loglevel error -y \
	-f lavfi -i "color=c=teal:s=640x360:d=1" -frames:v 1 \
	"$PROJ/slides/arch.png"

# Create a terminal step and its per-step narration sidecar (steps/hello.m4a)
cat >"$PROJ/steps/hello.sh" <<'EOF'
TYPE_DELAY=0.002
show "echo hello-from-step"
EOF

ffmpeg -nostdin -hide_banner -loglevel error -y \
	-f lavfi -i "sine=frequency=660:sample_rate=48000:duration=2" \
	-c:a aac -b:a 160k -ar 48000 -ac 2 \
	"$PROJ/steps/hello.m4a"

# Create a global narration.m4a track that is silent during the camera take (0..4s)
# and plays an 880 Hz tone later, verifying that mixing preserves camera speech
# during 2..4s AND per-step sidecar audio during the step segment.
ffmpeg -nostdin -hide_banner -loglevel error -y \
	-f lavfi -i "anullsrc=r=48000:cl=stereo:d=6" \
	-f lavfi -i "sine=frequency=880:sample_rate=48000:duration=4" \
	-filter_complex "[0:a][1:a]concat=n=2:v=0:a=1[a]" \
	-map "[a]" -c:a aac -b:a 160k -ar 48000 -ac 2 \
	"$PROJ/narration.m4a"

# Create SRT subtitles
cat >"$PROJ/captions.srt" <<'EOF'
1
00:00:00,200 --> 00:00:01,800
Welcome to the Bashy video pipeline

2
00:00:02,200 --> 00:00:03,800
Camera presenter speech is preserved
EOF

cat >"$PROJ/storyboard.txt" <<'EOF'
card title 2
camera intro 2
slide arch slides/arch.png 2
image slides/arch.png 1
step hello
card close 2
EOF

"$HERE/video.sh" render --subtitles "$PROJ/captions.srt" "$PROJ" >/dev/null
OUT="$PROJ/build/demo.mp4"
[ -f "$OUT" ] || fail "missing output $OUT"
pass "rendered $OUT"

# 3. Verify video stream is 1920x1080 @ 30fps H.264, audio is 48kHz stereo AAC,
#    and subtitle stream is mov_text.
vinfo=$(ffprobe -v error -select_streams v:0 -show_entries stream=codec_name,width,height,r_frame_rate -of csv=p=0 "$OUT")
[ "$vinfo" = "h264,1920,1080,30/1" ] || fail "unexpected video stream info: $vinfo"
pass "video stream is h264 1920x1080 30fps"

ainfo=$(ffprobe -v error -select_streams a:0 -show_entries stream=codec_name,sample_rate,channels -of csv=p=0 "$OUT")
[ "$ainfo" = "aac,48000,2" ] || fail "unexpected audio stream info: $ainfo"
pass "audio stream is aac 48000Hz stereo"

sinfo=$(ffprobe -v error -select_streams s:0 -show_entries stream=codec_name -of csv=p=0 "$OUT")
[ "$sinfo" = "mov_text" ] || fail "missing mov_text subtitle stream (got: $sinfo)"
pass "subtitle stream (mov_text) muxed"

# 4. Verify slide segment was rendered at 1920x1080 30fps
slide_info=$(ffprobe -v error -select_streams v:0 -show_entries stream=codec_name,width,height,r_frame_rate -of csv=p=0 "$PROJ/build/seg-103.mp4")
[ "$slide_info" = "h264,1920,1080,30/1" ] || fail "unexpected slide segment info: $slide_info"
pass "slide segment rendered at 1920x1080 30fps"

# 5. Verify camera presenter audio (t=2..4s, seg-102) and per-step sidecar audio
#    (seg-105) are preserved with non-zero audio energy in final demo.mp4.
audio_stays_non_silent() {
	local file=$1 ss=$2 dur=$3 label=$4 mean_db
	mean_db=$(ffmpeg -nostdin -hide_banner -ss "$ss" -t "$dur" -i "$file" -af volumedetect -f null - 2>&1 | awk '/mean_volume:/ {print $5}')
	[ -n "$mean_db" ] || fail "could not read mean_volume for $label"
	awk -v m="$mean_db" 'BEGIN { exit !(m > -45.0) }' || fail "$label audio was silent (mean_volume=${mean_db} dB)"
	pass "$label audio preserved (mean_volume=${mean_db} dB)"
}

audio_stays_non_silent "$PROJ/build/seg-105.mp4" 0 1.5 "per-step sidecar (seg-105)"
audio_stays_non_silent "$OUT" 2.2 1.5 "camera presenter in final demo.mp4"
audio_stays_non_silent "$OUT" 7.2 1.5 "narrated step in final demo.mp4"

# 6. Verify visual subtitle burn-in created sub-1.gif and sub-2.gif overlays
[ -f "$PROJ/build/sub-1.gif" ] && [ -f "$PROJ/build/sub-2.gif" ] || fail "missing burned-in subtitle overlays"
pass "subtitle burn-in overlays rendered and applied"

echo "ALL VIDEO TESTS PASSED"
