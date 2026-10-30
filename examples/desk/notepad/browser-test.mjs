// Optional real-browser check: PLAYWRIGHT_MODULE=/path/to/playwright/index.mjs
// NOTEPAD_BIN=/path/to/notepad node browser-test.mjs
import assert from 'node:assert/strict';
import {mkdtemp, rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {spawn} from 'node:child_process';

const {chromium} = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');

async function startServer(dir) {
  const bin = process.env.NOTEPAD_BIN || join(dir, 'notepad');
  const server = spawn(bin, ['-data', dir, '-addr', '127.0.0.1:0', 'serve']);
  let output = '';
  server.stdout.on('data', d => output += d);
  server.stderr.on('data', d => output += d);
  for (let i = 0; i < 100 && !output.includes('Participant:'); i++) {
    await new Promise(r => setTimeout(r, 100));
  }
  const participant = output.match(/Participant: (http:\/\/[^\s]+)/)?.[1];
  assert.ok(participant, 'Server failed to start or did not emit participant URL:\n' + output);
  return {server, participant, origin: new URL(participant).origin};
}

async function stopServer(server) {
  server.kill('SIGTERM');
  await new Promise(r => server.once('exit', r));
}

async function api(base, path, body, headers) {
  return await fetch(base + path, {
    method: body ? 'POST' : 'GET',
    headers: {'Content-Type': 'application/json', ...(headers || {})},
    body: body ? JSON.stringify(body) : undefined
  });
}

const dir = await mkdtemp(join(tmpdir(), 'notepad-browser-'));
let browser, srv;

try {
  srv = await startServer(dir);
  browser = await chromium.launch({
    headless: true,
    executablePath: process.env.CHROMIUM_PATH || undefined
  });

  const ctx = await browser.newContext({permissions: ['clipboard-read', 'clipboard-write']});
  const page = await ctx.newPage();

  // 1. Visit notepad root
  await page.goto(srv.participant);
  await page.locator('.desk-item-header').waitFor();
  assert.ok(await page.locator('.desk-badge').count() > 0, 'Status badges must be present');
  assert.ok(await page.locator('input[name="title"]').count() > 0, 'Title input must be present');
  assert.ok(await page.locator('textarea[name="body"]').count() > 0, 'Body textarea must be present');

  // 2. Fill note title and body
  await page.locator('input[name="title"]').fill('Browser E2E Note');
  await page.locator('textarea[name="body"]').fill('# E2E Heading\n\nNote text edited by human.');

  // 3. Save note
  await page.locator('#saveBtn').click();
  await page.locator('[data-desk-alert]').filter({hasText: 'Saved successfully'}).waitFor({timeout: 10000});

  // Verify URL updated with ?id=
  await page.waitForURL(/\?id=/, {timeout: 5000});
  const noteURL = page.url();
  const noteID = new URL(noteURL).searchParams.get('id');
  assert.ok(noteID, 'Saved note must have an id in URL');

  // 4. Verify API reflects content
  const stateRes = await api(srv.origin, '/api/state?id=' + encodeURIComponent(noteID));
  assert.equal(stateRes.status, 200);
  const state = await stateRes.json();
  assert.equal(state.id, noteID);
  assert.equal(state.body, undefined, 'open/state must never emit content');

  const getRes = await api(srv.origin, '/api/action', {action: 'get', id: noteID});
  assert.equal(getRes.status, 200);
  const noteData = await getRes.json();
  assert.equal(noteData.title, 'Browser E2E Note');
  assert.ok(noteData.body.includes('E2E Heading'), 'Body must be saved');

  // Preview goes through the kit renderer (semantic tags, .desk-content).
  await page.locator('.desk-content h1').filter({hasText: 'E2E Heading'}).waitFor({timeout: 5000});

  // 5. Test theme toggle
  const htmlTag = page.locator('html');
  await page.locator('[data-desk-theme]').click();
  const theme = await htmlTag.getAttribute('data-theme');
  assert.ok(theme === 'dark' || theme === 'light', 'data-theme attribute set on html');

  // 6. Test copy button
  const copyBtn = page.locator('[data-desk-copy]').first();
  await copyBtn.click();
  await page.waitForFunction(btn => btn.getAttribute('data-copied') === 'true', await copyBtn.elementHandle());

  console.log('browser-test.mjs passed cleanly for notepad');
} finally {
  if (browser) await browser.close();
  if (srv) await stopServer(srv.server);
  await rm(dir, {recursive: true, force: true});
}
