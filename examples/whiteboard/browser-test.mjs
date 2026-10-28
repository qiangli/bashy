// Optional real-browser check: PLAYWRIGHT_MODULE=/path/to/playwright/index.mjs
// WHITEBOARD_BIN=/path/to/whiteboard node browser-test.mjs
import assert from 'node:assert/strict';
import {mkdtemp, readFile, rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {spawn} from 'node:child_process';
import {hold} from '../../tests/collaboration-e2e/barrier.mjs';
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE||'playwright');
const PNG_SIG=Buffer.from([0x89,0x50,0x4e,0x47,0x0d,0x0a,0x1a,0x0a]);

async function startServer(dir,args){
 const server=spawn(process.env.WHITEBOARD_BIN,['-data',dir,'-addr','127.0.0.1:0',...args]);
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
async function draw(page,fx,fy,tx,ty){
 await page.locator('#canvas').scrollIntoViewIfNeeded();
 const box=await page.locator('#canvas').boundingBox();
 const X=f=>box.x+box.width*f,Y=f=>box.y+box.height*f;
 await page.mouse.move(X(fx),Y(fy));await page.mouse.down();
 for(let i=1;i<=12;i++)await page.mouse.move(X(fx+(tx-fx)*i/12),Y(fy+(ty-fy)*i/12),{steps:2});
 await page.mouse.up();
}
async function synced(page,strokes,notes){
 await page.waitForFunction((exp)=>window.__whiteboard&&window.__whiteboard.strokes.length===exp[0]&&window.__whiteboard.notes.length===exp[1], [strokes,notes], {timeout:15000});
}
async function api(base,path,body,headers){
 const r=await fetch(base+path,{method:body?'POST':'GET',headers:{'Content-Type':'application/json',...(headers||{})},body:body?JSON.stringify(body):undefined});
 return r;
}
async function facKeyOf(page,board){
 return await page.evaluate((b)=>sessionStorage.getItem('whiteboard-facilitator:'+b), board);
}

const dir=await mkdtemp(join(tmpdir(),'whiteboard-browser-'));
let browser,first,second;
try {
 // --- standalone: no prefix, brainstorm/Matrix absent ---
 first=await startServer(dir,[]);
 browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||undefined});
 const facCtx=await browser.newContext(),aCtx=await browser.newContext(),bCtx=await browser.newContext();
 const fac=await facCtx.newPage(),a=await aCtx.newPage(),b=await bCtx.newPage();
 // Facilitator creates a named private board, then mints an invite.
 await fac.goto(first.facilitator);
 await fac.locator('#connection').filter({hasText:'Connected'}).waitFor();
 await fac.locator('#boardInput').fill('e2e');
 await fac.locator('#createBoard').click();
 await fac.locator('#connection').filter({hasText:'e2e'}).waitFor();
 await fac.locator('#inviteBtn').click();
 await fac.locator('#inviteLink').filter({hasText:'invite='}).waitFor();
 const inviteLink=(await fac.locator('#inviteLink').textContent()).match(/http:\/\/[^\s]+/)?.[0];
 assert.ok(inviteLink);
 const participantLink=(await fac.locator('#participantLink').textContent()).match(/http:\/\/[^\s]+/)?.[0];
 assert.ok(participantLink);
 // A guessed URL without an invite is denied on every path.
 const stranger=await browser.newContext().then(c=>c.newPage());
 await stranger.goto(participantLink);
 await stranger.locator('#connection').filter({hasText:'Private board'}).waitFor();
 assert.equal((await api(first.origin,'/api/state?board=e2e')).status,403);
 assert.equal((await api(first.origin,'/api/export?board=e2e&format=json')).status,403);
 assert.equal((await api(first.origin,'/api/export?board=e2e&format=png')).status,403);
 assert.equal((await api(first.origin,'/api/action',{action:'stroke',board:'e2e',points:[{x:1,y:1},{x:2,y:2}],color:'#000000',width:2})).status,403);
 await stranger.close();
 // Invited browsers join through the invite link and sync.
 await Promise.all([a.goto(inviteLink),b.goto(inviteLink)]);
 await a.locator('#connection').filter({hasText:'Connected'}).waitFor();
 await b.locator('#connection').filter({hasText:'Connected'}).waitFor();
 // Three browsers draw and pin; every member sees every contribution.
 await draw(a,0.1,0.1,0.5,0.4);
 await synced(a,1,0);
 await b.locator('#noteText').fill('Shipped notes survive refresh <img src=x>');
 await b.locator('#addNote').click();
 await synced(b,1,1);
 await draw(fac,0.6,0.6,0.9,0.8);
 for(const p of [fac,a,b])await synced(p,2,1);
 assert.equal(await b.locator('.note img').count(),0);
 // Reconnect keeps state for members.
 await b.reload();
 await b.locator('#connection').filter({hasText:'Connected'}).waitFor();
 await synced(b,2,1);
 // Revocation: a joined latecomer loses reads/writes immediately.
 const lateCtx=await browser.newContext();
 const late=await lateCtx.newPage();
 await late.goto(inviteLink);
 await late.locator('#connection').filter({hasText:'Connected'}).waitFor();
 await synced(late,2,1);
 const lateCookies=await lateCtx.cookies();
 const lateId=(lateCookies.find(c=>c.name==='whiteboard-participant')||{}).value;
 assert.ok(lateId);
 const e2eKey=await facKeyOf(fac,'e2e');
 await fac.evaluate(async ({board,key,mid})=>{
  const r=await fetch(new URL('api/action',document.baseURI).toString(),{method:'POST',headers:{'Content-Type':'application/json','X-Facilitator-Key':key},body:JSON.stringify({action:'revoke',board,id:mid})});
  if(!r.ok)throw new Error('revoke failed '+r.status);
 }, {board:'e2e',key:e2eKey,mid:lateId});
 await late.waitForFunction(()=>document.getElementById('connection').textContent.includes('Private board')||document.getElementById('message').textContent.includes('private'), null, {timeout:15000}).catch(()=>{});
 const lateState=await late.evaluate(async ()=>{
  const r=await fetch(new URL('api/state?board=e2e',document.baseURI).toString(),{cache:'no-store'});
  return r.status;
 });
 assert.equal(lateState,403);
 await late.close();
 // Lock: participants frozen, facilitator still draws, server enforces it.
 await fac.locator('#lockBtn').click();
 await a.waitForFunction(()=>window.__whiteboard&&window.__whiteboard.locked===true);
 await draw(a,0.2,0.7,0.4,0.9);
 await a.locator('#message').filter({hasText:'locked'}).waitFor();
 assert.equal((await a.evaluate(()=>window.__whiteboard.strokes)).length,2);
 const denied=await api(first.origin,'/api/action',{action:'stroke',board:'e2e',points:[{x:1,y:1},{x:2,y:2}],color:'#000000',width:2});
 assert.equal(denied.status,403);
 await draw(fac,0.55,0.75,0.85,0.9);
 await synced(fac,3,1);
 await fac.locator('#lockBtn').click();
 await a.waitForFunction(()=>window.__whiteboard&&window.__whiteboard.locked===false);
 // Restart with the same data: members and invites resume without re-joining.
 await stopServer(first.server);
 first=await startServer(dir,[]);
 const saved=JSON.parse(await readFile(join(dir,'boards','e2e.json'),'utf8'));
 const savedKey=saved.facilitator;
 const savedInvite=(saved.invites&&Object.keys(saved.invites)[0])||inviteLink.match(/invite=([0-9a-f]+)/)?.[1];
 assert.ok(savedKey&&savedInvite);
 await fac.goto(first.origin+'/?board=e2e#facilitator='+savedKey);
 await fac.locator('#connection').filter({hasText:'Connected'}).waitFor();
 await synced(fac,3,1);
 assert.equal(await fac.locator('#facilitatorPanel').isHidden(),false);
 await a.goto(first.origin+'/?board=e2e');await b.goto(first.origin+'/?board=e2e');
 await a.locator('#connection').filter({hasText:'Connected'}).waitFor();
 await b.locator('#connection').filter({hasText:'Connected'}).waitFor();
 await synced(a,3,1);
 const exp=await api(first.origin,'/api/export?board=e2e&format=json',null,{'X-Facilitator-Key':savedKey});
 assert.equal(exp.status,200);
 const exported=await exp.json();
 assert.equal(exported.strokes.length,3);assert.equal(exported.notes.length,1);
 assert.ok(!JSON.stringify(exported).includes('owner')&&!JSON.stringify(exported).includes('facilitator')&&!JSON.stringify(exported).includes('invite'));
 const png=await api(first.origin,'/api/export?board=e2e&format=png',null,{'X-Facilitator-Key':savedKey});
 assert.equal(png.status,200);assert.equal(png.headers.get('content-type'),'image/png');
 const bytes=Buffer.from(await png.arrayBuffer());
 assert.ok(bytes.subarray(0,8).equals(PNG_SIG)&&bytes.length>2000);
 assert.equal((await api(first.origin,'/api/export?board=e2e&format=json')).status,403);
 // Clear is facilitator-only and server-enforced.
 const clearDenied=await api(first.origin,'/api/action',{action:'clear',board:'e2e'});
 assert.equal(clearDenied.status,403);
 assert.ok(await a.locator('#facilitatorPanel').isHidden());
 await a.goto(first.origin+'/?board=e2e');await b.goto(first.origin+'/?board=e2e');
 fac.on('dialog',d=>d.accept());
 await fac.locator('#clearBtn').click();
 for(const p of [fac,a,b])await synced(p,0,0);
 const stored=JSON.parse(await readFile(join(dir,'boards','e2e.json'),'utf8'));
 assert.equal(stored.strokes.length,0);assert.equal(stored.notes.length,0);
 await fac.screenshot({path:'/tmp/whiteboard-browser-standalone.png',fullPage:true});
 await stopServer(first.server);
 // --- same data under a sub-path prefix, invite link included ---
 second=await startServer(dir,['-prefix','/wb']);
 const persistedInvite=(JSON.parse(await readFile(join(dir,'boards','e2e.json'),'utf8')).invites&&Object.keys(JSON.parse(await readFile(join(dir,'boards','e2e.json'),'utf8')).invites)[0])||savedInvite;
 const pCtx=await browser.newContext(),qCtx=await browser.newContext();
 const p=await pCtx.newPage(),q=await qCtx.newPage();
 await p.goto(second.origin+'/wb/?board=e2e&invite='+persistedInvite);
 await p.locator('#connection').filter({hasText:'Connected'}).waitFor();
 await q.goto(second.origin+'/wb/?board=e2e&invite='+persistedInvite);
 await q.locator('#connection').filter({hasText:'Connected'}).waitFor();
 await draw(p,0.1,0.2,0.4,0.5);
 await synced(q,1,0);
 assert.equal((await api(second.origin,'/api/state?board=e2e')).status,404);
 assert.equal((await api(second.origin,'/wb/api/state?board=e2e')).status,403);
 const pq=await p.evaluate(async ()=>{
  const r=await fetch(new URL('api/state?board=e2e',document.baseURI).toString(),{cache:'no-store'});
  return r.status;
 });
 assert.equal(pq,200);
 const ppng=await api(second.origin,'/wb/api/export?board=e2e&format=png',null,{});
 assert.equal(ppng.status,403);
 await q.screenshot({path:'/tmp/whiteboard-browser-prefix.png',fullPage:true});
 await hold('whiteboard', second.origin, '/wb/api/state?board=main');
 console.log('PASS: invite-only named boards, 3-browser draw/note sync, guessed-URL denial, revocation, reconnect, lock/clear permission, restart persistence, valid PNG/JSON exports, standalone + prefix paths');
} finally {await browser?.close();if(first?.server&&!first.server.killed&&first.server.exitCode===null)try{await stopServer(first.server)}catch{}if(second)try{await stopServer(second.server)}catch{}await rm(dir,{recursive:true,force:true})}
