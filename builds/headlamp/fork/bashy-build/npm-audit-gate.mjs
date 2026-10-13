import fs from 'node:fs';

const report = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
// These packages are build-only at v0.45.0: vite.config includes only the
// process/buffer/stream polyfills (not crypto/elliptic), while braces and
// brace-expansion are used by Vite/static-copy/lint tooling. They are absent
// from the server binary and emitted browser chunks. Any other high/critical
// production-tree finding is treated as runtime-reachable and fails closed.
const reviewedBuildOnly = new Set(['brace-expansion', 'braces', 'chokidar', 'fast-glob', 'micromatch', 'shelljs', 'shx', 'elliptic', 'browserify-sign', 'create-ecdh', 'crypto-browserify', 'node-stdlib-browser', 'vite-plugin-node-polyfills', 'vite-plugin-static-copy']);
const bad = Object.entries(report.vulnerabilities || {}).filter(([, v]) =>
  ['high', 'critical'].includes(v.severity) && !reviewedBuildOnly.has(v.name));
if (bad.length) {
  console.error('npm audit: unreviewed runtime-reachable high/critical findings:', bad.map(([n]) => n).join(', '));
  process.exit(1);
}
console.log('npm audit: no runtime-reachable high/critical findings');
