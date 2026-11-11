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

echo "ALL MEDIA TESTS PASSED"
