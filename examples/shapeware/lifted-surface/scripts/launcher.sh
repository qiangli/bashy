#!/bin/sh
# Launcher snippet for a Go-backed lifted surface (shapeware spec §4.3b, §6).
# The surface's dag.md build step writes a wrapper into the bin folder that
# sets LAUNCHER_IMPL to the implementation it built and execs this file.
#
# This snippet carries one verb, the same in every surface:
#   gotest PKG NAME   run the pinned go test NAME in package PKG of the
#                     implementation; print exactly "PASS NAME" (exit 0) or
#                     "FAIL NAME" (exit 1). Nothing else reaches stdout, so
#                     the judge's comparison is stable.
# PKG is one of (spec §6):
#   . or ./path       a package of the implementation. ./path may lie in a
#                     nested module (a subdirectory with its own go.mod): the
#                     test runs from the nearest directory at or above ./path
#                     that holds a go.mod, never above the implementation
#                     root, as "go test ./<path below that module root>".
#   an import path    a package of a dependency whose module the
#                     implementation's go.mod requires, e.g.
#                     github.com/qiangli/yoke/pkg/chat. The test runs from the
#                     implementation root with -mod=readonly, so the module
#                     graph is the implementation's own: the version tested
#                     is the pinned one, and go.mod and go.sum are never
#                     rewritten. A test whose package or test-only imports
#                     are not in the implementation's go.sum, or that writes
#                     beside its sources (the module cache is read-only),
#                     fails here; cite a test in the implementation instead.
# Every go command runs with GOWORK=off, and LAUNCHER_IMPL is not passed to
# the test. PASS needs all of: "go test -list '^NAME$'" lists exactly NAME;
# the -json run exits 0 with exactly one "pass" event for NAME and no
# "fail" event; go.mod and go.sum (run module and implementation root) are
# unchanged. A package's own TestMain is trusted (spec §6): it runs first,
# in the test binary, and one written to forge the -list and -json output
# (test2json reads a printed "--- PASS: NAME" line as a pass event) can
# still make a test that never ran print PASS.
# A surface adds its own serve, status --json, request METHOD PATH [BODY]
# and stop verbs: they start the app on a fresh data dir and SQLite file
# under the scratch HOME, seeded with synthetic data only, on a free
# loopback port. ClassGo's reports-weekly-summary preview launcher is the
# worked example of those four verbs.
set -u
impl=${LAUNCHER_IMPL:?LAUNCHER_IMPL is not set (run the build step)}
# No cited test needs LAUNCHER_IMPL: it is the launcher's input only, and a
# test must not learn or rewrite the implementation's location through it.
unset LAUNCHER_IMPL

