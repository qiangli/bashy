#!/usr/bin/env bash
# Named test groups over `go list ./...` (manifest: scripts/test-groups.txt).
#
#   test-groups.sh --list                 group -> packages
#   test-groups.sh --check                fail-closed: every root package (union
#                                         over linux/darwin/windows) has exactly
#                                         one home group; no unknown entries
#   test-groups.sh [--dry-run] run G...   run groups as parallel local lanes
#
# A manifest entry whose directory holds its own go.mod is a nested module: it
# is tested with `go -C <dir> test ./...` and the check requires that module to
# list packages, so packages may leave the root `go list ./...` when they move
# into their own module without failing the check.
#
# Cache: core/agentos/frontend/tooling/examples run without -count=1, so Go's
# content-keyed test cache reuses unchanged results; e2e always runs -count=1.
# Each group writes <out>/<group>.log and <out>/<group>.tsv with one row per
# package: group, package, ok|FAIL|notests, seconds, reuse|fresh.
#
# Environment:
#   BASHY_TEST_GROUP_TIMEOUT  go test -timeout per test binary (default 45m)
#   BASHY_TEST_GROUP_OUT      result directory (default bin/test-groups)
#   BASHY_TEST_GROUP_SERIAL=1 run the named groups one after another
#   BASHY_TEST_GROUPS_FILE    manifest override (tests of this script)
set -euo pipefail

repo=$(CDPATH='' cd -P "$(dirname "$0")/.." && pwd)
cd "$repo"
export GOWORK=${GOWORK:-off}
manifest=${BASHY_TEST_GROUPS_FILE:-$repo/scripts/test-groups.txt}
known_groups='core agentos frontend tooling examples e2e'
timeout=${BASHY_TEST_GROUP_TIMEOUT:-45m}
out=${BASHY_TEST_GROUP_OUT:-$repo/bin/test-groups}
dry=0

die() { echo "test-groups: $*" >&2; exit 2; }
usage() { sed -n '2,25s/^# \{0,1\}//p' "$0"; }

[ -f "$manifest" ] || die "missing manifest $manifest"
modpath=$(go list -m)

# entries: group <TAB> ./pkg <TAB> tests-or-dash (comments and blanks dropped).
entries() {
  awk 'NF && $1 !~ /^#/ { if (NF > 3) { print "test-groups: too many fields: " $0 > "/dev/stderr"; exit 2 }
         printf "%s\t%s\t%s\n", $1, $2, (NF == 3 ? $3 : "-") }' "$manifest"
}
is_module() { [ "$1" != . ] && [ -f "$1/go.mod" ]; }
# root packages as ./rel, for the current GOOS or the union over several.
root_pkgs() {
  local os
  for os in "$@"; do GOOS=$os go list ./...; done |
    sed "s|^$modpath\$|.|; s|^$modpath/|./|" | sort -u
}

check() {
  local tmp rc=0 g p t name
  tmp=$(mktemp -d)
  entries >"$tmp/entries"
  root_pkgs linux darwin windows >"$tmp/universe"
  while IFS=$'\t' read -r g p t; do
    case " $known_groups " in *" $g "*) ;; *) echo "unknown group: $g ($p)"; rc=1 ;; esac
    case "$p" in ./*|.) ;; *) echo "package must be ./relative: $p"; rc=1 ;; esac
    if [ "$t" != - ]; then
      [ "$g" = e2e ] || { echo "test list outside e2e: $g $p"; rc=1; }
      is_module "$p" && { echo "test list on a nested module: $p"; rc=1; }
      for name in ${t//|/ }; do
        grep -qs "^func $name(" "$p"/*_test.go || { echo "unknown test: $p $name"; rc=1; }
      done
    elif is_module "$p"; then
      [ -n "$(go -C "$p" list ./... 2>/dev/null)" ] || { echo "nested module lists no packages: $p"; rc=1; }
      echo "$p" >>"$tmp/modules"
    else
      grep -qxF "$p" "$tmp/universe" || { echo "unknown package: $p"; rc=1; }
    fi
  done <"$tmp/entries"
  awk -F'\t' '$3 == "-" { print $2 }' "$tmp/entries" | sort >"$tmp/homes"
  uniq -d "$tmp/homes" | sed 's/^/package in two groups: /' | grep . && rc=1
  comm -23 "$tmp/universe" <(sort -u "$tmp/homes") | sed 's/^/missing from every group: /' | grep . && rc=1
  # A split-out test list needs a home group that runs the rest of the package.
  awk -F'\t' '$3 != "-" { print $2 }' "$tmp/entries" | sort -u |
    comm -23 - <(awk -F'\t' '$3 == "-" && $1 != "e2e" { print $2 }' "$tmp/entries" | sort -u) |
    sed 's/^/e2e test list without a non-e2e home: /' | grep . && rc=1
  local roots mods=0
  roots=$(wc -l <"$tmp/universe" | tr -d ' ')
  [ -f "$tmp/modules" ] && mods=$(wc -l <"$tmp/modules" | tr -d ' ')
  rm -rf "$tmp"
  if [ "$rc" -ne 0 ]; then echo 'test-groups: --check FAIL' >&2; return 1; fi
  echo "test-groups: --check PASS ($roots root packages, $mods nested modules)"
}

list() {
  local g
  for g in $known_groups; do
    entries | awk -F'\t' -v g="$g" '$1 == g { n++; l = l "  " $2 ($3 == "-" ? "" : "  -run ^(" $3 ")$") "\n" }
      END { printf "%s (%d)\n%s", g, n, l }'
  done
}

# invoke <group> <dir-or-dot> <go test args...>: run (or print) one go test and
# append its per-package rows to <group>.tsv.
invoke() {
  local g=$1 dir=$2; shift 2
  local -a cmd=(go)
  [ "$dir" = . ] || cmd+=(-C "$dir")
  cmd+=(test -timeout "$timeout" "$@")
  if [ "$dry" = 1 ]; then
    if [ "$dir" = . ]; then printf '%s:' "$g"; else printf '%s: GOTOOLCHAIN=%s' "$g" "$root_tc"; fi
    printf ' %q' "${cmd[@]}"; echo; return 0
  fi
  local rc=0
  if [ "$dir" = . ]; then "${cmd[@]}" >"$out/$g.part" 2>&1 || rc=$?
  else GOTOOLCHAIN=$root_tc "${cmd[@]}" >"$out/$g.part" 2>&1 || rc=$?; fi
  cat "$out/$g.part" >>"$out/$g.log"
  awk -F'\t' -v g="$g" -v OFS='\t' '
    $1 ~ /^ok/   { r = ($3 ~ /cached/) ? "reuse" : "fresh"; s = ($3 ~ /cached/) ? 0 : $3; sub(/s$/, "", s); print g, $2, "ok", s, r }
    $1 ~ /^FAIL/ && NF >= 2 { p = $2; sub(/ .*/, "", p); s = $3; sub(/s$/, "", s); print g, p, "FAIL", (s == "" ? 0 : s), "fresh" }
    $1 ~ /^\?/   { print g, $2, "notests", 0, "-" }' "$out/$g.part" >>"$out/$g.tsv"
  rm -f "$out/$g.part"
  return "$rc"
}

