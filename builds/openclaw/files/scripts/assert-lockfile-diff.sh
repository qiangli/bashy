#!/usr/bin/env bash
# Asserts the post-override lockfile regeneration touched ONLY
# http-cache-semantics entries. Any other lockfile change fails the build:
# the override must not silently pull in unrelated dependency moves.
#
# Method (exact, not heuristic): split HEAD and working-tree pnpm-lock.yaml
# into per-snapshot blocks keyed by snapshot key; every block whose text
# differs must be keyed by http-cache-semantics. Importer specifier lines
# for any other package must also be unchanged.
#
# Runs in CI AFTER `pnpm install --lockfile-only`, BEFORE the frozen install.
# Env:
#   OPENCLAW_DIR  pinned checkout (default: current directory)
set -euo pipefail

OPENCLAW_DIR="${OPENCLAW_DIR:-$PWD}"
PKG="http-cache-semantics"

cd "$OPENCLAW_DIR"
test -f pnpm-lock.yaml || { echo "assert-lockfile-diff: no pnpm-lock.yaml in $OPENCLAW_DIR" >&2; exit 1; }

node -e "
const { execSync } = require('node:child_process');
const fs = require('node:fs');
const before = execSync('git show HEAD:pnpm-lock.yaml', { maxBuffer: 64 * 1024 * 1024 }).toString('utf8');
const after = fs.readFileSync('pnpm-lock.yaml', 'utf8');
if (before === after) {
  console.error('assert-lockfile-diff: FAIL — lockfile unchanged; override had no effect');
  process.exit(1);
}
// Snapshot blocks: lines at exactly 2-space indent ending in ':' that are
// not section headers (section headers have no indent; 'importers:' is the
// only 2-space non-snapshot key and is handled separately below).
function snapshotBlocks(text) {
  const map = new Map();
  // pnpm >= 10 lockfiles hold several YAML documents ('---'); the same
  // package key can appear in more than one, so every key is per-document.
  let doc = 0;
  let section = null;
  let key = null;
  for (const line of text.split('\n')) {
    if (line === '---') { doc++; section = null; key = null; continue; }
    const sec = line.match(/^(\S[^:]*):$/);
    if (sec) { section = sec[1]; key = null; continue; }
    if ((section === 'packages' || section === 'snapshots') && /^ {2}\S.*:$/.test(line)) {
      // packages: and snapshots: reuse the same package keys, so the section
      // is part of the key; otherwise a snapshots block overwrites the
      // packages block holding resolution/integrity and hides its changes.
      key = doc + '|' + section + '|' + line.trim();
      map.set(key, []);
    } else if (key !== null) {
      map.get(key).push(line);
    }
  }
  return map;
}
// Importer pins: (importer, package) -> 'specifier@version' text.
function importerPins(text) {
  const map = new Map();
  let doc = 0;
  let importer = null;
  let pkg = null;
  let inDeps = false;
  for (const line of text.split('\n')) {
    let m;
    if (line === '---') { doc++; importer = null; pkg = null; inDeps = false; continue; }
    if (/^ {2}\S.*:$/.test(line)) { importer = doc + '|' + line.trim().replace(/:$/, ''); pkg = null; inDeps = false; }
    else if (/^ {4}(dependencies|devDependencies|optionalDependencies):$/.test(line)) { inDeps = true; pkg = null; }
    else if (/^ {4}\S/.test(line)) { inDeps = false; pkg = null; }
    else if (inDeps && (m = line.match(/^ {6}(\S+):$/))) { pkg = m[1]; }
    else if (pkg !== null && (m = line.match(/^ {8}(specifier|version): (.*)$/))) {
      const k = importer + ' :: ' + pkg + ' :: ' + m[1];
      map.set(k, m[2]);
    } else if (/^ {2}\S/.test(line)) { importer = null; pkg = null; inDeps = false; }
  }
  return map;
}
const offenders = [];
// Multiset line difference: returns lines added/removed between two blocks.
function changedLines(blines, alines) {
  const count = new Map();
  for (const l of blines) count.set(l, (count.get(l) ?? 0) + 1);
  for (const l of alines) count.set(l, (count.get(l) ?? 0) - 1);
  const out = [];
  for (const [l, n] of count) for (let i = 0; i < Math.abs(n); i++) out.push(l);
  return out;
}
const sb = snapshotBlocks(before);
const sa = snapshotBlocks(after);
for (const [key, blines] of sb) {
  const alines = sa.get(key);
  if (alines && alines.join('\n') === blines.join('\n')) continue;
  if (key.includes('$PKG')) continue; // the overridden package's own entries
  if (!alines) { offenders.push('removed snapshot ' + key); continue; }
  // A dependent block (e.g. make-fetch-happen) may change only by moving its
  // http-cache-semantics pin; every changed line must name the package.
  // Registry metadata drift: a 'deprecated:' notice can appear or change on
  // regeneration without any version/integrity move. Allow exactly that line
  // shape; every other changed line must name the overridden package.
  const bad = changedLines(blines, alines).filter((l) => !l.includes('$PKG') && !/^ {4}deprecated: /.test(l));
  if (bad.length > 0) offenders.push('snapshot ' + key + ': ' + bad.slice(0, 3).join(' | '));
}
for (const key of sa.keys()) {
  if (!sb.has(key) && !key.includes('$PKG')) offenders.push('+snapshot ' + key);
}
const ib = importerPins(before);
const ia = importerPins(after);
for (const [key, v] of ib) {
  if (ia.get(key) !== v && !key.includes('$PKG')) offenders.push('importer ' + key);
}
for (const key of ia.keys()) {
  if (!ib.has(key) && !key.includes('$PKG')) offenders.push('+importer ' + key);
}
// Whole-file safety net, independent of the block parsers above: every line
// added or removed anywhere must name the overridden package or be a registry
// 'deprecated:' notice. Catches sections the parsers do not model.
// Lines inside the overridden package's own blocks (its resolution/integrity
// lines do not repeat the name) are the only unnamed lines allowed to move.
const ownLines = new Set();
for (const m of [sb, sa]) for (const [k, ls] of m) if (k.includes('$PKG')) for (const l of ls) ownLines.add(l);
for (const l of changedLines(before.split('\n'), after.split('\n'))) {
  if (l.includes('$PKG') || ownLines.has(l) || /^ {4}deprecated: /.test(l)) continue;
  offenders.push('line: ' + l.trim().slice(0, 120));
}
if (offenders.length > 0) {
  console.error('assert-lockfile-diff: FAIL — lockfile changes outside $PKG:');
  for (const o of offenders.slice(0, 20)) console.error('  ' + o);
  process.exit(1);
}
console.log('assert-lockfile-diff: OK — lockfile diff touches only $PKG entries');
"
