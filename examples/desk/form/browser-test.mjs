// Form browser test (runs on a test host, not on Dragon):
// PLAYWRIGHT_MODULE=/path/to/playwright/index.mjs FORM_BIN=/path/to/form \
//   node browser-test.mjs
//
// Round-trip: create a form via the agent API, fill it as a human on the
// loopback page (partial save survives reload), submit, and read the
// answers back as JSON keyed by field id.
import assert from 'node:assert/strict';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawn } from 'node:child_process';

const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const BIN = process.env.FORM_BIN;
assert.ok(BIN, 'FORM_BIN must point at a built form binary');

const DEF = [
  { id: 'name', label: 'Full name', kind: 'text', required: true },
  { id: 'contact', label: 'Preferred contact', kind: 'choice', required: true,
    options: ['email', 'phone', 'post'], recommended: 'email' },
  { id: 'topics', label: 'Topics', kind: 'multi',
    options: ['news', 'events', 'offers'], recommended: 'news' },
  { id: 'subscribe', label: 'Subscribe?', kind: 'yes-no', required: true },
];

async function startServer(dir) {
  const server = spawn(BIN, ['serve', '-data', dir, '-addr', '127.0.0.1:0']);
  let output = '';
  server.stdout.on('data', (d) => { output += d; });
  server.stderr.on('data', (d) => { output += d; });
  for (let i = 0; i < 100 && !output.includes('Participant:'); i++) {
    await new Promise((r) => setTimeout(r, 100));
  }
  const origin = output.match(/Participant: (http:\/\/[^\s]+)/)?.[1];
  assert.ok(origin, `server did not start: ${output}`);
  return { server, origin: new URL(origin).origin };
}

async function api(origin, body, key) {
  const headers = { 'Content-Type': 'application/json' };
  if (key) headers['X-Facilitator-Key'] = key;
  const r = await fetch(`${origin}/api/action`, {
    method: 'POST', headers, body: JSON.stringify(body),
  });
  return r;
}

const dir = await mkdtemp(join(tmpdir(), 'form-browser-'));
let browser, first;
try {
  first = await startServer(dir);
  browser = await chromium.launch({ headless: true });

  // Agent defines the form.
  const created = await (await api(first.origin, {
    action: 'add', title: 'Contact', definition: DEF,
  })).json();
  assert.ok(created.id && created.url, created);
  const key = created.ownerKey;
  assert.ok(key);

  const page = await (await browser.newContext()).newPage();
  await page.goto(`${first.origin}/?id=${created.id}`);

  // Recommended options carry a mark; required fields are flagged.
  assert.equal(await page.locator('.desk-recommended').count(), 2);
  assert.ok((await page.locator('body').textContent()).includes('email'));

  // Partial save: fill one field, save, reload, value persists.
  await page.locator('[data-field="name"] input').fill('Ada');
  await page.locator('[data-desk-save]').click();
  await page.waitForFunction(() => document.querySelector('.desk-item-header .desk-badge')?.textContent === 'pending');
  await page.reload();
  assert.equal(await page.locator('[data-field="name"] input').inputValue(), 'Ada');

  // Complete submit without the required choice is refused server-side.
  await page.locator('[data-desk-submit]').click();
  await page.locator('[data-desk-alert]').filter({ hasText: 'subscribe' }).waitFor();

  // Finish and submit: status flips to answered.
  await page.locator('[data-field="contact"] input[value="phone"]').check();
  await page.locator('[data-field="topics"] input[value="news"]').check();
  await page.locator('[data-field="subscribe"] input[value="yes"]').check();
  await page.locator('[data-desk-submit]').click();
  await page.waitForFunction(() => document.querySelector('.desk-item-header .desk-badge')?.textContent === 'answered');

  // Agent reads answers back keyed by field id.
  const got = await (await api(first.origin, { action: 'show', id: created.id }, key)).json();
  assert.equal(got.status, 'answered');
  assert.equal(got.answers.name, 'Ada');
  assert.equal(got.answers.contact, 'phone');
  assert.deepEqual(got.answers.topics, ['news']);
  assert.equal(got.answers.subscribe, 'yes');

  // open re-surfaces id+url+status only, never the answers.
  const opened = await (await api(first.origin, { action: 'open', id: created.id }, key)).json();
  assert.equal(opened.id, created.id);
  assert.ok(!JSON.stringify(opened).includes('Ada'));

  await page.screenshot({ path: '/tmp/form-browser.png', fullPage: true });
  console.log('PASS: define via API, recommended marks, partial save survives reload, required enforced, submit answers, get keyed by field id, contentless open');
} finally {
  await browser?.close();
  if (first?.server && first.server.exitCode === null) {
    first.server.kill('SIGTERM');
    await new Promise((r) => first.server.once('exit', r));
  }
  await rm(dir, { recursive: true, force: true });
}
