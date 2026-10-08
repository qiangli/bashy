#!/bin/sh
# List and verify the deb, rpm and apk emitted from the one release build.
set -eu

dist=${1:-dist}
expected='usr/lib/bashy/bin/bashy usr/lib/bashy/bin/bash usr/lib/bashy/bin/sh usr/lib/bashy/bin/outpost usr/bin/bashy usr/bin/outpost'
# Packages must never own the distribution's shells.
forbidden='usr/bin/bash usr/bin/sh bin/bash bin/sh'

check_contents() {
  package=$1
  listing=$2
  printf '\n== %s ==\n' "$package"
  printf '%s\n' "$listing"
  for path in $expected; do
    printf '%s\n' "$listing" | grep -Eq "(^|[./])${path}([[:space:]]|$)" || {
      echo "$package: missing /$path" >&2
      exit 1
    }
  done
  for path in $forbidden; do
    if printf '%s\n' "$listing" | grep -Eq "(^|[[:space:]]|\./|/)${path}([[:space:]]|$)"; then
      echo "$package: must not install /$path" >&2
      exit 1
    fi
  done
}

found=0
for package in "$dist"/*.deb; do
  [ -f "$package" ] || continue
  found=$((found + 1))
  check_contents "$package" "$(dpkg-deb --contents "$package")"
done
for package in "$dist"/*.rpm; do
  [ -f "$package" ] || continue
  found=$((found + 1))
  check_contents "$package" "$(rpm -qlp "$package")"
done
for package in "$dist"/*.apk; do
  [ -f "$package" ] || continue
  found=$((found + 1))
  check_contents "$package" "$(tar -tzf "$package")"
done

[ "$found" -eq 6 ] || {
  echo "expected 6 Linux packages (deb/rpm/apk x amd64/arm64), found $found" >&2
  exit 1
}
