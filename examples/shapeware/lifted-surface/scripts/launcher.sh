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
  case $name in
    Test*) ;;
    *) echo "FAIL $name"; exit 1 ;;
  esac
  # -v so a run that matched no test ("no tests to run", exit 0) is a FAIL:
  # the named test itself must report "--- PASS: NAME".
  log=$(cd "$impl" && go test -count=1 -v -run "^${name}\$" "$pkg" 2>&1)
  rc=$?
  if [ $rc -eq 0 ] && printf '%s\n' "$log" | grep -q "^--- PASS: ${name} "; then
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
