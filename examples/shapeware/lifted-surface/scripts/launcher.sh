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
# Every go command runs with GOWORK=off.
# A surface adds its own serve, status --json, request METHOD PATH [BODY]
# and stop verbs: they start the app on a fresh data dir and SQLite file
# under the scratch HOME, seeded with synthetic data only, on a free
# loopback port. ClassGo's reports-weekly-summary preview launcher is the
# worked example of those four verbs.
set -u
impl=${LAUNCHER_IMPL:?LAUNCHER_IMPL is not set (run the build step)}

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
    run_dir=$root target=$pkg mode=-mod=readonly
    ;;
  esac
  # -v so a run that matched no test ("no tests to run", exit 0) is a FAIL:
  # the named test itself must report "--- PASS: NAME".
  log=$(cd "$run_dir" && go test $mode -count=1 -v -run "^${name}\$" "$target" 2>&1)
  rc=$?
  # Exactly one "--- PASS: NAME" line: the one named test ran, and passed.
  if [ $rc -eq 0 ] && [ "$(printf '%s\n' "$log" | grep -c "^--- PASS: ${name} ")" -eq 1 ]; then
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
