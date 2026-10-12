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

# pnpm >= 10 reads overrides from pnpm-workspace.yaml (the upstream repo is a
# pnpm 12 workspace); a package.json "pnpm.overrides" entry is silently ignored.
node -e "
const fs = require('node:fs');
const f = 'pnpm-workspace.yaml';
const spec = process.env.OVERRIDE_SPEC;
let s = fs.existsSync(f) ? fs.readFileSync(f, 'utf8') : '';
const entry = '  $PKG: \\'' + spec + '\\'';
if (new RegExp('^  $PKG:', 'm').test(s)) {
  s = s.replace(new RegExp('^  $PKG:.*$', 'm'), entry);
} else if (/^overrides:\\s*$/m.test(s)) {
  s = s.replace(/^overrides:\\s*$/m, 'overrides:\\n' + entry);
} else {
  s = s.replace(/\\n?$/, '\\n') + 'overrides:\\n' + entry + '\\n';
}
fs.writeFileSync(f, s);
console.log('override recorded in ' + f + ': $PKG = ' + spec);
"

echo "apply-override: done ($PKG = $OVERRIDE_SPEC)"
