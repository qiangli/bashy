#!/usr/bin/env bash
# tests/test.sh — contract / dry-run test for examples/kb-catalog/kb-catalog.bsh.
# Hermetic: a temp KB ring (written only through `bashy kb` verbs), a stub
# viewer and a stub klatch that record their argv. Nothing is registered and
# no server is contacted. Needs only `bashy` on PATH (or BASHY_BIN).
set -euo pipefail

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
EX=$(cd "$HERE/.." && pwd)
RUNNER=${BASHY_BIN:-bashy}
TMP=$(mktemp -d "${TMPDIR:-/tmp}/bashy-kb-catalog-test.XXXXXX")
trap 'rm -rf "$TMP"' EXIT
TMP=$(cd "$TMP" && pwd)
export BASHY_HINTS=off BASHY_TELEMETRY_QUIET=1
unset KB_CATALOG_CONFIG KB_CATALOG_KB KB_CATALOG_VIEWER KB_CATALOG_KLATCH

catalog() { "$RUNNER" --bashsharp "$EX/kb-catalog.bsh" "$@"; }
command -v jq >/dev/null 2>&1 || jq() { "$RUNNER" jq "$@"; }
fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "ok   $*"; }
has() { [[ $1 == *"$2"* ]] || fail "$3: missing '$2' in: $1"; }
lacks() { [[ $1 != *"$2"* ]] || fail "$3: unexpected '$2' in: $1"; }

# ---- fixtures: a temp KB ring written only with kb verbs -----------------
KB=$TMP/kb
kb() { "$RUNNER" kb --dir "$KB" "$@" >/dev/null 2>&1 || fail "kb $*"; }
kb add --title "Alpha note" --description "alpha: when testing the catalog" --tags demo,alpha --type fact \
  --body "Alpha body. See [[beta-ref]] and https://example.test/alpha?x=1&y=2. Card sprint:614, file docs/README.md."
kb add --title "Beta ref" --slug beta-ref --description "beta target: when linked" --tags demo --type runbook \
  --body "Beta body refers to [[kb:other/zeta]] and todo:abc123."
