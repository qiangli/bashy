#!/usr/bin/env bash
# Release gate: pinned govulncheck in source mode over bashy and the siblings it
# ships (outpost, yoke), each as its own module with GOWORK=off — exactly what a
# release build resolves. govulncheck is BUILT with the release toolchain (the
# `toolchain` line of bashy's go.mod), so the stdlib it judges is the one the
# release links. Same verdict rule as builds/_shared/kinds/go/audit.yml: a
# finding whose first trace frame names a function is REACHABLE and fails the
# gate; package- and module-level findings are reported, never waived away.
#
# usage: scripts/govulncheck-release.sh [MODULE_DIR...]   (default: . ../outpost ../yoke)
# env:   GOVULNCHECK_VERSION (default v1.9.0), GOVULNCHECK_OUT (report directory)
set -euo pipefail
cd "$(dirname "$0")/.."
root=$PWD
version=${GOVULNCHECK_VERSION:-v1.9.0}
out=${GOVULNCHECK_OUT:-$root/bin/govulncheck}
want=$(sed -n 's/^toolchain \(go[0-9][0-9.]*\)[[:space:]]*$/\1/p' go.mod | head -n1)
[[ -n $want ]] || { echo "govulncheck-release: go.mod declares no toolchain" >&2; exit 1; }
export GOTOOLCHAIN=$want GOWORK=off
have=$(go env GOVERSION)
[[ $have == "$want" ]] || { echo "govulncheck-release: toolchain $have, release needs $want" >&2; exit 1; }
command -v python3 >/dev/null || { echo "govulncheck-release: python3 is required" >&2; exit 1; }

mkdir -p "$out"
bin=$(mktemp -d)
trap 'rm -rf "$bin"' EXIT
GOBIN=$bin go install "golang.org/x/vuln/cmd/govulncheck@$version"
echo "govulncheck $version built with $have"

(($#)) || set -- . ../outpost ../yoke
status=0
for dir in "$@"; do
  mod=$(cd "$dir" && go list -m)
  report=$out/$(basename "$mod").govulncheck.json
  # govulncheck exits 3 when it finds anything; the verdict is rendered below.
  (cd "$dir" && "$bin/govulncheck" -format json ./...) >"$report" || true
  # One line per OSV at its most precise level: REACHABLE (a called symbol),
  # package (imported, vulnerable code not called) or module (required only).
  # Exit 2 means the stream carries no govulncheck config, i.e. no report.
  python3 - "$report" >"$bin/found" <<'PY' ||
import json, sys
dec, text, pos, best, config = json.JSONDecoder(), open(sys.argv[1]).read(), 0, {}, False
while True:
    while pos < len(text) and text[pos].isspace():
        pos += 1
    if pos == len(text):
        break
    msg, pos = dec.raw_decode(text, pos)
    config = config or "config" in msg
    f = msg.get("finding")
    if f:
        top = (f.get("trace") or [{}])[0]
        level = 3 if top.get("function") else 2 if top.get("package") else 1
        if level > best.get(f["osv"], (0, ""))[0]:
            best[f["osv"]] = (level, top.get("module", ""))
if not config:
    sys.exit(2)
for osv, (level, mod) in sorted(best.items()):
    print(f"{['', 'module', 'package', 'REACHABLE'][level]}\t{osv}\t{mod}")
PY
    { echo "$mod: govulncheck produced no report ($report)" >&2; status=1; continue; }
  sort -o "$bin/found" "$bin/found"
  reachable=$(grep -c '^REACHABLE' "$bin/found" || true)
  pkg=$(grep -c '^package' "$bin/found" || true)
  modonly=$(grep -c '^module' "$bin/found" || true)
  echo "$mod: $reachable reachable, $pkg package-level, $modonly module-only (report: $report)"
  sed 's/^/  /' "$bin/found"
  ((reachable == 0)) || status=1
done
((status == 0)) || echo "govulncheck-release: reachable vulnerabilities fail the release gate" >&2
exit "$status"
