// Run on the designated browser host, with Playwright already installed.
// node browser-test.mjs; no package download or browser install is performed.
import assert from 'node:assert/strict';
import { spawn, execFileSync } from 'node:child_process';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright';

const here = path.dirname(fileURLToPath(import.meta.url));
const dir = await mkdtemp(path.join(tmpdir(), 'desk-chart-browser-'));
const bin = path.join(dir, process.platform === 'win32' ? 'chart.exe' : 'chart');
let server, browser;
try {
  execFileSync('go', ['build', '-o', bin, '.'], { cwd: here, env: { ...process.env, GOWORK: 'off', CGO_ENABLED: '0' }, stdio: 'inherit' });
  server = spawn(bin, ['serve', '--addr', '127.0.0.1:0', '--data', path.join(dir, 'data')]);
  const base = await new Promise((resolve, reject) => {
    let output = '';
    const timer = setTimeout(() => reject(Error('server startup timeout')), 15000);
    server.on('error', reject);
    server.on('exit', code => reject(Error(`server exited ${code}`)));
    server.stdout.on('data', chunk => {
      output += chunk;
      const match = output.match(/Participant: (http:\/\/127\.0\.0\.1:\d+)\//);
      if (match) { clearTimeout(timer); resolve(match[1]); }
    });
    server.stderr.on('data', chunk => process.stderr.write(chunk));
  });
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ colorScheme: 'light', permissions: ['clipboard-read', 'clipboard-write'] });
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  await context.route('**/*', route => new URL(route.request().url()).origin === base ? route.continue() : route.abort());
  await page.goto(base);
  await page.locator('[name=title]').fill('Browser stats');
  await page.locator('[name=input]').fill('[3,5,9]');
  await page.locator('[name=widget]').selectOption('gauge');
  await page.getByRole('button', { name: 'Create chart', exact: true }).click();
  await page.waitForURL('**/?id=*');
  await page.locator('svg').waitFor();
  assert.match(await page.locator('svg').textContent(), /9/);
  await page.getByRole('button', { name: 'Copy link', exact: true }).click();
  await page.locator('[data-copied=true]').waitFor();
  const before = await page.locator('svg').evaluate(el => getComputedStyle(el).color);
  await page.getByRole('button', { name: 'Toggle theme' }).click();
  assert.notEqual(await page.locator('svg').evaluate(el => getComputedStyle(el).color), before);
  await page.getByRole('button', { name: 'Share', exact: true }).click();
  await page.waitForFunction(() => document.querySelector('#message').textContent.includes('#invite='));
  const invite = (await page.locator('#message').textContent()).replace('Single-use invite: ', '');
  const guestContext = await browser.newContext();
  const guest = await guestContext.newPage();
  await guest.goto(invite);
  await guest.locator('svg').waitFor();
  assert(!guest.url().includes('#invite='));
  await page.getByRole('button', { name: 'Revoke sharing' }).click();
  await page.waitForFunction(() => document.querySelector('#message').textContent === 'Sharing revoked.');
  await guest.reload();
  await guest.locator('#unlock:not([hidden])').waitFor();
  assert.equal(await guest.locator('svg').count(), 0);
  await page.getByRole('button', { name: 'Close', exact: true }).click();
  await page.locator('.desk-badge[data-status=closed]').waitFor();
  assert.deepEqual(errors, []);
  console.log('PASS: browser create, server SVG, copy, theme, share, revoke, close; no CDN');
} finally {
  await browser?.close();
  if (server && server.exitCode === null) {
    server.kill();
    await new Promise(resolve => { server.once('exit', resolve); setTimeout(resolve, 4000); });
  }
  await rm(dir, { recursive: true, force: true });
}