kb add --title "Gamma lesson" --description "gamma: unrelated" --tags other --type lesson --body "Gamma body, no refs."
pages_before=$(cat "$KB"/pages/*.md | cksum)

# ---- stubs: record argv, answer like the real verbs ----------------------
LOG=$TMP/calls.log
cat >"$TMP/stub-viewer" <<EOF
#!/usr/bin/env bash
printf '%s\n' "viewer \$*" >>"$LOG"
case \$1 in
  add) [[ -f "$TMP/book-exists" ]] && { echo '{"error":{"code":"conflict","message":"A book with that id already exists; add a page with viewer append kb-catalog"},"schema_version":"bashy-viewer-v1"}'; exit 1; }
       touch "$TMP/book-exists"; echo '{"id":"kb-catalog","url":"http://127.0.0.1:18096/?id=kb-catalog","status":"open","schema_version":"bashy-viewer-v1"}' ;;
  open) n=\$(cat "$TMP/pagecount" 2>/dev/null || echo 0)
       echo "{\"id\":\"\$2\",\"url\":\"http://127.0.0.1:18096/?id=\$2\",\"status\":\"open\",\"pages\":\$n,\"schema_version\":\"bashy-viewer-v1\"}" ;;
  append) n=\$(cat "$TMP/pagecount" 2>/dev/null || echo 0); n=\$((n+1)); echo \$n >"$TMP/pagecount"
       # keep the appended Markdown so the test can read what the book got
       f=; while (( \$# )); do [[ \$1 == -f ]] && f=\$2; shift; done
       mkdir -p "$TMP/book"; cp "\$f" "$TMP/book/p\$n.md"
       echo "{\"id\":\"kb-catalog\",\"page\":\$n,\"schema_version\":\"bashy-viewer-v1\"}" ;;
  set|edit|put) echo '{"error":{"code":"usage","message":"viewer is view-only"}}'; exit 2 ;;
  *) echo "{\"error\":{\"code\":\"usage\",\"message\":\"unknown verb \$1\"}}"; exit 2 ;;
esac
EOF
cat >"$TMP/stub-klatch" <<EOF
#!/usr/bin/env bash
printf '%s\n' "klatch \$*" >>"$LOG"
[[ \$1 == messages-send ]] || { echo '{"ok":false,"error":{"code":"USAGE"}}' >&2; exit 2; }
echo '{"ok":true,"result":{"id":"topic-1"}}'
EOF
chmod +x "$TMP/stub-viewer" "$TMP/stub-klatch"
echo 2 >"$TMP/pagecount"            # the book already holds 2 report pages (history to keep)
export KB_CATALOG_VIEWER=$TMP/stub-viewer KB_CATALOG_KLATCH=$TMP/stub-klatch

# 1. help / skill / contract / usage errors
out=$(catalog help); has "$out" "Usage: kb-catalog build|select" help
out=$(catalog --skill); has "$out" "name: kb-catalog" skill; lacks "$out" "~~~" skill-no-fence
out=$(catalog --contract); has "$out" "sokilili kabukatarogu" contract
set +e; err=$(catalog bogus 2>&1 >/dev/null); rc=$?; set -e
[[ $rc == 2 ]] || fail "unknown verb exit $rc, want 2"; has "$err" '"code":"USAGE"' usage-json
set +e; err=$(catalog share --dry-run 2>&1 >/dev/null); rc=$?; set -e
[[ $rc == 2 ]] || fail "share without --klatch exit $rc, want 2"; has "$err" "needs --klatch" share-usage
pass "help, --skill, --contract, USAGE envelopes"

# 2. select: filters compose kb list/search (kind, tag, query, ring scope)
out=$(catalog select --dir "$KB" --json 2>/dev/null)
[[ $(jq -r .count <<<"$out") == 3 ]] || fail "select all: $out"
out=$(catalog select --dir "$KB" --kind runbook --json 2>/dev/null)
[[ $(jq -r '.entries[].ref' <<<"$out") == kb:beta-ref ]] || fail "select --kind: $out"
out=$(catalog select --dir "$KB" --tag alpha --json 2>/dev/null)
[[ $(jq -r '.entries[].ref' <<<"$out") == kb:alpha-note ]] || fail "select --tag: $out"
out=$(catalog select --dir "$KB" --tag alpha --tag other --json 2>/dev/null)
[[ $(jq -r .count <<<"$out") == 2 ]] || fail "select two tags (OR): $out"
out=$(catalog select --dir "$KB" --kind fact --tag other --json 2>/dev/null)
[[ $(jq -r .count <<<"$out") == 0 ]] || fail "select kind AND tag: $out"
out=$(catalog select --dir "$KB" gamma --json 2>/dev/null)
[[ $(jq -r '.entries[].ref' <<<"$out") == kb:gamma-lesson ]] || fail "select query: $out"
has "$(jq -r .filters <<<"$out")" "query=gamma" select-filters
set +e; err=$(catalog select --dir "$KB" --ring nope 2>&1 >/dev/null); rc=$?; set -e
[[ $rc == 2 ]] || fail "--ring nope exit $rc"; has "$err" "repo|host|agent" ring-usage
pass "select: kind, tag (OR), kind AND tag, query terms, ring validation"

# 3. build --dry-run: plan only, no viewer call, pages rendered to --out
: >"$LOG"
out=$(catalog build --dir "$KB" --dry-run --out "$TMP/dry" 2>/dev/null)
[[ $(jq -r .ok <<<"$out") == true && $(jq -r .dry_run <<<"$out") == true ]] || fail "dry-run envelope: $out"
[[ $(jq -r .count <<<"$out") == 3 ]] || fail "dry-run count: $out"
[[ $(jq -r '.commands | length' <<<"$out") == 5 ]] || fail "dry-run plan wants add + 4 appends: $out"
[[ $(jq -r '.commands[0][0:2] | join(" ")' <<<"$out") == "$TMP/stub-viewer add" ]] || fail "plan[0]: $out"
[[ $(jq -r '[.commands[1:][] | .[0:2] | join(" ")] | unique | .[]' <<<"$out") == "$TMP/stub-viewer append" ]] || fail "plan appends: $out"
[[ $(jq -r '.commands[1][4]' <<<"$out") == "KB catalog" && $(jq -r '.commands[1][6]' <<<"$out") == *000-index.md ]] || fail "plan[1] is the index page: $out"
lacks "$(jq -c .commands <<<"$out")" '"set"' dry-run-view-only
[[ ! -s $LOG ]] || fail "dry-run called a stub: $(cat "$LOG")"
[[ -f $TMP/dry/000-index.md && -f $TMP/dry/001-alpha-note.md && -f $TMP/dry/002-beta-ref.md && -f $TMP/dry/003-gamma-lesson.md ]] || fail "dry-run pages: $(ls "$TMP/dry")"
pass "build --dry-run: plan (add + index + 3 pages), no viewer/klatch call, pages written to --out"

# 4. rendered page contract: original stays authoritative, refs are links, backlinks present
alpha=$(cat "$TMP/dry/001-alpha-note.md")
has "$alpha" '# Alpha note' page-title
has "$alpha" '`kb:alpha-note` · fact · candidate · tags: demo, alpha' page-meta
has "$alpha" 'alpha: when testing the catalog' page-summary
has "$alpha" '`bashy kb show kb:alpha-note`' page-original
lacks "$alpha" 'Alpha body.' page-no-body-copy
has "$alpha" '[https://example.test/alpha?x=1&y=2](https://example.test/alpha?x=1&y=2)' ref-url
has "$alpha" 'kb:beta-ref — [Beta ref](?id=kb-catalog&page=3)' ref-kb-in-catalog
has "$alpha" '`bashy kb show kb:beta-ref`' ref-kb-cli
has "$alpha" 'sprint:614 — `bashy define sprint:614`' ref-kind-id
has "$alpha" '`docs/README.md` — file in the owning repo' ref-path
beta=$(cat "$TMP/dry/002-beta-ref.md")
has "$beta" 'kb:other/zeta — `bashy kb show kb:other/zeta`' ref-kb-foreign
[[ $(grep -c 'kb:other/zeta' <<<"$beta") == 1 ]] || fail "foreign ref listed twice: $beta"
has "$beta" 'todo:abc123 — `bashy define todo:abc123`' ref-todo
has "$beta" 'kb:alpha-note — [Alpha note](?id=kb-catalog&page=2) (link)' backlink
gamma=$(cat "$TMP/dry/003-gamma-lesson.md")
has "$gamma" $'## References\n\nnone' refs-none
has "$gamma" $'## Backlinks\n\nnone' backlinks-none
index=$(cat "$TMP/dry/000-index.md")
has "$index" '# KB catalog' index-title
has "$index" '3 entries' index-count
has "$index" '| [Alpha note](?id=kb-catalog&page=2) | fact | candidate | demo, alpha | `bashy kb show kb:alpha-note` |' index-row
has "$index" "Rebuild: \`bashy kb-catalog build --dir $KB\`" index-rebuild
pass "pages: title/summary/kind/tags, original CLI line, refs as links, backlinks, no body copy"

# 5. build against the stub viewer: one book, append-only after the existing pages
: >"$LOG"
out=$(catalog build --dir "$KB" --kind fact --tag demo --json 2>/dev/null)
[[ $(jq -r .ok <<<"$out") == true ]] || fail "build: $out"
[[ $(jq -r .book <<<"$out") == kb-catalog ]] || fail "book id: $out"
[[ $(jq -r .url <<<"$out") == "http://127.0.0.1:18096/?id=kb-catalog" ]] || fail "url: $out"
[[ $(jq -r .index_page <<<"$out") == 3 ]] || fail "index after 2 existing pages: $out"
[[ $(jq -r .index_url <<<"$out") == "http://127.0.0.1:18096/?id=kb-catalog&page=3" ]] || fail "index_url: $out"
[[ $(jq -r '.entries | length' <<<"$out") == 1 && $(jq -r '.entries[0].page' <<<"$out") == 4 ]] || fail "entry pages: $out"
[[ $(grep -c '^viewer add kb-catalog --title KB catalog --json$' "$LOG") == 1 ]] || fail "viewer add once: $(cat "$LOG")"
[[ $(grep -c '^viewer open kb-catalog --json$' "$LOG") == 1 ]] || fail "viewer open once: $(cat "$LOG")"
[[ $(grep -c '^viewer append kb-catalog --title ' "$LOG") == 2 ]] || fail "viewer append index + 1: $(cat "$LOG")"
grep -q '^viewer append kb-catalog --title Alpha note -f .*001-alpha-note.md --json$' "$LOG" || fail "append argv: $(cat "$LOG")"
! grep -qE '^viewer (set|edit|put) ' "$LOG" || fail "a write verb other than add/append was used"
[[ $(cat "$TMP/pagecount") == 4 ]] || fail "book has $(cat "$TMP/pagecount") pages, want 4"
has "$(cat "$TMP/book/p3.md")" '1 entries' book-index-page
has "$(cat "$TMP/book/p4.md")" '# Alpha note' book-entry-page
has "$(cat "$TMP/book/p3.md")" '(kind=fact tags=demo)' book-index-filters
lacks "$(cat "$TMP/book/p4.md")" '&page=' entry-outside-catalog-not-paged   # beta-ref is filtered out: no catalog page link
has "$(cat "$TMP/book/p4.md")" 'kb:beta-ref — `bashy kb show kb:beta-ref`' entry-outside-catalog-cli
pass "build: viewer add once, open, append index+entries after existing pages (history intact), filtered refs fall back to CLI"

# 6. rebuild: existing book is reused (conflict tolerated), new pages appended after the old ones
: >"$LOG"
out=$(catalog build --dir "$KB" --json 2>/dev/null)
[[ $(jq -r .index_page <<<"$out") == 5 ]] || fail "rebuild index page: $out"
[[ $(jq -r '.entries[-1].page' <<<"$out") == 8 ]] || fail "rebuild last page: $out"
[[ $(cat "$TMP/pagecount") == 8 ]] || fail "rebuild page count $(cat "$TMP/pagecount")"
grep -q '^viewer add kb-catalog' "$LOG" || fail "rebuild skipped add"
pass "rebuild: conflict on add tolerated, pages 5-8 appended, earlier pages untouched"

# 7. KB ring untouched by build (reads only; writes go through kb verbs, never the store)
[[ $(cat "$KB"/pages/*.md | cksum) == "$pages_before" ]] || fail "build modified KB pages"
pass "KB store pages unchanged by build"

# 8. open = viewer open passthrough
: >"$LOG"
out=$(catalog open --json 2>/dev/null)
[[ $(jq -r .url <<<"$out") == "http://127.0.0.1:18096/?id=kb-catalog" ]] || fail "open: $out"
grep -q '^viewer open kb-catalog --json$' "$LOG" || fail "open argv: $(cat "$LOG")"
out=$(catalog open --book other-book 2>/dev/null); has "$out" 'other-book' open-book
pass "open: viewer open ID [--json]"

# 9. share --dry-run: plan only; share: klatch messages-send with the viewer URL
: >"$LOG"
out=$(catalog share --klatch C1 --dry-run 2>/dev/null)
[[ $(jq -r .dry_run <<<"$out") == true && $(jq -r .conversation <<<"$out") == C1 ]] || fail "share dry-run: $out"
[[ $(jq -r '.commands[0][1:4] | join(" ")' <<<"$out") == "messages-send channelId=C1 text=KB catalog: http://127.0.0.1:18096/?id=kb-catalog (8 entries; original records via bashy kb show REF)" ]] || fail "share plan: $out"
! grep -q '^klatch' "$LOG" || fail "share --dry-run called klatch: $(cat "$LOG")"
: >"$LOG"
out=$(catalog share --klatch C1 --text "see the KB" --json 2>/dev/null)
[[ $(jq -r .ok <<<"$out") == true && $(jq -r .result.id <<<"$out") == topic-1 ]] || fail "share: $out"
grep -q '^klatch messages-send channelId=C1 text=see the KB$' "$LOG" || fail "share argv: $(cat "$LOG")"
out=$(catalog share --klatch C2 2>/dev/null); has "$out" "shared kb-catalog http://127.0.0.1:18096/?id=kb-catalog -> klatch C2" share-text
pass "share: --dry-run plans messages-send, live call posts channelId/text"

# 10. shaping hooks: another template and config change the page / defaults
cat >"$TMP/entry.md" <<'EOF'
## {{title}} [{{kind}}]
{{summary}}
refs: {{refs}}
EOF
out=$(catalog build --dir "$KB" --template "$TMP/entry.md" --dry-run --out "$TMP/shaped" gamma 2>/dev/null)
[[ $(jq -r .count <<<"$out") == 1 ]] || fail "shaped build: $out"
[[ $(cat "$TMP/shaped/001-gamma-lesson.md") == $'## Gamma lesson [lesson]\ngamma: unrelated\nrefs: none' ]] || fail "shaped page: $(cat "$TMP/shaped/001-gamma-lesson.md")"
cat >"$TMP/catalog.json" <<EOF
{"book":"team-kb","title":"Team KB","kind":"runbook","template":"$TMP/entry.md","index_template":"templates/index.md","share_text":"{{title}} -> {{url}}"}
EOF
out=$(catalog build --dir "$KB" --config "$TMP/catalog.json" --dry-run 2>/dev/null)
[[ $(jq -r .book <<<"$out") == team-kb && $(jq -r '.entries[].ref' <<<"$out") == kb:beta-ref ]] || fail "config defaults: $out"
out=$(catalog share --klatch C3 --config "$TMP/catalog.json" --dry-run 2>/dev/null)
[[ $(jq -r .text <<<"$out") == "Team KB -> http://127.0.0.1:18096/?id=team-kb" ]] || fail "config share_text: $out"
set +e; err=$(catalog build --dir "$KB" --template "$TMP/missing.md" --dry-run 2>&1 >/dev/null); rc=$?; set -e
[[ $rc == 2 ]] || fail "missing template exit $rc"; has "$err" TEMPLATE_NOT_FOUND template-error
pass "shaping: --template layout, --config defaults (book, kind, share_text), actionable template error"

# 11. empty selection is an actionable error, not an empty book
: >"$LOG"
set +e; err=$(catalog build --dir "$KB" --kind decision --dry-run 2>&1 >/dev/null); rc=$?; set -e
[[ $rc == 1 ]] || fail "empty selection exit $rc"; has "$err" NO_ENTRIES empty-error
[[ ! -s $LOG ]] || fail "empty selection touched the viewer"
pass "empty selection: NO_ENTRIES, nothing appended"

echo "all kb-catalog contract tests passed"
