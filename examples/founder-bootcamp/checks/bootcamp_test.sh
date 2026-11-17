#!/usr/bin/env bash
# Founder bootcamp checks: progress, deadlines (with bashy todo mirroring),
# served-HTML site check (bashy fetch), PDF pre-flight (bashy lens), names.
set -eu  # no pipefail: assertions pipe into grep -q, which closes early
export BASHY_HINTS=off
here=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
runner=${BASHY_BIN:-bashy}
work=$(mktemp -d); trap '[[ -n ${srv:-} ]] && { kill "$srv" 2>/dev/null; wait "$srv" 2>/dev/null || true; }; rm -rf "$work"' EXIT
export BOOTCAMP_DIR=$work/records BOOTCAMP_TODAY=2026-11-16 BOOTCAMP_TODO=0
bc() { "$runner" --bashsharp "$here/bootcamp.bsh" "$@"; }
pass=0
ok() { pass=$((pass+1)); printf 'ok %d - %s\n' "$pass" "$1"; }
fail() { printf 'FAIL - %s\n' "$1" >&2; exit 1; }

# Fictitious defaults, and any name is safe.
bc show company | grep -q 'Incorporate Acme Labs, Inc.' || fail 'default company name'
BOOTCAMP_COMPANY='Zed & Co | Ltd' bc show company | grep -qF 'Incorporate Zed & Co | Ltd (' || fail 'name with & and |'
ok 'names come from the environment'

# Progress.
bc done company employment-check --note 'read both agreements' >/dev/null
bc skip company ip-release >/dev/null
bc next company | head -1 | grep -q '^company/atlas-ready' || fail 'next skips done and skipped steps'
bc status --json | grep -q '"track":"company","done":2,"total":9' || fail 'status counts'
bc reopen company ip-release >/dev/null
bc status --json | grep -q '"track":"company","done":1,' || fail 'reopen'
bc show company | grep -q '\[x\] employment-check.*read both agreements' || fail 'note shown'
if bc done company no-such-step 2>/dev/null; then fail 'unknown step accepted'; fi
ok 'progress: done, skip, reopen, next, status'

# Deadlines: overdue, today, within 30 days, later; close by text.
bc deadline add 2026-11-10 'Old filing' >/dev/null
bc deadline add 2026-11-16 'Due today' >/dev/null
bc deadline add 2026-12-01 '83(b) election reaches the IRS' --track company >/dev/null
bc deadline add 2027-10-21 'Non-provisional' --track patent >/dev/null
out=$(bc status)
grep -q 'OVERDUE by 6 days  Old filing' <<<"$out" || fail 'overdue'
grep -q 'TODAY.*Due today' <<<"$out" || fail 'today'
grep -q 'in 15 days.*83(b)' <<<"$out" || fail 'within 30 days'
grep -q 'Non-provisional' <<<"$out" && fail 'later deadline in the 30-day view'
bc deadlines | grep -q 'Non-provisional (patent)' || fail 'full list'
bc deadline done 'Old filing' >/dev/null
bc deadlines | grep -q 'Old filing' && fail 'closed deadline still listed'
ok 'deadlines: overdue, today, 30-day window, close'

# Deadlines mirror into bashy todo inside a records repository.
repo=$work/repo; mkdir -p "$repo"; git -C "$repo" init -q
BOOTCAMP_DIR=$repo/founder BOOTCAMP_TODO=1 bc deadline add 2026-12-15 'Redeem credit code' --track funding | grep -q 'bashy todo' || fail 'mirror message'
grep -rqs 'Redeem credit code' "$repo/docs/todo" || fail 'todo item written in the records repo'
grep -rqs 'founder-bootcamp' "$repo/docs/todo" || fail 'todo label'
ok 'deadlines mirrored into bashy todo'

