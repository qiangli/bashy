// Real-browser happy path: PLAYWRIGHT_MODULE=/path/to/playwright/index.mjs
// BRAINSTORM_BIN=/path/to/brainstorm node browser-test.mjs
// CHROMIUM_PATH may select an existing browser.
import assert from 'node:assert/strict';
import {mkdtemp, readFile, rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {spawn} from 'node:child_process';
import {hold} from '../../tests/collaboration-e2e/barrier.mjs';
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE||'playwright');

async function startServer(dir,args){
 const server=spawn(process.env.BRAINSTORM_BIN,['-data',dir,'-addr','127.0.0.1:0',...args]);
 let output='';server.stdout.on('data',d=>output+=d);server.stderr.on('data',d=>output+=d);
 for(let i=0;i<100&&!output.includes('Participant:');i++)await new Promise(r=>setTimeout(r,100));
 const participant=output.match(/Participant: (http:\/\/[^\s]+)/)?.[1];
 const facilitator=output.match(/Facilitator \(keep private\): (http:\/\/[^\s]+)/)?.[1];
 assert.ok(participant,output);assert.ok(facilitator,output);
 return {server,participant,facilitator,origin:new URL(participant).origin};
}
async function stopServer(server){
 server.kill('SIGTERM');await new Promise(r=>server.once('exit',r));
}
async function cards(page,n){
 await page.waitForFunction(n=>document.querySelectorAll('.card').length===n,n,{timeout:15000});
}

