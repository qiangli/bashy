#!/usr/bin/env bash
# Applies the security-only pnpm override for GHSA-ch52-4w7c-c8xp
# (http-cache-semantics >=4.3.0) to the pinned OpenClaw checkout.
#
# Runs in CI BEFORE install. Writes the override into pnpm-workspace.yaml
# (`overrides:`), preserving reproducibility: the lockfile is regenerated
# afterwards and assert-lockfile-diff.sh verifies the regeneration touched
# only http-cache-semantics entries.
#
# SCOPED, not global, by operator decision. A global
# `http-cache-semantics: '>=4.3.0'` entry makes upstream's own
# scripts/generate-npm-package-lock.mts validation fail the pack step:
# its broad rule flags npm's BUNDLED copy, which no override can move
# ("generated package-lock.json violates workspace overrides:
# node_modules/npm/node_modules/http-cache-semantics locked 4.2.0,
# expected >=4.3.0"; npm 12.x bundles its own http-cache-semantics 4.2.0
# and bundled deps ignore overrides). The pnpm parent>child selector below
# pins the resolvable dev path instead. Upstream's checker understands the
# scoped form (verified against v2026.10.2 source:
# scripts/generate-npm-package-lock.mts parses parent>child via
# @pnpm/parse-overrides `parsePkgAndParentSelector`, drops object-valued
# entries from the broad rules in `validationOverrideRulesFromOverrides`,
# and enforces the child rule only along edges beneath a matched parent in
# `matchOverridePolicy`, so the immutable bundled copy is no longer
# charged against the rule). The shipped bundled copy is fixed separately
# AFTER packing by scripts/patch-bundled.mjs, which also updates the SBOM.
#
# Env:
#   OPENCLAW_DIR    pinned checkout (default: current directory)
#   OVERRIDE_SPEC   override version range (default: >=4.3.0)
set -euo pipefail

OPENCLAW_DIR="${OPENCLAW_DIR:-$PWD}"
OVERRIDE_SPEC="${OVERRIDE_SPEC:->=4.3.0}"
export OVERRIDE_SPEC
# pnpm parent>child selector: only http-cache-semantics resolved THROUGH
# make-fetch-happen (the dev-graph path, incl. make-fetch-happen@16.0.1) is
# pinned. npm's pre-bundled copy is out of scope here; see patch-bundled.mjs.
PARENT="make-fetch-happen"
CHILD="http-cache-semantics"
export PARENT CHILD

cd "$OPENCLAW_DIR"
test -f package.json || { echo "apply-override: no package.json in $OPENCLAW_DIR" >&2; exit 1; }

# pnpm >= 10 reads overrides from pnpm-workspace.yaml (the upstream repo is a
# pnpm 12 workspace); a package.json "pnpm.overrides" entry is silently ignored.
node -e "
const fs = require('node:fs');
const f = 'pnpm-workspace.yaml';
const parent = process.env.PARENT;
const child = process.env.CHILD;
const spec = process.env.OVERRIDE_SPEC;
const key = parent + '>' + child;
// Quote the key: '>' mid-scalar is legal YAML but quoting keeps every
// parser (and our own re-run match below) unambiguous.
const entry = '  \\'' + key + '\\': \\'' + spec + '\\'';
let s = fs.existsSync(f) ? fs.readFileSync(f, 'utf8') : '';
const bareRe = new RegExp('^  \\'?' + parent.replace(/[.*+?^\${}()|[\]\\\\]/g, '\\\\$&') + '>' + child.replace(/[.*+?^\${}()|[\]\\\\]/g, '\\\\$&') + '\\'?:.*$', 'm');
if (bareRe.test(s)) {
  s = s.replace(bareRe, entry);
} else if (/^overrides:\s*$/m.test(s)) {
  s = s.replace(/^overrides:\s*$/m, 'overrides:\n' + entry);
} else {
  s = s.replace(/\n?$/, '\n') + 'overrides:\n' + entry + '\n';
}
fs.writeFileSync(f, s);
console.log('override recorded in ' + f + ': ' + key + ' = ' + spec);
"

echo "apply-override: done ($PARENT>$CHILD = $OVERRIDE_SPEC)"
