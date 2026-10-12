#!/usr/bin/env bash
# Applies the security-only pnpm override for GHSA-ch52-4w7c-c8xp
# (http-cache-semantics >=4.3.0) to the pinned OpenClaw checkout.
#
# Runs in CI BEFORE install. Writes the override into package.json
# (pnpm.overrides), preserving reproducibility: the lockfile is regenerated
# afterwards and assert-lockfile-diff.sh verifies the regeneration touched
# only http-cache-semantics entries.
#
# Env:
#   OPENCLAW_DIR    pinned checkout (default: current directory)
#   OVERRIDE_SPEC   override version range (default: >=4.3.0)
set -euo pipefail

OPENCLAW_DIR="${OPENCLAW_DIR:-$PWD}"
OVERRIDE_SPEC="${OVERRIDE_SPEC:->=4.3.0}"
export OVERRIDE_SPEC
PKG="http-cache-semantics"

cd "$OPENCLAW_DIR"
test -f package.json || { echo "apply-override: no package.json in $OPENCLAW_DIR" >&2; exit 1; }

node -e "
const fs = require('node:fs');
const pkg = JSON.parse(fs.readFileSync('package.json', 'utf8'));
pkg.pnpm ??= {};
pkg.pnpm.overrides ??= {};
pkg.pnpm.overrides['$PKG'] = process.env.OVERRIDE_SPEC;
fs.writeFileSync('package.json', JSON.stringify(pkg, null, 2) + '\n');
console.log('override recorded: $PKG = ' + process.env.OVERRIDE_SPEC);
"

echo "apply-override: done ($PKG = $OVERRIDE_SPEC)"