const dir=await mkdtemp(join(tmpdir(),'brainstorm-browser-'));
let browser,first,second;
try {
 // --- standalone: no prefix, whiteboard/Matrix absent ---
 first=await startServer(dir,[]);
 browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||undefined});
 const facCtx=await browser.newContext(),aCtx=await browser.newContext(),bCtx=await browser.newContext();
 const fac=await facCtx.newPage(),alice=await aCtx.newPage(),bob=await bCtx.newPage();
 // Facilitator creates a named session with a custom vote budget.
 await fac.goto(first.facilitator);
 await fac.locator('#connection').filter({hasText:'Connected'}).waitFor();
 await fac.locator('#sessionInput').fill('e2e');
 await fac.locator('#titleInput').fill('E2E Session');
 await fac.locator('#budgetInput').fill('2');
 await fac.locator('#createSession').click();
 await fac.locator('#connection').filter({hasText:'Connected · e2e'}).waitFor();
 assert.equal(await fac.locator('h1').textContent(),'E2E Session');
 const link=(await fac.locator('#participantLink').textContent()).match(/http:\/\/[^\s]+/)?.[0];
 assert.ok(link);
 await Promise.all([alice.goto(link),bob.goto(link)]);
 await alice.locator('#connection').filter({hasText:'Connected · e2e'}).waitFor();
 await bob.locator('#connection').filter({hasText:'Connected · e2e'}).waitFor();
 // Three contexts write privately; each sees only its own cards.
 await alice.locator('#name').fill('Alice');
 await alice.locator('#text').fill('Help neighbors find a quiet place to study.');
 await alice.locator('#submit').click();
 await cards(alice,1);
 await bob.locator('#text').fill('Unsaved draft survives refresh');
 await bob.waitForTimeout(2400);
 assert.equal(await bob.locator('#text').inputValue(),'Unsaved draft survives refresh');
 assert.equal(await bob.locator('.card').count(),0);
 const privateState=await bCtx.request.get(first.origin+'/api/state?session=e2e');
 assert.ok(!(await privateState.text()).includes('quiet place'));
 await fac.locator('#text').fill('A tool library for the block.');
 await fac.locator('#submit').click();
 await cards(fac,1);
 assert.equal(await alice.locator('.card').count(),1);
 // Unsaved draft becomes a second idea; markup stays inert text.
 await bob.locator('#submit').click();
 await cards(bob,1);
 await bob.locator('#text').fill('<img src=x onerror="window.injected=true">');
 await bob.locator('#submit').click();
 await cards(bob,2);
 assert.equal(await bob.locator('.card img').count(),0);
 // Reveal: every browser sees all four ideas.
 fac.on('dialog',d=>d.accept());
 await fac.getByRole('button',{name:'Reveal all ideas',exact:true}).click();
 for(const p of [fac,alice,bob])await cards(p,4);
 assert.equal(await bob.evaluate(()=>window.injected),undefined);
 // Voting is constrained by the session budget of 2.
 await fac.getByRole('button',{name:'Open voting',exact:true}).click();
 await alice.getByRole('button',{name:'+ Vote',exact:true}).first().waitFor();
 for(let i=0;i<2;i++){await alice.getByRole('button',{name:'+ Vote',exact:true}).first().click();await alice.waitForFunction(n=>document.querySelectorAll('.voted').length===n,i+1)}
 assert.equal(await alice.getByRole('button',{name:'+ Vote',exact:true}).first().isDisabled(),true);
 await alice.getByRole('button',{name:'✓ Voted · undo',exact:true}).first().click();
 await alice.waitForFunction(()=>document.querySelectorAll('.voted').length===1);
 await alice.getByRole('button',{name:'+ Vote',exact:true}).first().click();
 await alice.waitForFunction(()=>document.querySelectorAll('.voted').length===2);
 const phone=await browser.newContext({viewport:{width:390,height:844}});const mobile=await phone.newPage();await mobile.goto(link);await cards(mobile,4);
 assert.ok(await mobile.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
 // Close, export, and verify the saved session.
 await fac.getByRole('button',{name:'Close voting',exact:true}).click();
 await fac.getByText('Session complete.',{exact:false}).waitFor();
 const download=fac.waitForEvent('download');await fac.getByRole('button',{name:'Download JSON',exact:true}).click();assert.equal((await download).suggestedFilename(),'brainstorm-e2e.json');
 const stored=JSON.parse(await readFile(join(dir,'sessions','e2e.json'),'utf8'));assert.equal(stored.phase,'closed');assert.equal(stored.cards.length,4);assert.equal(stored.voteBudget,2);
 await fac.screenshot({path:'/tmp/brainstorm-browser-standalone.png',fullPage:true});
 await stopServer(first.server);
 // --- restart with the same data: the session resumes ---
 first=await startServer(dir,[]);
 const savedKey=JSON.parse(await readFile(join(dir,'sessions','e2e.json'),'utf8')).facilitator;
 await fac.goto(first.origin+'/?session=e2e#facilitator='+savedKey);
 await fac.locator('#connection').filter({hasText:'Connected · e2e'}).waitFor();
 await cards(fac,4);
 const exp=await facCtx.request.get(first.origin+'/api/export?session=e2e&format=json',{headers:{'X-Facilitator-Key':savedKey}});
 assert.equal(exp.status(),200);
 const exported=await exp.json();
 assert.equal(exported.cards.length,4);
 assert.ok(!JSON.stringify(exported).includes('owner')&&!JSON.stringify(exported).includes(savedKey));
 const md=await facCtx.request.get(first.origin+'/api/export?session=e2e&format=markdown',{headers:{'X-Facilitator-Key':savedKey}});
 assert.equal(md.status(),200);
 assert.ok((await md.text()).includes('# E2E Session'));
 await stopServer(first.server);
 // --- same data under a sub-path prefix ---
 second=await startServer(dir,['-prefix','/bs']);
 const pCtx=await browser.newContext(),qCtx=await browser.newContext();
 const p=await pCtx.newPage(),q=await qCtx.newPage();
 await p.goto(second.origin+'/bs/?session=e2e');
 await p.locator('#connection').filter({hasText:'Connected · e2e'}).waitFor();
 await q.goto(second.origin+'/bs/?session=e2e');
 await q.locator('#connection').filter({hasText:'Connected · e2e'}).waitFor();
 await cards(p,4);await cards(q,4);
 assert.equal((await pCtx.request.get(second.origin+'/api/state?session=e2e')).status(),404);
 assert.equal((await pCtx.request.get(second.origin+'/bs/api/state?session=e2e')).status(),200);
 const pexp=await pCtx.request.get(second.origin+'/bs/api/export?session=e2e&format=json',{headers:{'X-Facilitator-Key':savedKey}});
 assert.equal(pexp.status(),200);assert.equal((await pexp.json()).cards.length,4);
 await q.screenshot({path:'/tmp/brainstorm-browser-prefix.png',fullPage:true});
 await hold('brainstorm', second.origin, '/bs/api/state?session=e2e');
 console.log('PASS: create session, 3-context private writing, reveal, budget-constrained votes, close, JSON/markdown export, restart persistence, standalone + prefix paths');
} finally {await browser?.close();if(first?.server&&!first.server.killed&&first.server.exitCode===null)try{await stopServer(first.server)}catch{}if(second)try{await stopServer(second.server)}catch{}await rm(dir,{recursive:true,force:true})}
