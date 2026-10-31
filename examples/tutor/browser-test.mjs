// Real-browser check of the tutor server: display, 390px phone, owner, and
// a join from the LAN address. A fake `bashy` (node script) supplies canned status.
//   PLAYWRIGHT_MODULE=/path/to/playwright/index.mjs [CHROMIUM_PATH=...] node browser-test.mjs
// TUTOR_SERVER_BIN selects a prebuilt server; otherwise it is built from server/.
import assert from 'node:assert/strict';
import {mkdtemp, writeFile, readFile, chmod, rm} from 'node:fs/promises';
import {tmpdir, networkInterfaces} from 'node:os';
import {join, dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import {spawn, spawnSync} from 'node:child_process';
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE||'playwright');
const here=dirname(fileURLToPath(import.meta.url));

const dir=await mkdtemp(join(tmpdir(),'tutor-browser-'));
let exe=process.env.TUTOR_SERVER_BIN;
if(!exe){exe=join(dir,'tutor-server');
 const b=spawnSync('go',['build','-o',exe,'.'],{cwd:join(here,'server'),encoding:'utf8',env:{...process.env,GOWORK:'off',CGO_ENABLED:'0'}});
 assert.equal(b.status,0,b.stderr)}

const state=join(dir,'status.json'),calls=join(dir,'calls.log');
const status=(over)=>({sprint:7,title:'Robotics session',format:'meeting',roles:{owner:'facilitator',participants:['participant']},clock:{state:'running'},
 current:{index:2,total:3,id:'t2',title:'Build the arm',owners:['alice'],timebox_s:600,elapsed_s:over?700:100,remaining_s:over?-100:500,overdue:over},
 steps:[{index:1,id:'t1',title:'Warm up',owner:'bob',role:'participant',status:'done',timebox_s:300,overdue:false,evidence:null},
  {index:2,id:'t2',title:'Build the arm',owner:'alice',role:'participant',status:'doing',timebox_s:600,overdue:over,evidence:null},
  {index:3,id:'t3',title:'Share',owner:'bob',role:'participant',status:'todo',timebox_s:300,overdue:false,evidence:null}]});
await writeFile(state,JSON.stringify(status(false)));
const fake=join(dir,'bashy');
await writeFile(fake,`#!/usr/bin/env node
const fs=require('fs');fs.appendFileSync(${JSON.stringify(calls)},process.argv.slice(2).join(' ')+'\\n');
if(process.argv[3]==='status')process.stdout.write(fs.readFileSync(${JSON.stringify(state)}));else console.log('{"ok":true}');
`);
await chmod(fake,0o755);

const env={...process.env,BASHY_BIN:fake,XDG_DATA_HOME:join(dir,'xdg')};
const server=spawn(exe,['serve','7','--addr','0.0.0.0:0'],{env});
let out='';server.stdout.on('data',d=>out+=d);server.stderr.on('data',d=>out+=d);
for(let i=0;i<100&&!out.includes('Join bob');i++)await new Promise(r=>setTimeout(r,100));
const display=out.match(/Display: (\S+)/)?.[1],owner=out.match(/Facilitator \(keep private\): (\S+)/)?.[1];
const alice=out.match(/Join alice: (\S+)/)?.[1],bob=out.match(/Join bob: (\S+)/)?.[1];
assert.ok(display&&owner&&alice&&bob,out);
const tokens=[owner.split('/').pop(),alice.split('/').pop(),bob.split('/').pop()];

let browser;
try{
 browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||undefined});
 // --- display, 1280x720 ---
 const proj=await (await browser.newContext({viewport:{width:1280,height:720}})).newPage();
 await proj.goto(display);
 await proj.locator('h1',{hasText:'Build the arm'}).waitFor();
 const clock=await proj.locator('#clock').textContent();
 assert.match(clock,/^\d+:\d\d$/);
 assert.ok(await proj.locator('#clock').evaluate(e=>parseFloat(getComputedStyle(e).fontSize))>=120,'big countdown');
 const html=await proj.content();
 for(const t of tokens)assert.ok(!html.includes(t),'display must not show tokens');
 assert.equal(await proj.locator('.desk-appbar').count(),1,'kit app bar');
 assert.equal(await proj.locator('#clock[data-over]').count(),0);
 await writeFile(state,JSON.stringify(status(true)));
 await proj.locator('#clock[data-over]').waitFor({timeout:8000});
 assert.match(await proj.locator('#clock').textContent(),/^\+\d+:\d\d$/);
 await proj.screenshot({path:join(dir,'display.png')});
 await writeFile(state,JSON.stringify(status(false)));

 // --- phone 390px: alice's own step, Done with evidence link ---
 const phone=await (await browser.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true})).newPage();
 await phone.goto(alice);
 await phone.locator('h1',{hasText:'Build the arm'}).waitFor();
 assert.equal(await phone.title(),'Participant');
 assert.equal(await phone.locator('.desk-appbar').count(),1,'kit app bar on phone');
 assert.ok(await phone.locator('#done').evaluate(e=>e.classList.contains('desk-btn')),'Done is a kit button');
 assert.ok(await phone.evaluate(()=>document.documentElement.scrollWidth<=390),'no horizontal scroll');
 const done=phone.locator('#done');await done.waitFor();
 const box=await done.boundingBox();assert.ok(box.width>=300&&box.height>=40,'tap-sized Done');
 await phone.locator('#evidence').fill('http://notepad.local/n/42');
 await phone.screenshot({path:join(dir,'phone.png')});
 await done.click();
 await phone.locator('#msg',{hasText:'Marked done'}).waitFor();
 assert.match(await readFile(calls,'utf8'),/tutor done 7 t2 -m http:\/\/notepad\.local\/n\/42/);
 // bob's phone cannot act on alice's step
 const bobPage=await (await browser.newContext({viewport:{width:390,height:844}})).newPage();
 await bobPage.goto(bob);
 await bobPage.locator('h1',{hasText:'Not your turn yet'}).waitFor();
 assert.equal(await bobPage.locator('#done').count(),0);

 // --- owner: next ---
 const t=await (await browser.newContext({viewport:{width:1280,height:720}})).newPage();
 await t.goto(owner);
 await t.locator('.desk-appbar').waitFor();
 await t.locator('#next').click();
 assert.equal(await t.title(),'Facilitator');
 await t.locator('#msg',{hasText:'ok'}).waitFor();
 assert.match(await readFile(calls,'utf8'),/tutor next 7 --json/);
 await t.screenshot({path:join(dir,'owner.png')});

 // --- second host: join from the LAN address with its own data dir ---
 const lan=Object.values(networkInterfaces()).flat().find(i=>i.family==='IPv4'&&!i.internal)?.address;
 if(lan){
  const port=new URL(alice).port,lanURL=`http://${lan}:${port}${new URL(alice).pathname}`;
  const j=spawnSync(exe,['join',lanURL,'--done','-m','http://form.local/f/1'],{env:{...env,XDG_DATA_HOME:join(dir,'other-host')},encoding:'utf8',cwd:tmpdir()});
  assert.equal(j.status,0,j.stderr);
  assert.match(await readFile(calls,'utf8'),/-m http:\/\/form\.local\/f\/1/);
 }else console.log('SKIP LAN join: no non-loopback IPv4');
 console.log('ok browser-test (screenshots in '+dir+')');
}finally{
 await browser?.close();server.kill('SIGTERM');
 if(!process.env.KEEP)setTimeout(()=>rm(dir,{recursive:true,force:true}),500);
}