run_group() {
  local g=$1 p t rc=0
  local -a plain=() count=()
  [ "$g" = e2e ] && count=(-count=1)
  local here log=; here=$(root_pkgs "$(go env GOOS)")
  [ "$dry" = 1 ] || log=$out/$g.log
  if [ "$dry" = 0 ]; then : >"$out/$g.log"; printf 'group\tpackage\tstatus\tseconds\tlabel\n' >"$out/$g.tsv"; fi
  while IFS=$'\t' read -r p t; do
    [ "$t" = - ] || continue
    if is_module "$p"; then invoke "$g" "$p" ${count[@]+"${count[@]}"} ./... || rc=1; continue; fi
    grep -qxF "$p" <<<"$here" || { echo "$g: skip $p (not built for $(go env GOOS))" >>"${log:-/dev/stderr}"; continue; }
    # A package with tests split out to e2e runs alone with the matching -skip.
    t=$(entries | awk -F'\t' -v p="$p" '$1 == "e2e" && $2 == p && $3 != "-" { print $3 }')
    if [ -n "$t" ] && [ "$g" != e2e ]; then
      invoke "$g" . ${count[@]+"${count[@]}"} -skip "^(${t//$'\n'/|})\$" "$p" || rc=1
    else
      plain+=("$p")
    fi
  done < <(entries | awk -F'\t' -v g="$g" -v OFS='\t' '$1 == g { print $2, $3 }')
  if [ "${#plain[@]}" -gt 0 ]; then invoke "$g" . ${count[@]+"${count[@]}"} "${plain[@]}" || rc=1; fi
  if [ "$g" = e2e ]; then
    while IFS=$'\t' read -r p t; do
      grep -qxF "$p" <<<"$here" || continue
      invoke "$g" . -count=1 -run "^($t)\$" "$p" || rc=1
    done < <(entries | awk -F'\t' -v OFS='\t' '$1 == "e2e" && $3 != "-" { print $2, $3 }')
  fi
  return "$rc"
}

summary() {
  local g
  for g in "$@"; do
    awk -F'\t' -v g="$g" 'NR > 1 { n++; c[$3]++; if ($5 == "reuse") r++; if ($5 == "fresh") f++ }
      END { printf "%-9s %3d pkgs  ok=%d FAIL=%d notests=%d  reuse=%d fresh=%d\n", g, n, c["ok"], c["FAIL"], c["notests"], r, f }' "$out/$g.tsv"
  done
}

run() {
  local g rc=0
  [ "$#" -gt 0 ] || die 'run: name at least one group'
  for g in "$@"; do case " $known_groups " in *" $g "*) ;; *) die "unknown group: $g" ;; esac; done
  check >&2 || exit 1
  root_tc=$(awk '$1 == "toolchain" { print $2 }' go.mod); [ -n "$root_tc" ] || root_tc=$(go env GOVERSION)
  if [ "$dry" = 1 ]; then for g in "$@"; do run_group "$g"; done; return 0; fi
  mkdir -p "$out"
  local -a pids=()
  for g in "$@"; do
    if [ "${BASHY_TEST_GROUP_SERIAL:-0}" = 1 ]; then run_group "$g" || rc=1
    else run_group "$g" & pids+=($!); fi
  done
  for g in ${pids[@]+"${pids[@]}"}; do wait "$g" || rc=1; done
  summary "$@"
  echo "test-groups: results in $out"
  return "$rc"
}

case "${1:-}" in
  --list) list ;;
  --check) check ;;
  --dry-run) shift; [ "${1:-}" = run ] || die '--dry-run applies to run'; shift; dry=1; run "$@" ;;
  run) shift; run "$@" ;;
  -h|--help) usage ;;
  *) usage >&2; exit 2 ;;
esac
