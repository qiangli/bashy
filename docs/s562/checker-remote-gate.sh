#!/usr/bin/env bash
# Sprint 562 / Story 7da98b722aca. Parent runs this on the remote gate host.
# Arguments: candidate bashy binary, directory containing the three unchanged
# examples-fenced/{context,http-server,tcp-server} fixture directories.
set -euo pipefail

binary=$1
corpus=$2
command -v python3 >/dev/null
test -x "$binary"
test -f "$corpus/context/context.bsh"
test -f "$corpus/http-server/http-server.bsh"
test -f "$corpus/tcp-server/tcp-server.bsh"

python3 - "$corpus" <<'PY'
import hashlib, pathlib, sys
root = pathlib.Path(sys.argv[1])
expected = {
    'context/context.bsh': '0c73d2f832b95ec6c77adf6f744d6c0289ea33b5d2083e552175fbceb9d520c3',
    'http-server/http-server.bsh': '2f0af538b5e4225567c7b84a7a82acfc099ca4c7da6484e2574d707a80fce57a',
    'tcp-server/tcp-server.bsh': 'bfd7f0f57629bc0dd6eadb3b61f9695ecd8e98cb13f30b092ed4adbf584a0738',
}
for name, digest in expected.items():
    data = (root / name).read_bytes()
    assert hashlib.sha256(data).hexdigest() == digest, name
    assert data.rstrip().endswith(b'g.Main "$@"'), name
PY

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
for name in context http-server tcp-server; do
    "$binary" check --bashsharp --json "$corpus/$name/$name.bsh" >"$tmp/$name.json"
    python3 - "$tmp/$name.json" <<'PY'
import json, sys
r = json.load(open(sys.argv[1]))
assert r['summary']['errors'] == 0, r['diagnostics']
assert any(d.get('command') == 'g.Main' and d.get('resolver') == 'go-fence'
           for d in r['diagnostics']), r['diagnostics']
PY
done

cat >"$tmp/negative.bsh" <<'EOF'
~~~go as g
func Main() {}
func hidden() {}
var Value = 1
func (x *Thing) Method() {}
~~~
other.Main
g.Missing
g.hidden
g.Value
g.Method
a.b
g.Main.Extra
EOF
if "$binary" check --bashsharp --json "$tmp/negative.bsh" >"$tmp/negative.json"; then
    echo 'negative commands unexpectedly passed' >&2
    exit 1
fi
python3 - "$tmp/negative.json" <<'PY'
import json, sys
r = json.load(open(sys.argv[1]))
want = {'other.Main', 'g.Missing', 'g.hidden', 'g.Value', 'g.Method', 'a.b', 'g.Main.Extra'}
got = {x['name'] for x in r['inventory'].get('not_found', [])}
assert got == want, (got, r['diagnostics'])
PY

cat >"$tmp/malformed.bsh" <<'EOF'
~~~go as g
func Main( {
~~~
g.Main
EOF
if "$binary" check --bashsharp --json "$tmp/malformed.bsh" >"$tmp/malformed.json"; then
    echo 'malformed Go source unexpectedly passed' >&2
    exit 1
fi
python3 - "$tmp/malformed.json" <<'PY'
import json, sys
r = json.load(open(sys.argv[1]))
assert any(x['name'] == 'g.Main' for x in r['inventory'].get('not_found', [])), r
PY

printf '#!/bin/sh\nexit 0\n' >"$tmp/hosttool"
chmod +x "$tmp/hosttool"
cat >"$tmp/path.bsh" <<'EOF'
~~~go as g
func Main() int { return 1 }
~~~
result := g.Main()
g.Main
hosttool
missingordinarytool
EOF
if PATH="$tmp:$PATH" "$binary" check --bashsharp --json "$tmp/path.bsh" >"$tmp/path.json"; then
    echo 'missing ordinary command unexpectedly passed' >&2
    exit 1
fi
python3 - "$tmp/path.json" <<'PY'
import json, sys
r = json.load(open(sys.argv[1]))
assert r['summary']['commands'] == 3, r
assert any(x['name'] == 'hosttool' for x in r['inventory'].get('system', [])), r
assert any(x['name'] == 'missingordinarytool' for x in r['inventory'].get('not_found', [])), r
assert any(d.get('command') == 'g.Main' and d.get('resolver') == 'go-fence'
           for d in r['diagnostics']), r
PY
echo 'exact three Go fence CLI positives and negative/PATH controls passed'