# Site check reads the served HTML, not what JavaScript renders.
mkdir -p "$work/site/good" "$work/site/spa"
printf '<html><body><h1>Acme Labs, Inc.</h1><p>Founder: Alex Founder</p></body></html>' > "$work/site/good/index.html"
printf '<html><body><div id="root"></div><script>document.body.innerText="Acme Labs, Inc. Alex Founder"</script></body></html>' > "$work/site/spa/index.html"
port=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1])')
(cd "$work/site" && exec python3 -m http.server "$port" --bind 127.0.0.1 >/dev/null 2>&1) & srv=$!
for _ in $(seq 50); do curl -s "http://127.0.0.1:$port/" >/dev/null 2>&1 && break; sleep 0.1; done
bc check-site "http://127.0.0.1:$port/good/" >/dev/null || fail 'served names found'
if bc check-site "http://127.0.0.1:$port/spa/" >/dev/null; then fail 'names only in JavaScript passed'; fi
ok 'check-site: served HTML vs JavaScript-only'

# PDF pre-flight with bashy lens.
python3 - "$work/spec.pdf" <<'PY'
import sys
text = "Abstract " + " ".join(["word"] * 40)
stream = f"BT /F1 12 Tf 72 720 Td ({text}) Tj ET".encode()
objs = [b"<< /Type /Catalog /Pages 2 0 R >>", b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
        b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
        b"<< /Length %d >>\nstream\n" % len(stream) + stream + b"\nendstream",
        b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"]
out = b"%PDF-1.4\n"; offs = []
for i, o in enumerate(objs, 1):
    offs.append(len(out)); out += b"%d 0 obj\n" % i + o + b"\nendobj\n"
x = len(out)
out += b"xref\n0 %d\n0000000000 65535 f \n" % (len(objs) + 1) + b"".join(b"%010d 00000 n \n" % o for o in offs)
out += b"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n" % (len(objs) + 1, x)
open(sys.argv[1], "wb").write(out)
PY
pf=$(bc preflight "$work/spec.pdf")
grep -q '^ok      text extracts' <<<"$pf" || fail "pdf text: $pf"
grep -q '^sha256  [0-9a-f]\{64\}' <<<"$pf" || fail 'sha256'
ok 'preflight: text extracts, checksum'

# Report and go.
bc page "$work/report.html" >/dev/null
grep -q '<h1>Founder bootcamp: Acme Labs, Inc.</h1>' "$work/report.html" || fail 'page title'
grep -q 'href="https://patentcenter.uspto.gov/"' "$work/report.html" || fail 'real site link'
BOOTCAMP_DRY_RUN=1 bc go patent file | grep -q 'patentcenter.uspto.gov' || fail 'go opens the real site'
ok 'page and go use the real sites'

bc --skill | grep -q '^name: founder-bootcamp' || fail 'skill face'
ok 'skill face'

# Pitch deck (bashy browser renders slides; media pdf joins them) and the
# demo-video scaffold for media. Skipped when no browser is available.
bc deck init "$work/pitch" >/dev/null
grep -q '^# Acme Labs, Inc.$' "$work/pitch/deck.md" || fail 'deck outline names the company'
if [[ ${BOOTCAMP_TEST_BROWSER:-1} == 1 ]] && bc deck build "$work/pitch" >/dev/null 2>&1; then
  [[ -s $work/pitch/build/deck.pdf && -f $work/pitch/build/11.png ]] || fail 'deck.pdf with 11 slides'
  bc demo init "$work/demo" --deck "$work/pitch" >/dev/null
  grep -q '^slide problem.png 8$' "$work/demo/storyboard.txt" || fail 'demo uses deck slides'
  [[ $(wc -l < "$work/demo/storyboard.txt") -eq $(wc -l < "$work/demo/narration.txt") ]] || fail 'one narration line per segment'
  ok 'deck built; demo scaffold from the deck'
else
  bc demo init "$work/demo" >/dev/null
  grep -q '^step product$' "$work/demo/storyboard.txt" || fail 'demo scaffold'
  ok 'demo scaffold (deck build skipped: no browser)'
fi
echo "all $pass checks passed"
