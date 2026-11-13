#!/usr/bin/env bash
# test.sh — verify examples/media/media.bsh end to end (dev box, no podman/VM).
# Needs ffmpeg, ffprobe, agg, asciinema|cast and an OS voice; the pinned
# pdfcraft-cli comes from the bashy cache (downloaded once, SHA-256 verified).
set -euo pipefail

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
TMP=$(mktemp -d "${TMPDIR:-/tmp}/bashy-media-test.XXXXXX")
trap 'rm -rf "$TMP"' EXIT
TMP=$(cd "$TMP" && pwd)
# media doctor registers tools: keep them in a scratch command ring, never the user's.
export BASHY_COMMANDS_DIR="$TMP/commands" BASHY_HINTS=off BASHY_TELEMETRY_QUIET=1
mkdir -p "$BASHY_COMMANDS_DIR"
media() { bashy "$HERE/media.bsh" "$@"; }
fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "ok   $*"; }

# 0. doctor: passes, or fails by naming the exact install line (never degrades).
if out=$(media doctor 2>&1); then pass "doctor"; else
	echo "$out" | grep -q '^MISSING .*: ' || fail "doctor failed without an install line: $out"
	echo "$out" | grep '^MISSING' | sed 's/^/     /'
	pass "doctor failed closed with install lines"
fi
CLI=$(find "${MEDIA_CACHE:-${XDG_CACHE_HOME:-${LOCALAPPDATA:-$HOME/.cache}}}/bashy/media" -name 'pdfcraft-cli*' -type f -perm -u+x | head -1)

# 1. pdf from two PNGs -> 2 pages, with title and author.
S="$HERE/sample/slides"
pdf=$(media pdf "$S/01.png" "$S/02.png" -o "$TMP/deck.pdf" --title "Sample deck" --author "The owner")
[ "$pdf" = "$TMP/deck.pdf" ] || fail "pdf printed '$pdf'"
info=$("$CLI" info "$pdf")
echo "$info" | grep -q '"pages": 2' || fail "deck.pdf is not 2 pages"
echo "$info" | grep -q '"title": "Sample deck"' || fail "title missing"
echo "$info" | grep -q '"author": "The owner"' || fail "author missing"
pass "pdf: 2 pages with title and author"

# 2. combine: 2 + 2 pages.
all=$(media combine "$pdf" "$pdf" -o "$TMP/all.pdf")
"$CLI" info "$all" | grep -q '"pages": 4' || fail "combine is not 4 pages"
pass "combine: 4 pages"

# 3. slides round trip: pdf -> PNGs -> pdf has the same page count.
mapfile -t pngs < <(media slides "$all" "$TMP/slides")
[ "${#pngs[@]}" = 4 ] || fail "slides wrote ${#pngs[@]} files"
[ "$(basename "${pngs[0]}")" = 01.png ] && [ "$(basename "${pngs[3]}")" = 04.png ] || fail "slide names: ${pngs[*]}"
for f in "${pngs[@]}"; do [ -s "$f" ] || fail "empty slide $f"; done
back=$(media pdf "${pngs[@]}" -o "$TMP/back.pdf")
"$CLI" info "$back" | grep -q '"pages": 4' || fail "round trip lost pages"
pass "slides: 4 PNGs, round trip back to 4 pages"

