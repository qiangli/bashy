import fs from 'node:fs';
import path from 'node:path';

const dir = process.argv[2];
const file = path.join(dir, 'package.json');
const pkg = JSON.parse(fs.readFileSync(file, 'utf8'));
pkg.dependencies['js-yaml'] = '4.3.2';
pkg.overrides = {
  ...pkg.overrides,
  'js-yaml': '4.3.2',
  browserslist: '4.28.7',
  'fast-uri': '3.1.8',
  'source-map-js': '1.2.2',
};
fs.writeFileSync(file, `${JSON.stringify(pkg, null, 2)}\n`);
