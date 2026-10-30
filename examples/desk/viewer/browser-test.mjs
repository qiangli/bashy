// Optional real-browser check: PLAYWRIGHT_MODULE=/path/to/playwright/index.mjs
// VIEWER_BIN=/path/to/viewer node browser-test.mjs
import assert from 'node:assert/strict';
import {mkdtemp, rm, readFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {spawn, spawnSync} from 'node:child_process';

const {chromium} = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const bin = process.env.VIEWER_BIN || 'viewer';

async function startServer(dir) {
  const server = spawn(bin, ['--dir', dir, '--addr', '127.0.0.1:0', 'serve']);
  let output = '';
  server.stdout.on('data', d => output += d);
  server.stderr.on('data', d => output += d);
  for (let i = 0; i < 100 && !output.includes('Participant:'); i++) {
    await new Promise(r => setTimeout(r, 100));
  }
  const participant = output.match(/Participant: (http:\/\/[^\s]+)/)?.[1];
  assert.ok(participant, 'Server failed to start:\n' + output);
  return {server, participant, addr: new URL(participant).host};
}

function cli(dir, addr, ...args) {
  const r = spawnSync(bin, ['--dir', dir, '--addr', addr, ...args], {encoding: 'utf8'});
  return {code: r.status, out: (r.stdout || '') + (r.stderr || '')};
}

const dir = await mkdtemp(join(tmpdir(), 'viewer-browser-'));
let browser, srv;

try {
  srv = await startServer(dir);
  // Agent door: a session book, three pages, one from a template with hostile data.
  let r = cli(dir, srv.addr, 'add', '--session', 'e2e', '--body', '# Day 1\n\nAlpha stays.\n\nBeta original.\n');
  assert.equal(r.code, 0, r.out);
  assert.match(r.out, /^session-e2e http:\/\/\S+\?id=session-e2e pending|filled-by-agent/);
  r = cli(dir, srv.addr, 'append', 'session-e2e', '--body', '# Day 2\n\nAlpha stays.\n\nBeta revised.\n');
  assert.equal(r.code, 0, r.out);
  const data = JSON.stringify({title: 'Status', summary: 'All **green**.',
    done: ['<img src=x onerror="window.__pwned=1"> shipped', '<script>window.__pwned=1</script>'],
    metrics: [{label: 'pass', value: 86}, {label: 'fail', value: 0}]});
  r = cli(dir, srv.addr, 'append', 'session-e2e', '--template', 'status', '--data', data);
  assert.equal(r.code, 0, r.out);
  r = cli(dir, srv.addr, 'set', 'session-e2e', '--body', 'edited');
  assert.equal(r.code, 2, 'view-only: set must exit 2');

  browser = await chromium.launch({headless: true, executablePath: process.env.CHROMIUM_PATH || undefined});
  const ctx = await browser.newContext({permissions: ['clipboard-read', 'clipboard-write'], acceptDownloads: true});
  const page = await ctx.newPage();

  // 1. Root lists the book; the jump field opens it.
  await page.goto(srv.participant);
  await page.locator('.desk-appbar').waitFor();
  assert.ok(await page.locator('a', {hasText: 'session-e2e'}).count() > 0, 'book listed at root');
  await page.locator('input[name="id"]').fill('session-e2e');
  await page.locator('button[type="submit"]').click();
  await page.waitForURL(/id=session-e2e/);

  // 2. Latest page: template rendered by the kit renderer, chart SVG, data neutralised.
  await page.locator('.vw-pos', {hasText: 'p3 of 3'}).waitFor();
  await page.locator('.desk-content h1', {hasText: 'Status'}).waitFor();
  await page.locator('.desk-content strong', {hasText: 'green'}).waitFor();
  assert.ok(await page.locator('.desk-content svg').count() > 0, 'metrics chart renders as SVG');
  assert.equal(await page.evaluate(() => window.__pwned), undefined, 'data must never execute');
  assert.ok((await page.locator('.desk-content').innerText()).includes('<script>'), 'raw HTML in data is shown as text');
  assert.equal(await page.locator('textarea, [contenteditable="true"], .desk-content input:not([type=checkbox])').count(), 0, 'view-only: nothing editable');

  // 3. Navigation: prev via keyboard, first/last via buttons, index, jump.
  await page.keyboard.press('ArrowLeft');
  await page.locator('.vw-pos', {hasText: 'p2 of 3'}).waitFor();
  // Changed since p1: heading + Beta highlighted, Alpha not.
  assert.equal(await page.locator('.vw-block[data-changed="true"]').count(), 2);
  assert.equal(await page.locator('.vw-block[data-changed="true"]', {hasText: 'Alpha'}).count(), 0);
  await page.locator('[data-vw-changes="2"]').waitFor();
  await page.locator('a[rel="first"]').click();
  await page.locator('.vw-pos', {hasText: 'p1 of 3'}).waitFor();
  assert.equal(await page.locator('[data-vw-changes]').count(), 0, 'first page has no comparison');
  await page.locator('a', {hasText: 'Index'}).click();
  await page.locator('table.vw-index tbody tr').nth(2).waitFor();
  await page.locator('input[name="page"]').fill('p2');
  await page.locator('input[name="page"]').press('Enter');
  await page.locator('.vw-pos', {hasText: 'p2 of 3'}).waitFor();

  // 4. Theme toggle and copy button (kit shell).
  await page.locator('[data-desk-theme]').click();
  const theme = await page.locator('html').getAttribute('data-theme');
  assert.ok(theme === 'dark' || theme === 'light', 'theme toggles');
  const copyBtn = page.locator('[data-desk-copy]').first();
  await copyBtn.click();
  await page.waitForFunction(btn => btn.getAttribute('data-copied') === 'true', await copyBtn.elementHandle());

  // 5. Export: the downloaded book is self-contained and opens offline.
  const [download] = await Promise.all([page.waitForEvent('download'), page.locator('a', {hasText: 'Download book'}).click()]);
  const exported = join(dir, 'export.html');
  await download.saveAs(exported);
  const html = await readFile(exported, 'utf8');
  assert.ok(html.includes('Day 1') && html.includes('Day 2') && html.includes('href="#p3"'), 'export has every page');
  const offline = await ctx.newPage();
  await offline.route('**/*', route => route.request().url().startsWith('file:') ? route.continue() : route.abort());
  await offline.goto('file://' + exported);
  await offline.locator('#p2 .vw-block[data-changed="true"]').first().waitFor();
  assert.equal(await offline.evaluate(() => window.__pwned), undefined);

  console.log('browser-test.mjs passed cleanly for viewer');
} finally {
  if (browser) await browser.close();
  if (srv) { srv.server.kill('SIGTERM'); await new Promise(r => srv.server.once('exit', r)); }
  await rm(dir, {recursive: true, force: true});
}