# requires GOMOD PKG: GOMOD has a require line (single or in a block) for
# the module PKG lies in: PKG is that module path or below it. A replace
# line alone does not count, nor does a lookalike prefix.
requires() {
  awk -v pkg="$2" '
    { sub(/\/\/.*/, "") }
    blk && $1 == ")" { blk = 0; next }
    blk && NF { m = $1 }
    !blk && $1 == "require" && $2 == "(" { blk = 1; next }
    !blk && $1 == "require" && NF >= 3 { m = $2 }
    m != "" {
      gsub(/["`]/, "", m)
      if (pkg == m || index(pkg, m "/") == 1) { found = 1; exit }
      m = ""
    }
    END { exit !found }' "$1"
}
# sums DIR...: a fingerprint of go.mod and go.sum in each DIR ("-" if missing).
sums() {
  for d in "$@"; do
    for f in "$d/go.mod" "$d/go.sum"; do
      if [ -f "$f" ]; then cksum <"$f"; else echo -; fi
    done
  done
}

case ${1:-} in
gotest)
  [ $# -eq 3 ] || { echo "usage: gotest PKG NAME" >&2; exit 2; }
  pkg=$2 name=$3
  # NAME is one top-level test, ^Test[A-Za-z0-9_]+$: never a pattern such
  # as Test.* that -run would widen to many tests.
  case $name in
    Test?*) ;;
    *) echo "FAIL $name"; exit 1 ;;
  esac
  case ${name#Test} in *[!A-Za-z0-9_]*) echo "FAIL $name"; exit 1 ;; esac
  export GOWORK=off
  root=$(cd "$impl" 2>/dev/null && pwd -P) || { echo "FAIL $name"; exit 1; }
  case $pkg in
  . | ./*)
    # A clean ./path only: no "..", empty or "." segment, no pattern.
    rest=
    if [ "$pkg" != . ]; then
      rest=${pkg#./}
      case /$rest/ in
        // | */../* | */./* | *//* | *...* | *[!A-Za-z0-9._~+/-]*) echo "FAIL $name"; exit 1 ;;
      esac
    fi
    dir=$(cd "$root/$rest" 2>/dev/null && pwd -P) || { echo "FAIL $name"; exit 1; }
    case $dir/ in "$root"/*) ;; *) echo "FAIL $name"; exit 1 ;; esac
    # The nearest module root at or above the package, inside the implementation.
    mod=$dir
    while [ ! -f "$mod/go.mod" ] && [ "$mod" != "$root" ]; do mod=${mod%/*}; done
    [ -f "$mod/go.mod" ] || { echo "FAIL $name"; exit 1; }
    rel=${dir#"$mod"}
    run_dir=$mod target=.$rel mode=
    ;;
  *)
    # A dependency import path, from the implementation's own module graph:
    # clean segments of letters, digits and -._~+, no "..." pattern, and a
    # first element with a dot that does not start with one (not a
    # standard-library package, not a flag, not an absolute path).
    case /$pkg/ in
      // | */../* | */./* | *//* | *...* | *[!A-Za-z0-9._~+/-]*) echo "FAIL $name"; exit 1 ;;
    esac
    first=${pkg%%/*}
    case $first in
      .* | -*) echo "FAIL $name"; exit 1 ;;
      *.*) ;;
      *) echo "FAIL $name"; exit 1 ;;
    esac
    # The implementation's own packages are cited as ./ paths.
    self=$(awk '$1 == "module" { gsub(/"/, "", $2); print $2; exit }' "$root/go.mod" 2>/dev/null)
    case $pkg in "$self" | "$self"/*) [ -n "$self" ] && { echo "FAIL $name"; exit 1; } ;; esac
    requires "$root/go.mod" "$pkg" || { echo "FAIL $name"; exit 1; }
    run_dir=$root target=$pkg mode=-mod=readonly
    ;;
  esac
  # go.mod and go.sum, in the module the test runs from and in the
  # implementation root, are fingerprinted before -list and after the run.
  before=$(sums "$run_dir" "$root")
  # 1. -list: the package lists NAME and nothing else (besides go's own
  # "ok" line), so a TestMain that prints its own text here fails.
  listed=$(cd "$run_dir" && go test $mode -count=1 -list "^${name}\$" "$target" 2>/dev/null) || { echo "FAIL $name"; exit 1; }
  [ "$(printf '%s\n' "$listed" | grep -v '^ok ')" = "$name" ] || { echo "FAIL $name"; exit 1; }
  # 2. Run it with -json.
  log=$(cd "$run_dir" && go test $mode -count=1 -json -run "^${name}\$" "$target" 2>/dev/null)
  rc=$?
  after=$(sums "$run_dir" "$root")
  # A test that wrote go.mod or go.sum fails; nothing is restored, so the
  # change stays visible to whoever reruns it.
  [ "$before" = "$after" ] || { echo "FAIL $name"; exit 1; }
  # Exactly one test2json "pass" event for NAME itself (not a subtest; a
  # printed line is an "output" event, its quotes escaped), and no "fail"
  # event at all.
  passes=$(printf '%s\n' "$log" | grep -F '"Action":"pass"' | grep -cE "\"Test\":\"${name}\"[,}]")
  fails=$(printf '%s\n' "$log" | grep -cF '"Action":"fail"')
  # test2json also turns a plain printed "--- PASS: NAME" line into a pass
  # event, so a run where the testing package found nothing to run fails too.
  none=$(printf '%s\n' "$log" | grep -c 'no tests to run')
  if [ $rc -eq 0 ] && [ "$passes" -eq 1 ] && [ "$fails" -eq 0 ] && [ "$none" -eq 0 ]; then
    echo "PASS $name"
    exit 0
  fi
  # The go test log is not printed: the judge reads stdout and stderr
  # together. Rerun the go test command by hand to see why it failed.
  echo "FAIL $name"
  exit 1
  ;;
*)
  echo "usage: launcher gotest PKG NAME" >&2
  exit 2
  ;;
esac
