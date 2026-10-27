import {readFile, writeFile} from 'node:fs/promises';
import {join} from 'node:path';

// Only active for the umbrella run; standalone browser tests remain unchanged.
export async function hold(name, origin, path) {
  const dir = process.env.COLLAB_E2E_CONTROL_DIR;
  if (!dir) return;
  await writeFile(join(dir, `${name}.ready`), JSON.stringify({origin, path}));
  const deadline = Date.now() + 180_000;
  while (Date.now() < deadline) {
    try {
      if ((await readFile(join(dir, `${name}.release`), 'utf8')).trim() === 'release') return;
    } catch (error) {
      if (error.code !== 'ENOENT') throw error;
    }
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  throw new Error(`umbrella release timeout: ${name}`);
}
