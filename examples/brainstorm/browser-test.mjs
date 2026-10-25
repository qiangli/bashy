// Optional real-browser check: PLAYWRIGHT_MODULE=/path/to/playwright/index.mjs
// BRAINSTORM_BIN=/path/to/brainstorm node browser-test.mjs
import assert from 'node:assert/strict';
import {mkdtemp, readFile, rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {spawn} from 'node:child_process';
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE||'playwright');
const dir=await mkdtemp(join(tmpdir(),'brainstorm-browser-'));
const server=spawn(process.env.BRAINSTORM_BIN,['-data',dir,'-addr','127.0.0.1:0']);
let output='';server.stdout.on('data',d=>output+=d);
let browser;
try {
 for(let i=0;i<100&&!output.includes('Teacher');i++)await new Promise(r=>setTimeout(r,100));
 const base=output.match(/Student: (http:\/\/[^\s]+)/)?.[1];assert.ok(base,output);
 const board=JSON.parse(await readFile(join(dir,'board.json'),'utf8'));
 browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||undefined});
 const ac=await browser.newContext(),bc=await browser.newContext(),tc=await browser.newContext();
 const alice=await ac.newPage(),bob=await bc.newPage(),teacher=await tc.newPage();
 await Promise.all([alice.goto(base),bob.goto(base),teacher.goto(base+'#teacher='+board.teacher)]);
 await alice.locator('#connection').filter({hasText:'Connected'}).waitFor();
 await alice.locator('#name').fill('Alice');
 await alice.locator('#text').fill('Help students find a quiet place to study.');
 await alice.locator('#submit').click();
 await alice.locator('.card').filter({hasText:'quiet place'}).waitFor();
 await bob.locator('#text').fill('Unsaved draft survives refresh');
 await bob.waitForTimeout(2400);
 assert.equal(await bob.locator('#text').inputValue(),'Unsaved draft survives refresh');
 assert.equal(await bob.locator('.card').count(),0);
 const privateState=await bc.request.get(base+'api/state');assert.ok(!(await privateState.text()).includes('quiet place'));
 await alice.getByRole('button',{name:'Edit',exact:true}).click();
 await alice.locator('#text').fill('Help students find a quiet place to study after school.');
 await alice.locator('#submit').click();
 await alice.locator('.card').filter({hasText:'after school'}).waitFor();
 await bob.locator('#text').fill('<img src=x onerror="window.injected=true">');
 await bob.locator('#submit').click();
 await bob.locator('.card').waitFor();
 assert.equal(await bob.locator('.card img').count(),0);
 for(let i=0;i<2;i++){await alice.locator('#text').fill('Another useful idea '+i);await alice.locator('#submit').click();await alice.waitForFunction(n=>document.querySelectorAll('.card').length===n,i+2)}
 teacher.on('dialog',d=>d.accept());
 await teacher.getByRole('button',{name:'Reveal all ideas',exact:true}).click();
 await bob.waitForFunction(()=>document.querySelectorAll('.card').length===4);
 assert.equal(await bob.evaluate(()=>window.injected),undefined);
 await teacher.getByRole('button',{name:'Open voting',exact:true}).click();
 await alice.getByRole('button',{name:'+ Vote',exact:true}).first().waitFor();
 for(let i=0;i<3;i++){await alice.getByRole('button',{name:'+ Vote',exact:true}).first().click();await alice.waitForFunction(n=>document.querySelectorAll('.voted').length===n,i+1)}
 assert.equal(await alice.getByRole('button',{name:'+ Vote',exact:true}).isDisabled(),true);
 await alice.getByRole('button',{name:'✓ Voted · undo',exact:true}).first().click();
 await alice.waitForFunction(()=>document.querySelectorAll('.voted').length===2);
 const phone=await browser.newContext({viewport:{width:390,height:844}});const mobile=await phone.newPage();await mobile.goto(base);await mobile.locator('.card').first().waitFor();
 assert.ok(await mobile.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
 await teacher.getByRole('button',{name:'Close voting',exact:true}).click();
 await teacher.getByText('Session complete.',{exact:false}).waitFor();
 const download=teacher.waitForEvent('download');await teacher.getByRole('button',{name:'Download JSON',exact:true}).click();assert.equal((await download).suggestedFilename(),'brainstorm.json');
 const stored=JSON.parse(await readFile(join(dir,'board.json'),'utf8'));assert.equal(stored.phase,'closed');assert.equal(stored.cards.length,4);
 await teacher.screenshot({path:'/tmp/brainstorm-browser-results.png',fullPage:true});
 console.log('PASS: independent browsers, private API, edit, drafts, reveal, XSS text, voting, mobile layout, close, export');
} finally {await browser?.close();server.kill('SIGTERM');await new Promise(r=>server.once('exit',r));await rm(dir,{recursive:true,force:true})}