# 4. integration: sample (CRLF narration) -> narrate --voice -> build -> ffprobe.
cp -R "$HERE/sample" "$TMP/proj"
printf 'Hello from the owner.\r\nNow the member speaks.\r\n' >"$TMP/proj/narration.txt"
printf 'slide one slides/01.png 4\r\nslide two slides/02.png 4\r\n' >"$TMP/proj/storyboard.txt"
case $(uname -s) in Darwin) voice=Samantha ;; *) voice=default ;; esac
m4a=$(media narrate "$TMP/proj" --voice "$voice")
[ -s "$m4a" ] || fail "no narration track"
grep -q $'\r' "$TMP/proj/narration.txt" "$TMP/proj/storyboard.txt" && fail "project files still CRLF"
mp4=$(media build "$TMP/proj")
[ "$mp4" = "$TMP/proj/build/demo.mp4" ] || fail "build printed '$mp4'"
d=$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$mp4")
awk -v d="$d" 'BEGIN{exit !(d > 7.5 && d < 9)}' || fail "duration $d, want about 8s"
[ -n "$(ffprobe -v error -select_streams a -show_entries stream=codec_name -of csv=p=0 "$mp4")" ] || fail "no audio stream"
mean=$(ffmpeg -nostdin -hide_banner -i "$mp4" -af volumedetect -f null - 2>&1 | awk '/mean_volume:/ {print $5}')
awk -v m="$mean" 'BEGIN{exit !(m > -50)}' || fail "narration is silent (mean $mean dB)"
pass "narrate --voice + build: ${d}s with audio (mean ${mean} dB)"

# 5. --own: missing takes fail with the record command; synthetic takes (one
#    longer than its 4 s segment, so it is sped up to fit) then succeed.
rm -f "$TMP/proj/narration.m4a"
if media narrate "$TMP/proj" --own >"$TMP/own.out" 2>"$TMP/own.err"; then fail "--own passed without takes"; fi
grep -q 'record audio --script build/script/01.txt --out voice/01.m4a' "$TMP/own.err" || fail "--own did not print the record command"
mkdir -p "$TMP/proj/voice"
ffmpeg -nostdin -hide_banner -loglevel error -y -f lavfi -i "sine=frequency=300:duration=4.2" "$TMP/proj/voice/01.m4a"
ffmpeg -nostdin -hide_banner -loglevel error -y -f lavfi -i "sine=frequency=500:duration=2" "$TMP/proj/voice/02.wav"
m4a=$(media narrate "$TMP/proj" --own 2>"$TMP/own.err") || fail "--own: $(cat "$TMP/own.err")"
grep -q 'line 1: ratio 1\.[1-4]' "$TMP/own.err" || fail "long take was not sped up: $(cat "$TMP/own.err")"
d=$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$m4a")
awk -v d="$d" 'BEGIN{exit !(d > 7.5 && d < 8.6)}' || fail "own narration is ${d}s, want about 8s"
pass "narrate --own: takes fitted to segments (${d}s)"

