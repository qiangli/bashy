#!/usr/bin/env bash
# Keep the INSTALLED bashy current with the live sibling checkouts, so every
# weave/delegate launch and the fixer/gate machinery use the latest bashy WHILE
# agents are building new bashy features. This is the steward's standing duty: a
# stale PATH bashy silently degrades the fleet (a July-14 install once broke
# CI-repair fixer selection because it predated the `kind` field).
#
# Idempotent + SAFE: rebuilds, runs the fast gate, and installs ONLY when the live
# checkout advanced AND the gate passes — a broken checkout never gets installed.
# Safe to run on a timer or after every merge. `--force` rebuilds regardless.
set -euo pipefail

repo="$(cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$repo"

# Key over BOTH bashy and its coreutils sibling: a coreutils change (e.g. pkg/bre,
# the fleet catalog) changes the linked binary even when bashy's own HEAD is unchanged.
bashy_head="$(git rev-parse HEAD 2>/dev/null || echo unknown)"
core_head="$(git -C ../coreutils rev-parse HEAD 2>/dev/null || echo unknown)"
key="${bashy_head}:${core_head}"
stamp="${HOME}/.bashy/installed-key"
mkdir -p "$(dirname "$stamp")"

expected_build="$(git describe --tags --exact-match HEAD 2>/dev/null || git rev-parse --short=7 HEAD 2>/dev/null || true)"
if [[ -n "$expected_build" ]] &&
   { ! git diff --quiet --ignore-submodules -- 2>/dev/null ||
     ! git diff --cached --quiet --ignore-submodules -- 2>/dev/null; }; then
	expected_build="${expected_build}-dirty"
fi

active_is_current() {
	local active version
	active="${DHNT_BIN_DIR:-$HOME/.local/bin}/bashy"
	[[ -n "$active" && -x "$active" ]] || return 1
	version="$("$active" --version 2>/dev/null || true)"
	[[ -z "$expected_build" || "$version" == *"$expected_build"* ]] || return 1
	"$active" judge --help >/dev/null 2>&1 || return 1
	"$active" agents --help >/dev/null 2>&1 || return 1
}

if [[ "${1:-}" != "--force" && -f "$stamp" && "$(cat "$stamp" 2>/dev/null)" == "$key" ]]; then
	if active_is_current; then
		echo "refresh-bashy: already current (bashy ${bashy_head:0:8} / coreutils ${core_head:0:8}) — nothing to do"
		exit 0
	fi
	echo "refresh-bashy: stamp matched but active PATH binary is stale or incomplete — repairing"
fi

echo "refresh-bashy: building (bashy ${bashy_head:0:8} / coreutils ${core_head:0:8})…"
GOTOOLCHAIN=auto make build

echo "refresh-bashy: gate — go test ./internal/agentos (dispatch/atlas coverage)…"
if ! GOTOOLCHAIN=auto go test ./internal/agentos >/dev/null 2>&1; then
	echo "refresh-bashy: GATE FAILED — NOT installing; the live checkout is broken, leaving the last-good binary in place." >&2
	exit 1
fi

GOTOOLCHAIN=auto go run ./tools/installbashy -bash bin/bash -bashy bin/bashy
if ! active_is_current; then
	echo "refresh-bashy: INSTALL VERIFY FAILED — active PATH bashy is not the build just installed." >&2
	exit 1
fi
printf '%s\n' "$key" >"$stamp"
echo "refresh-bashy: installed + stamped (bashy ${bashy_head:0:8} / coreutils ${core_head:0:8})"
