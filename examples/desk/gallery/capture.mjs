// Desk family gallery: start every desk tool, seed one representative item,
// screenshot it in light and dark, and write index.html — one side-by-side
// page (styled by kit.css itself) that shows the tools read as one suite.
//
//   PLAYWRIGHT_MODULE=/path/to/playwright/index.mjs \
//   DESK_BIN=/dir/with/{notepad,form,chart,whiteboard,brainstorm} node capture.mjs
//
// Build the binaries with each tool's `GOWORK=off go build`. CHROMIUM_PATH
// may select an existing browser. Output: ./shots/*.jpg + ./index.html.
import assert from 'node:assert/strict';
import {mkdtemp, mkdir, readFile, rm, writeFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {dirname, join} from 'node:path';
import {spawn} from 'node:child_process';
import {fileURLToPath} from 'node:url';

const {chromium} = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const here = dirname(fileURLToPath(import.meta.url));
const bins = process.env.DESK_BIN || assert.fail('DESK_BIN is required');

const NOTE = `# Release notes

Desk tools share **one look**: tokens, shell and this renderer.

- Notepad preview, viewer, form help and brainstorm cards
- Charts and \`\`\`chart embeds use the kit data palette

| Tool | Accent |
|------|--------|
| notepad | blue |
| chart | violet |

\`\`\`chart
{"mon": 12, "tue": 18, "wed": 15, "thu": 24, "fri": 31}
\`\`\`
`;

const TOOLS = [
  {name: 'notepad', args: d => ['-data', d, '-addr', '127.0.0.1:0', 'serve'],
    async seed(origin) {
      const v = await act(origin, {action: 'add', id: 'release-notes', title: 'Release notes', body: NOTE});
      return {url: `${origin}/?id=${v.id}`};
    }},
  {name: 'form', args: d => ['serve', '-data', d, '-addr', '127.0.0.1:0'],
    async seed(origin) {
      const v = await act(origin, {action: 'add', title: 'Contact', definition: [
        {id: 'name', label: 'Full name', kind: 'text', required: true, help: 'As it should appear on the **badge**.'},
        {id: 'contact', label: 'Preferred contact', kind: 'choice', options: ['email', 'phone', 'post'], recommended: 'email'},
        {id: 'subscribe', label: 'Subscribe?', kind: 'yes-no'},
      ]});
      return {url: `${origin}/?id=${v.id}`};
    }},
  {name: 'chart', args: d => ['serve', '--addr', '127.0.0.1:0', '--data', d],
    async seed(origin) {
      const v = await act(origin, {action: 'add', title: 'Requests', chart: {input: '[12,18,15,24,31,28,35]', widget: 'line', title: 'Requests'}});
      return {url: v.url, init: [`chart-key-${v.id}`, v.ownerKey]};
    }},
  {name: 'whiteboard', args: d => ['-data', d, '-addr', '127.0.0.1:0'],
    async seed(origin) { return {url: `${origin}/`, ready: '#connection', async after(page) {
      const box = await page.locator('#canvas').boundingBox();
      await page.mouse.move(box.x + box.width * .1, box.y + box.height * .3);
      await page.mouse.down();
      for (let i = 1; i <= 12; i++) await page.mouse.move(box.x + box.width * (.1 + i * .03), box.y + box.height * (.3 + Math.sin(i / 2) * .1));
      await page.mouse.up();
      await page.locator('#noteText').fill('Ship the **desk kit** today');
      await page.locator('#addNote').click();
      await page.locator('.note').first().waitFor();
    }}; }},
  {name: 'brainstorm', args: d => ['-data', d, '-addr', '127.0.0.1:0'],
    async seed(origin) { return {url: `${origin}/`, ready: '#connection', async after(page) {
      await page.locator('#text').fill('One shared look across *every* desk tool');
      await page.locator('#submit').click();
      await page.locator('.card').first().waitFor();
    }}; }},
];

async function act(origin, body) {
  const r = await fetch(origin + '/api/action', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)});
  const v = await r.json();
  assert.ok(r.ok, JSON.stringify(v));
  return v;
}

async function start(tool, dir) {
  const server = spawn(join(bins, tool.name), tool.args(dir));
  let out = '';
  server.stdout.on('data', d => out += d);
  server.stderr.on('data', d => out += d);
  for (let i = 0; i < 100 && !/Participant: http/.test(out); i++) await new Promise(r => setTimeout(r, 100));
  const url = out.match(/Participant: (http:\/\/[^\s]+)/)?.[1];
  assert.ok(url, `${tool.name} did not start:\n${out}`);
  return {server, origin: new URL(url).origin};
}

