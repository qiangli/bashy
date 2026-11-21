// Real-browser round-trip (gate-host lane only):
// PLAYWRIGHT_MODULE=/path/to/playwright/index.mjs CALENDAR_BIN=/path/to/calendar node browser-test.mjs
import assert from 'node:assert/strict';
import {access, mkdtemp, rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {dirname, join} from 'node:path';
import {execFileSync, spawn} from 'node:child_process';
import {fileURLToPath} from 'node:url';

const {chromium} = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const here = dirname(fileURLToPath(import.meta.url));

async function ensureBinary(dir) {
  if (process.env.CALENDAR_BIN) {
    await access(process.env.CALENDAR_BIN);
    return process.env.CALENDAR_BIN;
  }
  const built = join(dir, 'calendar-bin');
  execFileSync('go', ['build', '-trimpath', '-o', built, '.'], {
    cwd: here,
    env: {...process.env, GOWORK: 'off', CGO_ENABLED: '0'},
    stdio: 'inherit'
  });
  return built;
}

async function startServer(bin, dir) {
  const server = spawn(bin, ['-data', dir, '-addr', '127.0.0.1:0', 'serve'], {
    env: {...process.env, CALENDAR_SOURCE_DIR: here}
  });
  let output = '';
  server.stdout.on('data', d => { output += d; });
  server.stderr.on('data', d => { output += d; });
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

const dir = await mkdtemp(join(tmpdir(), 'calendar-browser-'));
let browser, srv;

try {
  const bin = await ensureBinary(dir);
  srv = await startServer(bin, dir);
  browser = await chromium.launch({
    headless: true,
    executablePath: process.env.CHROMIUM_PATH || undefined
  });

  // 1. Owner context: visit agenda root and verify Desk shell markers
  const ownerCtx = await browser.newContext({permissions: ['clipboard-read', 'clipboard-write']});
  const page = await ownerCtx.newPage();

  await page.goto(srv.participant + '?tz=America/New_York');
  await page.locator('.desk-appbar').waitFor();
  await page.locator('.desk-item-header').waitFor();
  assert.equal(await page.locator('html').getAttribute('data-desk-tool'), 'calendar');
  assert.ok(await page.locator('.desk-badge').count() > 0, 'Status badge must be present');

  // 2. Create an event with Markdown body and refs via the UI
  await page.locator('input[name="title"]').fill('Sprint 616 Review');
  await page.locator('input[name="start"]').fill('2026-11-25T17:00:00Z');
  await page.locator('input[name="end"]').fill('2026-11-25T18:00:00Z');
  await page.locator('input[name="tz"]').fill('UTC');
  await page.locator('input[name="location"]').fill('Room 42');
  await page.locator('input[name="refs"]').fill('kb:enable-an-app-for-consumer-shaping, https://example.com/spec');
  await page.locator('input[name="audience"]').fill('alice, bob');
  await page.locator('textarea[name="body"]').fill('# Agenda Notes\n\nWalk through calendar MVP.');

  await page.locator('#saveBtn').click();
  await page.locator('[data-desk-alert]').filter({hasText: 'Saved event'}).waitFor({timeout: 10000});

  await page.waitForURL(/\?id=/, {timeout: 5000});
  const evID = new URL(page.url()).searchParams.get('id');
  assert.ok(evID, 'Saved event must populate ?id= in URL');

  // 3. Verify detail panel renders Markdown body and refs through /api/render (.desk-content)
  await page.locator('#detailBody.desk-content h1').filter({hasText: 'Agenda Notes'}).waitFor({timeout: 5000});
  await page.locator('#detailRefs.desk-content a').first().waitFor({timeout: 5000});
  const detailWhen = await page.locator('#detailWhen').textContent();
  assert.ok(detailWhen.includes('12:00:00-05:00'), 'Viewer TZ America/New_York must convert 17:00Z to 12:00-05:00, got: ' + detailWhen);

  // 4. Stranger (non-member) receives HTTP 404 on state/show/export
  const strangerState = await api(srv.origin, '/api/state?id=' + encodeURIComponent(evID));
  assert.equal(strangerState.status, 404, 'Non-member GET /api/state must return 404');
  const strangerShow = await api(srv.origin, '/api/action', {action: 'show', id: evID});
  assert.equal(strangerShow.status, 404, 'Non-member show must return 404');

  // 5. Share single-use invite, join in a second browser context, then revoke -> 404
  await page.locator('#shareBtn').click();
  await page.locator('#shareBox').waitFor({state: 'visible'});
  const inviteURL = (await page.locator('#shareUrlText').textContent()).trim();
  assert.ok(inviteURL.includes('#invite='), 'Share must mint an invite URL');

  const memberCtx = await browser.newContext();
  const memberPage = await memberCtx.newPage();
  await memberPage.goto(inviteURL);
  await memberPage.locator('[data-desk-alert]').filter({hasText: 'Joined shared event'}).waitFor({timeout: 10000});
  await memberPage.locator('#detailTitle').filter({hasText: 'Sprint 616 Review'}).waitFor({timeout: 5000});

  // Reload owner view to see active members and revoke the joined member
  await page.locator('#applyFilterBtn').click();
  await page.locator('.cal-revoke-btn').nth(1).waitFor({timeout: 5000});
  // Revoke any member other than owner's own browser session, or revoke all and verify owner key keeps owner working
  const revokeBtns = page.locator('.cal-revoke-btn');
  const btnCount = await revokeBtns.count();
  for (let i = 0; i < btnCount; i++) {
    await revokeBtns.first().click();
    await page.locator('[data-desk-alert]').filter({hasText: 'Revoked member'}).waitFor({timeout: 5000});
  }

  // Member reloads and now gets 404
  await memberPage.locator('#applyFilterBtn').click();
  await memberPage.locator('[data-desk-alert]').filter({hasText: '404'}).waitFor({timeout: 5000});

  // 6. Cancel event in owner browser (ownerKey in localStorage still authorizes owner)
  await page.locator('#cancelBtn').click();
  await page.locator('[data-desk-alert]').filter({hasText: 'Cancelled event'}).waitFor({timeout: 5000});
  assert.equal(await page.locator('#detailBadge .desk-badge').getAttribute('data-status'), 'cancelled');

  // 7. Theme toggle & copy button
  await page.locator('[data-desk-theme]').click();
  const theme = await page.locator('html').getAttribute('data-theme');
  assert.ok(theme === 'dark' || theme === 'light', 'data-theme set on html');

  const copyBtn = page.locator('.desk-appbar [data-desk-copy]').first();
  await copyBtn.click();
  await page.waitForFunction(btn => btn.getAttribute('data-copied') === 'true', await copyBtn.elementHandle());

  console.log('browser-test.mjs passed cleanly for calendar');
} finally {
  if (browser) await browser.close();
  if (srv) await stopServer(srv.server);
  await rm(dir, {recursive: true, force: true});
}