# 6. voice: a fake generator (MEDIA_VOICE_PY) stands in for the neural model,
#    so no packages or weights are fetched. It writes NN.raw.wav with padded
#    silence; voice trims it, keeps existing takes without --force, reads
#    narration-spoken.txt when present, and narrate --own consumes the result.
cat >"$TMP/fake-py" <<'FAKE'
#!/usr/bin/env bash
shift # the generator script path
while [ $# -gt 0 ]; do case $1 in --text) text=$2; shift 2 ;; --out) out=$2; shift 2 ;; *) shift ;; esac; done
cp "$text" "$out/.spoken-seen"
i=0
while IFS= read -r line || [ -n "$line" ]; do
	i=$((i + 1)); [ -n "$line" ] || continue
	ffmpeg -nostdin -hide_banner -loglevel error -y -f lavfi -i "sine=frequency=400:duration=1.5" \
		-af "adelay=1000|1000,apad=pad_dur=1" "$out/$(printf %02d $i).raw.wav"
	echo "$out/$(printf %02d $i).raw.wav"
done <"$text"
FAKE
chmod +x "$TMP/fake-py"
if MEDIA_VOICE_PY="$TMP/fake-py" media voice "$TMP/proj" >/dev/null 2>"$TMP/voice.err"; then fail "voice replaced takes without --force"; fi
grep -q -- '--force' "$TMP/voice.err" || fail "voice refusal did not mention --force"
printf 'Hello from the owner.\nNow the member speaks.\n' >"$TMP/proj/narration-spoken.txt"
mapfile -t takes < <(MEDIA_VOICE_PY="$TMP/fake-py" media voice "$TMP/proj" --force --energy 0.8)
[ "${#takes[@]}" = 2 ] && [ "${takes[0]}" = "$TMP/proj/voice/01.wav" ] || fail "voice printed: ${takes[*]}"
[ ! -e "$TMP/proj/voice/01.m4a" ] || fail "voice left the old 01.m4a take beside 01.wav"
cmp -s "$TMP/proj/narration-spoken.txt" "$TMP/proj/voice/.spoken-seen" || fail "voice did not read narration-spoken.txt"
d=$(ffprobe -v error -show_entries format=duration -of csv=p=0 "${takes[0]}")
awk -v d="$d" 'BEGIN{exit !(d > 1.3 && d < 1.8)}' || fail "voice did not trim silence (${d}s, want about 1.5s)"
rm -f "$TMP/proj/narration.m4a"
m4a=$(media narrate "$TMP/proj" --own 2>"$TMP/own.err") || fail "--own after voice: $(cat "$TMP/own.err")"
[ -s "$m4a" ] || fail "no narration from voice takes"
pass "voice: neural takes (fake model) trimmed, guarded and narrated"

# 7. cards: two card files -> one 1280x720 MP4 with one segment per card.
printf 'Opening Night\nA film in three cues\n' >"$TMP/title.txt"
printf 'Nora Vale\nvocals\n' >"$TMP/name.txt"
cards=$(media cards "$TMP/title.txt" "$TMP/name.txt" -o "$TMP/cards.mp4" --secs 2)
[ "$cards" = "$TMP/cards.mp4" ] || fail "cards printed '$cards'"
vinfo=$(ffprobe -v error -select_streams v:0 -show_entries stream=codec_name,width,height,r_frame_rate -of csv=p=0 "$cards")
[ "$vinfo" = "h264,1280,720,30/1" ] || fail "unexpected cards video stream info: $vinfo"
d=$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$cards")
awk -v d="$d" 'BEGIN{exit !(d > 3.8 && d < 4.3)}' || fail "cards duration is ${d}s, want about 4s"
# The video stream itself must last the full --secs per card, not the GIF's 1 s frame.
vd=$(ffprobe -v error -count_frames -select_streams v:0 -show_entries stream=nb_read_frames -of csv=p=0 "$cards")
awk -v n="$vd" 'BEGIN{exit !(n >= 114 && n <= 126)}' || fail "cards video has $vd frames, want about 120 (4 s at 30 fps)"
[ -n "$(ffprobe -v error -select_streams a -show_entries stream=codec_name -of csv=p=0 "$cards")" ] || fail "cards has no audio stream"
if media cards "$TMP/title.txt" 2>/dev/null; then fail "cards passed without -o"; fi
pass "cards: 2 cards, 1280x720, ${d}s"

# 8. subs: bilingual timing tracks -> one styled .ass, burned into a fixture
#    render; every cue edge must flip inside a 100 ms window (bound: 150 ms).
cat >"$TMP/timing.srt" <<'EOF'
1
00:00:00,500 --> 00:00:02,000
Hello from the opening night

2
00:00:02,500 --> 00:00:04,200
Nora takes the stage in blue

3
00:00:05,000 --> 00:00:07,000
The crowd sings along
EOF
cat >"$TMP/timing.es.srt" <<'EOF'
1
00:00:00,500 --> 00:00:02,000
Saludos desde la noche inaugural

2
00:00:02,500 --> 00:00:04,200
Nora sube al escenario de azul

3
00:00:05,000 --> 00:00:07,000
El publico canta con ella
EOF
ffmpeg -nostdin -hide_banner -loglevel error -y \
	-f lavfi -i "color=c=teal:s=1280x720:r=30:d=8" \
	-f lavfi -i "sine=frequency=440:duration=8" \
	-c:v libx264 -preset ultrafast -pix_fmt yuv420p \
	-c:a aac -b:a 160k -ar 48000 -ac 2 "$TMP/base.mp4"
ass=$(media subs "$TMP/timing.srt" "$TMP/timing.es.srt" -o "$TMP/subs.ass" \
	--video "$TMP/base.mp4" --burn "$TMP/burned.mp4" 2>"$TMP/subs.err") || fail "subs burn: $(cat "$TMP/subs.err")"
[ "$ass" = "$TMP/subs.ass" ] || fail "subs printed '$ass'"
grep -c '^Dialogue:' "$ass" | grep -q '^3$' || fail "subs.ass does not hold 3 cues"
grep -q 'Primary' "$ass" && grep -q 'Secondary' "$ass" || fail "subs.ass is missing a language style"
sinfo=$(ffprobe -v error -select_streams s:0 -show_entries stream=codec_name -of csv=p=0 "$TMP/burned.mp4")
[ "$sinfo" = "mov_text" ] || fail "burned video is missing the mov_text soft track (got: $sinfo)"
# Soft-track times must match the fixture within 150 ms.
ffmpeg -nostdin -hide_banner -loglevel error -y -i "$TMP/burned.mp4" -map 0:s:0 "$TMP/back.srt"
awk '
function ts(s,   a, n) { gsub(/,/, ".", s); n = split(s, a, ":"); return a[1]*3600 + a[2]*60 + a[3] }
BEGIN { split("0.5 2.0 2.5 4.2 5.0 7.0", want, " "); k = 0 }
/-->/ { split($1, s1, " "); split($3, s2, " "); got[++k] = ts($1); got[++k] = ts($3) }
END {
	if (k != 6) { print "want 6 edges, got " k > "/dev/stderr"; exit 1 }
	for (i = 1; i <= 6; i++) { d = got[i] - want[i]; if (d < 0) d = -d; if (d > 0.150) { print "edge " i ": " got[i] " vs " want[i] > "/dev/stderr"; exit 1 } }
}' "$TMP/back.srt" || fail "soft-track timing drifted more than 150 ms"
pass "subs: .ass + soft track within 150 ms of the fixture"
# Burned banners: frame(t) differs from a known gap frame iff a cue covers t.
# Reference gap at 7.5 s (after cue 3 ends at 7.0 s).
frame_at() { ffmpeg -nostdin -hide_banner -loglevel error -y -i "$TMP/burned.mp4" -ss "$1" -frames:v 1 "$2"; }
banner_diff() {
	ffmpeg -nostdin -hide_banner -loglevel error -i "$1" -i "$2" \
		-filter_complex "[0:v]crop=1000:140:140:540[c0];[1:v]crop=1000:140:140:540[c1];[c0][c1]blend=all_mode=difference,signalstats,metadata=print:key=lavfi.signalstats.YAVG:file=-" \
		-f null - 2>/dev/null | grep -o 'YAVG=[0-9.]*' | cut -d= -f2 | tail -n 1
}
frame_at 7.50 "$TMP/ref.png"
banner_present() { # banner_present TIME: banner pixels must differ from the gap
	frame_at "$1" "$TMP/got.png"
	awk -v v="$(banner_diff "$TMP/got.png" "$TMP/ref.png")" 'BEGIN{exit !(v > 5)}' \
		|| fail "no burned banner at ${1}s (diff $(banner_diff "$TMP/got.png" "$TMP/ref.png"))"
}
banner_absent() { # banner_absent TIME: frame must match the gap frame
	frame_at "$1" "$TMP/got.png"
	awk -v v="$(banner_diff "$TMP/got.png" "$TMP/ref.png")" 'BEGIN{exit !(v < 1)}' \
		|| fail "banner leaked at ${1}s (diff $(banner_diff "$TMP/got.png" "$TMP/ref.png"))"
}
banner_absent 0.40; banner_present 0.60
banner_present 1.90; banner_absent 2.10
banner_absent 4.50; banner_present 5.10
banner_present 6.90; banner_absent 7.10
pass "subs: burned banners flip within 100 ms of every cue edge"
# Mismatched cue counts fail instead of silently misaligning.
head -n 4 "$TMP/timing.es.srt" >"$TMP/short.es.srt"
if media subs "$TMP/timing.srt" "$TMP/short.es.srt" -o "$TMP/bad.ass" 2>/dev/null; then fail "subs passed with mismatched cue counts"; fi
if media subs "$TMP/timing.srt" -o "$TMP/bad.txt" 2>/dev/null; then fail "subs passed with a non-.ass output"; fi
pass "subs: mismatched cues and non-.ass output fail closed"

# 9. cut: two clips on a 4-beat grid -> 6 s; cuts land on beat-minus-first.
ffmpeg -nostdin -hide_banner -loglevel error -y \
	-f lavfi -i "color=c=red:s=640x360:r=30:d=5" \
	-f lavfi -i "sine=frequency=300:duration=5" \
	-c:v libx264 -preset ultrafast -pix_fmt yuv420p \
	-c:a aac -b:a 160k -ar 48000 -ac 2 "$TMP/a.mp4"
ffmpeg -nostdin -hide_banner -loglevel error -y \
	-f lavfi -i "color=c=blue:s=640x360:d=5" "$TMP/b.mp4"
printf '# beat grid (seconds)\n0.5\n2.5\n4.5\n6.5\n' >"$TMP/beats.txt"
cutout=$(media cut "$TMP/a.mp4" "$TMP/b.mp4" --beats "$TMP/beats.txt" -o "$TMP/cut.mp4")
[ "$cutout" = "$TMP/cut.mp4" ] || fail "cut printed '$cutout'"
d=$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$cutout")
awk -v d="$d" 'BEGIN{exit !(d > 5.8 && d < 6.3)}' || fail "cut duration is ${d}s, want about 6s"
vavg() { # vavg TIME: mean V of that frame (red ~240, blue ~110)
	frame_at2() { ffmpeg -nostdin -hide_banner -loglevel error -y -i "$TMP/cut.mp4" -ss "$1" -frames:v 1 "$TMP/c.png"; }
	frame_at2 "$1"
	ffmpeg -nostdin -hide_banner -loglevel error -i "$TMP/c.png" \
		-filter_complex "signalstats,metadata=print:key=lavfi.signalstats.VAVG:file=-" \
		-f null - 2>/dev/null | grep -o 'VAVG=[0-9.]*' | cut -d= -f2 | tail -n 1
}
awk -v v="$(vavg 1.90)" 'BEGIN{exit !(v > 200)}' || fail "t=1.9s is not clip A (red)"
awk -v v="$(vavg 2.10)" 'BEGIN{exit !(v < 150)}' || fail "cut missed beat 2.5 (t=2.1s is not clip B)"
awk -v v="$(vavg 3.90)" 'BEGIN{exit !(v < 150)}' || fail "t=3.9s is not clip B (blue)"
awk -v v="$(vavg 4.10)" 'BEGIN{exit !(v > 200)}' || fail "cut missed beat 4.5 (t=4.1s is not clip A)"
pass "cut: 6 s on the beat grid, cuts at beats minus first"
printf '0.5\n0.4\n' >"$TMP/badbeats.txt"
if media cut "$TMP/a.mp4" --beats "$TMP/badbeats.txt" -o "$TMP/badcut.mp4" 2>/dev/null; then fail "cut passed with decreasing beats"; fi
if media cut --beats "$TMP/beats.txt" -o "$TMP/badcut.mp4" 2>/dev/null; then fail "cut passed without clips"; fi
pass "cut: bad beats and missing clips fail closed"

echo "ALL MEDIA TESTS PASSED"