const SHELL_ACTIONS = ['Copy link', 'Toggle theme'];

// Every tool page must show the same top-bar actions and exactly one status
// badge. DESK_PLANT=extra|missing mutates the DOM to prove the check goes red.
async function assertShell(page, label) {
  if (process.env.DESK_PLANT === 'extra') await page.evaluate(() => document.querySelector('.desk-appbar').insertAdjacentHTML('beforeend', '<button type="button">Extra</button>'));
  if (process.env.DESK_PLANT === 'missing') await page.evaluate(() => document.querySelector('#deskShareLink')?.remove());
  const actions = await page.locator('.desk-appbar button').evaluateAll(bs => bs.map(b => (b.getAttribute('aria-label') || b.textContent).trim()));
  assert.deepEqual(actions, SHELL_ACTIONS, `${label}: top-bar actions differ`);
  assert.equal(await page.locator('.desk-badge').count(), 1, `${label}: expected exactly one status badge`);
  assert.equal(await page.getByRole('button', {name: 'Copy link'}).count(), 1, `${label}: duplicate Copy link`);
}

const shots = join(here, 'shots');
await mkdir(shots, {recursive: true});
const browser = await chromium.launch({headless: true, executablePath: process.env.CHROMIUM_PATH || undefined});
const rows = [];
try {
  for (const tool of TOOLS) {
    const dir = await mkdtemp(join(tmpdir(), `desk-gallery-${tool.name}-`));
    const {server, origin} = await start(tool, dir);
    try {
      const s = await tool.seed(origin);
      for (const scheme of ['light', 'dark']) {
        const ctx = await browser.newContext({colorScheme: scheme, viewport: {width: 1180, height: 760}, deviceScaleFactor: 1});
        if (s.init) await ctx.addInitScript(([k, v]) => sessionStorage.setItem(k, v), s.init);
        const page = await ctx.newPage();
        const errors = [];
        page.on('pageerror', e => errors.push(e.message));
        await page.goto(s.url);
        await page.locator('.desk-appbar').waitFor();
        if (s.ready) await page.locator(s.ready).filter({hasText: 'Connected'}).waitFor();
        if (s.after && scheme === 'light') await s.after(page);
        if (tool.name === 'chart') await page.locator('#charts svg').waitFor();
        await page.waitForTimeout(300);
        await assertShell(page, `${tool.name}/${scheme}`);
        const file = `${tool.name}-${scheme}.jpg`;
        await page.screenshot({path: join(shots, file), type: 'jpeg', quality: 72});
        assert.deepEqual(errors, [], `${tool.name}/${scheme}: page errors`);
        rows.push({tool: tool.name, scheme, file});
        await ctx.close();
      }
    } finally {
      server.kill('SIGTERM');
      await new Promise(r => server.once('exit', r));
      await rm(dir, {recursive: true, force: true});
    }
  }
} finally {
  await browser.close();
}

const kitCSS = await readFile(join(here, '..', 'kit', 'kit.css'), 'utf8');
const figs = TOOLS.map(t => `<section class="desk-panel"><h2 class="desk-h2">${t.name}</h2><div class="g-pair">` +
  ['light', 'dark'].map(s => `<figure><a href="shots/${t.name}-${s}.jpg"><img src="shots/${t.name}-${s}.jpg" alt="${t.name} ${s}" loading="lazy"></a><figcaption class="desk-muted">${s}</figcaption></figure>`).join('') +
  `</div></section>`).join('\n');
await writeFile(join(here, 'index.html'), `<!doctype html><html lang="en" data-desk-tool="viewer"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>Desk family gallery</title>
<style>${kitCSS}
.g-pair{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,420px),1fr));gap:var(--desk-space-4)}
.g-pair figure{margin:0}.g-pair img{width:100%;height:auto;border:1px solid var(--desk-line);border-radius:var(--desk-radius-md)}
</style></head><body class="desk-app">
<header class="desk-appbar"><span class="desk-brand"><span class="desk-icon" aria-hidden="true">👁</span><span class="desk-name">Desk family gallery</span></span>
<div class="desk-item-header"><span class="desk-id">light + dark</span></div></header>
<main class="desk-main"><p class="desk-muted">Every desk tool on the kit tokens and app shell, captured by <code>capture.mjs</code> (${new Date().toISOString().slice(0, 10)}).</p>
${figs}
</main></body></html>
`);
console.log(`PASS: ${rows.length} screenshots (${TOOLS.length} tools × light/dark) → ${join(here, 'index.html')}`);
