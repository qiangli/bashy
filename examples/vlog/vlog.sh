#!/usr/bin/env bash
# vlog.sh — a tiny static vlog: posts (Markdown-ish, with a video) in,
# a static site out, then publish it two ways.
#
#   vlog.sh build   SITE_DIR            -> SITE_DIR/public/
#   vlog.sh server  [GOOS GOARCH]       -> bin/vlog-server[-GOOS-GOARCH][.exe]
#   vlog.sh serve   SITE_DIR [ADDR]     serve public/ on loopback (foreground)
#   vlog.sh publish-outpost NAME PORT   register the loopback site with Outpost as a public link
#   vlog.sh publish-pages SITE_DIR REPO [BRANCH]
#                                       push public/ to a GitHub Pages branch (default gh-pages)
#
# SITE_DIR holds site.conf (TITLE=, TAGLINE=), posts/*.md and optional
# media/. A post starts with a front matter block:
#   ---
#   title: ...
#   date: YYYY-MM-DD
#   video: path/to/file.mp4   (relative to the post file)
#   summary: ...
#   ---
# then paragraphs separated by blank lines; [text](url) becomes a link.
set -euo pipefail
export BASHY_HINTS=off
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
die() { echo "vlog: $*" >&2; exit 1; }

esc() { local s=$1; s=${s//&/&amp;}; s=${s//</&lt;}; s=${s//>/&gt;}; s=${s//\"/&quot;}; printf '%s' "$s"; }

# inline: escape, then turn [text](http...) into links.
inline() {
	esc "$1" | sed -E 's#\[([^]]+)\]\((https?://[^) ]+)\)#<a href="\2">\1</a>#g'
}

field() { sed -n "/^---$/,/^---$/{s/^$2:[[:space:]]*//p;}" "$1" | head -1; }

body() {
	# Everything after the second '---', paragraphs split on blank lines.
	awk 'f>=2{print} /^---$/{f++}' "$1" | awk -v RS= '{gsub(/\n/," "); print}' |
		while IFS= read -r para; do [ -n "$para" ] && printf '<p>%s</p>\n' "$(inline "$para")"; done
}

page() { # page TITLE BODY_HTML -> full HTML document
	cat <<EOF
<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>$(esc "$1")</title>
<style>
:root{--bg:#fbfbf9;--fg:#1d1d1b;--muted:#6b6b66;--line:#e4e4df;--link:#1f5fbf}
@media (prefers-color-scheme:dark){:root{--bg:#141413;--fg:#ecebe6;--muted:#9a9a93;--line:#2c2c29;--link:#8ab4ff}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--fg);font:17px/1.6 system-ui,-apple-system,"Segoe UI",sans-serif}
main{max-width:46rem;margin:0 auto;padding:2.5rem 1rem 4rem}
header h1{margin:0;font-size:1.6rem}header p{margin:.25rem 0 2rem;color:var(--muted)}
a{color:var(--link)}article{padding:1.5rem 0;border-top:1px solid var(--line)}
h2{margin:0 0 .25rem;font-size:1.25rem}time{color:var(--muted);font-size:.9rem}
video{width:100%;margin:1rem 0;border-radius:8px;background:#000}
footer{margin-top:3rem;color:var(--muted);font-size:.85rem}
</style></head><body><main>
$2
<footer>Made with <a href="https://github.com/qiangli/bashy">bashy</a> · served from the author's own computer</footer>
</main></body></html>
EOF
}

build() {
	local site; site=$(cd "${1:?site dir}" && pwd)
	local out="$site/public" title tagline
	[ -d "$site/posts" ] || die "missing $site/posts"
	title=$(sed -n 's/^TITLE=//p' "$site/site.conf" 2>/dev/null | head -1); title=${title:-vlog}
	tagline=$(sed -n 's/^TAGLINE=//p' "$site/site.conf" 2>/dev/null | head -1)
	rm -rf "$out"; mkdir -p "$out/p" "$out/media"
	[ -d "$site/media" ] && cp -R "$site/media/." "$out/media/"
	local list="" f slug t d v s vid n=0
	# Newest first: posts are named YYYY-MM-DD-slug.md.
	while IFS= read -r f; do
		slug=$(basename "$f" .md)
		t=$(field "$f" title); d=$(field "$f" date); v=$(field "$f" video); s=$(field "$f" summary)
		vid=""
		if [ -n "$v" ]; then
			local src; src=$(cd "$(dirname "$f")" && cd "$(dirname "$v")" 2>/dev/null && pwd)/$(basename "$v")
			if [ -f "$src" ]; then
				cp "$src" "$out/media/$slug.mp4"
				vid="<video controls preload=\"metadata\" src=\"../media/$slug.mp4\"></video>"
			else
				echo "vlog: $slug: video not found: $v (post kept without it)" >&2
			fi
		fi
		page "$t · $title" "<header><p><a href=\"../index.html\">← $(esc "$title")</a></p></header>
<article><h2>$(esc "$t")</h2><time>$(esc "$d")</time>
$vid
$(body "$f")</article>" >"$out/p/$slug.html"
		list+="<article><h2><a href=\"p/$slug.html\">$(esc "$t")</a></h2><time>$(esc "$d")</time><p>$(inline "$s")</p></article>"$'\n'
		n=$((n + 1))
	done < <(ls -1 "$site/posts"/*.md 2>/dev/null | sort -r)
	page "$title" "<header><h1>$(esc "$title")</h1><p>$(esc "$tagline")</p></header>
$list" >"$out/index.html"
	touch "$out/.nojekyll" # GitHub Pages: serve files as-is
	echo "vlog: $n post(s) -> $out"
}

server() {
	local os=${1:-} arch=${2:-} name=vlog-server
	[ -n "$os" ] && name+="-$os-$arch"
	[ "$os" = windows ] && name+=.exe
	mkdir -p "$HERE/bin"
	(cd "$HERE/server" && env GOWORK=off CGO_ENABLED=0 ${os:+GOOS=$os} ${arch:+GOARCH=$arch} go build -trimpath -o "$HERE/bin/$name" .)
	echo "vlog: $HERE/bin/$name"
}

serve() {
	local site; site=$(cd "${1:?site dir}" && pwd)
	[ -x "$HERE/bin/vlog-server" ] || server >/dev/null
	exec "$HERE/bin/vlog-server" -dir "$site/public" -addr "${2:-127.0.0.1:18090}"
}

publish_outpost() {
	local name=${1:?app name} port=${2:-18090}
	# No --require-login: anyone with the link can open it. Add
	# --require-login to limit it to you and the people you share with.
	outpost apps add "$name" --url "http://127.0.0.1:$port"
}

publish_pages() {
	local site repo=${2:?owner/repo} branch=${3:-gh-pages} tmp
	site=$(cd "${1:?site dir}" && pwd)
	[ -f "$site/public/index.html" ] || die "build first: $site/public is empty"
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' RETURN
	if git ls-remote --exit-code --heads "https://github.com/$repo.git" "$branch" >/dev/null 2>&1; then
		git clone -q --depth 1 --branch "$branch" "git@github.com:$repo.git" "$tmp/site"
	else
		git init -q "$tmp/site"; git -C "$tmp/site" checkout -q --orphan "$branch"
		git -C "$tmp/site" remote add origin "git@github.com:$repo.git"
	fi
	find "$tmp/site" -mindepth 1 -maxdepth 1 ! -name .git -exec rm -rf {} +
	cp -R "$site/public/." "$tmp/site/"
	git -C "$tmp/site" add -A
	if git -C "$tmp/site" diff --cached --quiet; then echo "vlog: pages already up to date"; return; fi
	git -C "$tmp/site" commit -q -m "vlog: publish $(date -u +%Y-%m-%dT%H:%MZ)"
	git -C "$tmp/site" push -q origin "$branch"
	echo "vlog: pushed to $repo ($branch)"
}

case ${1:-} in
build) build "${2:-}" ;;
server) server "${2:-}" "${3:-}" ;;
serve) serve "${2:-}" "${3:-}" ;;
publish-outpost) publish_outpost "${2:-}" "${3:-}" ;;
publish-pages) publish_pages "${2:-}" "${3:-}" "${4:-}" ;;
*) sed -n '2,12p' "$0" >&2; exit 2 ;;
esac
