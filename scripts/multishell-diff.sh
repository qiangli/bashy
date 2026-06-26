#!/usr/bin/env bash
# multishell-diff.sh — broadened differential POSIX-conformance harness.
#
# Runs the corpus through bashy + an EXPANDED oracle panel, across two images
# (one strict userland each), to catch divergences a single reference (bash)
# misses before the formal VSC-PCTS run. It drives scripts/posix-diff.sh twice
# via its ORACLE_SPEC/SHELLS_DOCKERFILE env hooks:
#
#   PANEL 1 — Alpine bash:5.3   bash53 · dash · ash · yash · mksh · loksh · zsh
#   PANEL 2 — Debian strict     posh · ksh93 · dash · ash · bash52 · mksh · zsh
#
# Why two: the exact bash 5.3 release + yash live only on Alpine (apk); posh —
# the deliberately strict shell that REJECTS bashisms — and ksh93 (the
# feature-rich reference KornShell) live only on Debian (apt). Together the
# panel spans the strict-POSIX shells (dash, ash, posh, yash) and the
# feature-rich/dual-mode shells (bash, zsh, ksh93, mksh, loksh).
#
# Classification is posix-diff.sh's: DEVIATION = bashy disagrees where ALL
# oracles agree (a high-confidence bug); AMBIGUOUS = oracles disagree among
# themselves (bash-extension / unspecified — annotated with who bashy matches).
#
# Usage: scripts/multishell-diff.sh [corpus-dir]   (run from the bashy repo root)
# Exit 0 iff BOTH panels report zero DEVIATIONs.
set -u
HERE=$(cd "$(dirname "$0")/.." && pwd)
CORPUS=${1:-test/posix-corpus}
cd "$HERE" || exit 2

echo "############################################################"
echo "# PANEL 1 — Alpine bash:5.3 broad"
echo "#   bash53 dash ash yash mksh loksh zsh"
echo "############################################################"
# Note: Alpine's `loksh` package installs its binary as /bin/ksh (not `loksh`),
# so the panel invokes it as `ksh` — distinct from mksh's /bin/mksh.
POSIX_SHELLS_IMAGE=localhost/posix-shells-broad \
SHELLS_DOCKERFILE=$'FROM bash:5.3\nRUN apk add --no-cache dash yash zsh mksh loksh\n' \
ORACLE_SPEC='bash53:bash --posix|dash:dash|ash:busybox ash|yash:yash --posix|mksh:mksh|loksh:ksh|zsh:zsh --emulate sh' \
  bash "$HERE/scripts/posix-diff.sh" "$CORPUS"
rc1=$?

echo
echo "############################################################"
echo "# PANEL 2 — Debian strict (posh + ksh93, which Alpine lacks)"
echo "#   dash ash posh bash52 ksh93 mksh zsh"
echo "############################################################"
POSIX_SHELLS_IMAGE=localhost/posix-shells-deb \
SHELLS_DOCKERFILE=$'FROM debian:stable-slim\nRUN apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq posh ksh dash zsh mksh busybox >/dev/null 2>&1\n' \
ORACLE_SPEC='dash:dash|ash:busybox ash|posh:posh|bash52:bash --posix|ksh93:ksh|mksh:mksh|zsh:zsh --emulate sh' \
  bash "$HERE/scripts/posix-diff.sh" "$CORPUS"
rc2=$?

echo
echo "============================================================"
echo "= multishell-diff summary: panel1(alpine) rc=$rc1 · panel2(debian) rc=$rc2"
echo "=   (rc 0 = zero DEVIATIONs on that panel)"
echo "============================================================"
[ "$rc1" -eq 0 ] && [ "$rc2" -eq 0 ]
