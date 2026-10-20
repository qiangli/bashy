#!/usr/bin/env bash
# go-security-overrides.sh SRC
#
# Custom-recipe mirror of the go-kind GO_SECURITY_OVERRIDES mechanism
# (builds/README.md "GO_SECURITY_OVERRIDES", builds/_shared/kinds/go/setup.yml).
# Applies the reviewed module bumps declared in PIN GO_SECURITY_OVERRIDES to
# the pinned upstream checkout at SRC, after the pin gates and before any
# go list / go build / go-licenses / SBOM / govulncheck reads the module graph,
# so inputs, deps, SBOM and binaries all reflect the same overridden tree.
#
# S436/#2028: nine x/* modules at their MVS-selected versions plus
# github.com/buger/jsonparser@v1.1.2 (GO-2026-4514). The one OSV waiver
# (GO-2026-5932) lives in vuln-ignore for the audit job's OSV gate.
#
# Gate (not `go get` itself) is what makes this safe: afterwards go.mod may
# differ ONLY in the require lines of exactly the declared modules, plus the
# go/toolchain directives within the bounds the effective toolchain sets
# (go may rise but never past GO_TOOLCHAIN; toolchain may only drop or lower).
# Anything else fails: a new or removed require, a replace/exclude/retract
# edit, or an undeclared module moved as a transitive effect (declare it too,
# so it is reviewed). go.sum is deliberately NOT gated: `go get` has to add
# the new hashes. There is no `go mod tidy`.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$here/PIN"
src=$(cd "${1:?usage: go-security-overrides.sh SRC}" && pwd)

if [[ -z ${GO_SECURITY_OVERRIDES:-} ]]; then
  echo "GO_SECURITY_OVERRIDES: none declared (no-op)"
  exit 0
fi
for spec in $GO_SECURITY_OVERRIDES; do
  [[ $spec == *@v* ]] || { echo "GO_SECURITY_OVERRIDES entry $spec is not module@vVERSION" >&2; exit 1; }
done
[[ -n ${GO_TOOLCHAIN:-} ]] || { echo "GO_SECURITY_OVERRIDES: PIN GO_TOOLCHAIN is unset" >&2; exit 1; }
effective=$GO_TOOLCHAIN

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# Gated view of go.mod: everything except go.sum. `go mod edit -json` is used
# rather than a textual diff so a reformat, a comment or a moved require block
# cannot hide a real change.
gated() {
  (cd "$src" && go mod edit -json go.mod) | python3 -c '
import json, sys
d = json.load(sys.stdin)
print("module\t" + (d.get("Module", {}).get("Path", "")))
print("go\t" + (d.get("Go") or ""))
print("toolchain\t" + (d.get("Toolchain") or ""))
for r in sorted(d.get("Require") or [], key=lambda x: x["Path"]):
    print("require\t" + r["Path"] + "\t" + r["Version"] + "\t" + str(bool(r.get("Indirect"))).lower())
for e in d.get("Exclude") or []:
    print("exclude\t" + e["Path"] + "\t" + e["Version"])
for r in d.get("Replace") or []:
    print("replace\t" + r["Old"]["Path"] + "\t" + (r["Old"].get("Version") or "") + "\t" + r["New"]["Path"] + "\t" + (r["New"].get("Version") or ""))
for r in d.get("Retract") or []:
    print("retract\t" + (r.get("Low") or "") + "\t" + (r.get("High") or ""))
'
}
gated >"$tmp/before"
: >"$tmp/allowed"
for spec in $GO_SECURITY_OVERRIDES; do
  echo "${spec%@*}" >>"$tmp/allowed"
  echo "security override: go get $spec"
  (cd "$src" && go get "$spec")
done
sort -u -o "$tmp/allowed" "$tmp/allowed"
gated >"$tmp/after"

# --- the go/toolchain directive gate --------------------------------------
# A dependency carrying a security fix can require a newer LANGUAGE version,
# so taking the fix may force `go get` to raise the `go` line. The directive
# may rise, but never past the effective toolchain (PIN GO_TOOLCHAIN), which
# is what keeps the allowance honest. The `toolchain` line may only be dropped
# or lowered: GOTOOLCHAIN=local / the pinned toolchain selects the release, so
# a higher line would be a build on something other than the gated toolchain.
governum() {
  [[ ${1#go} =~ ^([0-9]+)\.([0-9]+)(\.([0-9]+))?$ ]] || return 1
  printf '%d%03d%03d\n' "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}" "${BASH_REMATCH[4]:-0}"
}
govlt() { local a b; a=$(governum "$1") && b=$(governum "$2") && ((a < b)); }
govle() { local a b; a=$(governum "$1") && b=$(governum "$2") && ((a <= b)); }
directive() { awk -F'\t' -v key="$1" '$1 == key {print $2}' "$2"; }
go_before=$(directive go "$tmp/before")
go_after=$(directive go "$tmp/after")
tc_before=$(directive toolchain "$tmp/before")
tc_after=$(directive toolchain "$tmp/after")
directives_ok=1
if [[ $go_after != "$go_before" ]]; then
  if govlt "$go_before" "$go_after" && govle "$go_after" "$effective"; then
    echo "go directive raised: ${go_before:-(none)} -> ${go_after:-(none)} (at or below the effective go$effective)"
  else
    echo "refused go directive change: ${go_before:-(none)} -> ${go_after:-(none)} (must rise, and not past the effective go$effective)" >&2
    directives_ok=0
  fi
fi
if [[ $tc_after != "$tc_before" ]]; then
  if [[ -z $tc_after ]] || govle "$tc_after" "$effective"; then
    echo "toolchain directive changed: ${tc_before:-(none)} -> ${tc_after:-(none)} (pinned toolchain go$effective still selects the release)"
  else
    echo "refused toolchain directive change: ${tc_before:-(none)} -> ${tc_after:-(none)} (above the effective go$effective)" >&2
    directives_ok=0
  fi
fi

{ diff "$tmp/before" "$tmp/after" || true; } | grep -E '^[<>] ' >"$tmp/diff" || true
if [[ ! -s $tmp/diff ]]; then
  echo "GO_SECURITY_OVERRIDES: already satisfied by the pinned go.mod — retire the override"
  exit 0
fi
refused=0
while IFS=$'\t' read -r field path _; do
  [[ -n $field ]] || continue
  skip=0
  case $field in
  require) if grep -qxF "$path" "$tmp/allowed"; then skip=1; fi ;;
  go | toolchain) skip=$directives_ok ;;
  esac
  if ((skip)); then continue; fi
  echo "  refused go.mod change: $field ${path:-(empty)}" >&2
  refused=1
done < <(sed 's/^[<>] //' "$tmp/diff")
((refused == 0)) || { echo "GO_SECURITY_OVERRIDES: go.mod changed outside the require lines of $GO_SECURITY_OVERRIDES" >&2; exit 1; }
echo "GO_SECURITY_OVERRIDES: applied; go.mod changed only in the declared require lines:"
sed 's/^/  /' "$tmp/diff"
