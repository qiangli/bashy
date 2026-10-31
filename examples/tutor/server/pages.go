package main

const pageCSS = `
body{margin:0}main{max-width:var(--desk-page-width);margin:0 auto;padding:var(--desk-space-5)}
button{border:1px solid transparent;border-radius:var(--desk-radius-md);padding:8px 16px;background:var(--desk-accent);color:var(--desk-accent-ink);cursor:pointer;font-weight:var(--desk-weight-strong);font-size:var(--desk-text-sm)}
button:disabled{opacity:.55;cursor:default}button.secondary{background:var(--desk-panel);color:var(--desk-ink);border-color:var(--desk-line-strong)}
input{border:1px solid var(--desk-line-strong);border-radius:var(--desk-radius-md);padding:10px;background:var(--desk-panel);color:var(--desk-ink);width:100%;box-sizing:border-box;font-size:16px}
.muted{color:var(--desk-muted);font-size:var(--desk-text-sm)}
.panel{background:var(--desk-panel);border:1px solid var(--desk-line);border-radius:var(--desk-radius-lg);padding:var(--desk-space-4) var(--desk-space-5);margin-bottom:var(--desk-space-4);box-shadow:var(--desk-shadow-1)}
.eyebrow{letter-spacing:.13em;text-transform:uppercase;color:var(--desk-muted);font-size:var(--desk-text-xs);font-weight:700}
.clock{font-variant-numeric:tabular-nums;font-weight:800;letter-spacing:-.03em;line-height:1}
.over{color:var(--desk-danger-ink)}.clock.over{color:var(--desk-danger-ink)}
.chips{display:flex;gap:var(--desk-space-2);flex-wrap:wrap}.chip{padding:3px 12px;border-radius:var(--desk-radius-pill);background:var(--desk-subtle);color:var(--desk-muted);font-size:var(--desk-text-sm)}
.chip.doing{background:var(--desk-accent);color:var(--desk-accent-ink);font-weight:var(--desk-weight-strong)}.chip.done{background:var(--desk-ok-bg);color:var(--desk-ok-ink)}.chip.over{background:var(--desk-danger-bg);color:var(--desk-danger-ink)}
.proj{text-align:center;padding:2vh 4vw}.proj .clock{font-size:clamp(5rem,26vw,22rem);margin:2vh 0}.proj h1{font-size:clamp(1.6rem,5vw,4rem);margin:1vh 0;letter-spacing:-.02em}
.proj .owners{font-size:clamp(1.1rem,2.6vw,2rem);color:var(--desk-muted)}.proj .chips{justify-content:center;margin-top:3vh}
.stu .clock{font-size:clamp(3.5rem,22vw,6rem);margin:var(--desk-space-3) 0}.stu h1{font-size:var(--desk-text-xl);margin:var(--desk-space-1) 0}
.stu main{padding:var(--desk-space-3)}.stu button.big{width:100%;padding:14px;font-size:1.05rem}
.row{display:flex;gap:var(--desk-space-2);align-items:center;flex-wrap:wrap}
table{width:100%;border-collapse:collapse}td,th{padding:6px 8px;border-bottom:1px solid var(--desk-line);text-align:left;font-size:var(--desk-text-sm)}
@media(max-width:650px){main{padding:var(--desk-space-3)}.panel{padding:var(--desk-space-3)}td.wide{display:none}}
`

const commonJS = `
function h(tag,attrs,...kids){const e=document.createElement(tag);for(const k in attrs||{}){if(k==='class')e.className=attrs[k];else e.setAttribute(k,attrs[k])}for(const c of kids)e.append(c);return e}
function fmt(s){const a=Math.abs(s);return (s<0?'+':'')+Math.floor(a/60)+':'+String(a%60).padStart(2,'0')}
let STATE=null,RECV=0,RENDER=null,URL_=null;
function now(){return Math.floor((Date.now()-RECV)/1000)}
function remaining(c){return c.remaining_s-now()}
async function load(){try{const r=await fetch(URL_,{cache:'no-store'});STATE=await r.json();RECV=Date.now();STATE._ok=r.ok}catch(e){STATE={error:String(e)}}draw()}
function draw(){const root=document.getElementById('root');if(!STATE){return}if(STATE._ok===false&&!STATE.current){root.replaceChildren(h('p',{class:'muted'},STATE.error||'waiting'));return}RENDER(root,STATE)}
function start(url,render){URL_=url;RENDER=render;load();setInterval(load,3000);setInterval(draw,1000)}
function safeLink(t){return /^https?:\/\/[^\s]+$/.test(t||'')?h('a',{href:t,target:'_blank',rel:'noopener noreferrer'},t):document.createTextNode(t||'')}
async function post(path,body){const r=await fetch(path,{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify(body||{})});const j=await r.json().catch(()=>({}));if(!r.ok)throw new Error(j.error||r.statusText);return j}
function chips(steps){return h('div',{class:'chips'},...(steps||[]).map(s=>h('span',{class:'chip '+(s.status==='done'?'done':s.status==='doing'?(s.overdue?'over':'doing'):'')},s.index+'. '+s.title)))}
function clockEl(sec,cls){return h('div',{class:'clock '+(sec<0?'over':'')+(cls||''),id:'clock','aria-live':'off'},fmt(sec))}

function renderDisplay(root,st){
 const c=st.current,kids=[];
 if(!c){kids.push(h('p',{class:'eyebrow'},st.title||'Tutor'),h('h1',{},(st.steps||[]).length&&st.steps.every(s=>s.status==='done')?'All steps done':'Waiting to start'))}
 else{const sec=remaining(c);
  kids.push(h('p',{class:'eyebrow'},(st.title||'')+' · step '+c.index+' of '+c.total),h('h1',{},c.title),clockEl(sec),
   h('div',{class:'owners'},(sec<0?'Over time · ':'')+(c.owners||[]).join(', ')))}
 kids.push(chips(st.steps));
 root.replaceChildren(...kids)}

function roleLabel(word,fallback){return word?word[0].toUpperCase()+word.slice(1):fallback}
function renderParticipant(root,st){
 document.title=roleLabel(st.role||st.roles?.participants?.[0],"Participant");
 const m=st.mine,kids=[h('p',{class:'eyebrow'},document.title+' · '+(st.title||'Tutor')+' · '+st.person)];
 if(m){const sec=m.remaining_s-now();
  kids.push(h('div',{class:'panel'},h('p',{class:'eyebrow'},'Step '+m.index+' of '+m.total),h('h1',{},m.title),clockEl(sec),
    h('p',{class:'muted'},m.done_when?'Done when: '+m.done_when:'')));
  const ev=h('input',{id:'evidence',placeholder:'Link to your work (optional)',type:'url',inputmode:'url',autocomplete:'off'});
  const msg=h('p',{class:'muted',id:'msg','aria-live':'polite'});
  const b=h('button',{class:'big',id:'done'},'Done');
  b.onclick=async()=>{b.disabled=true;try{await post(BASE+'/done',{evidence:ev.value.trim()});msg.textContent='Marked done.';await load()}catch(e){msg.textContent=e.message;b.disabled=false}};
  kids.push(h('div',{class:'panel'},h('label',{for:'evidence'},'Evidence'),ev,h('p'),b,msg))}
 else{kids.push(h('div',{class:'panel'},h('h1',{},st.done_count&&!(st.upcoming||[]).length?'All your steps are done':'Not your turn yet'),
   st.now?h('p',{class:'muted'},'Session is on step '+st.now.index+': '+st.now.title):h('p',{class:'muted'},'Waiting for the session to start.')))}
 if((st.upcoming||[]).length)kids.push(h('div',{class:'panel'},h('p',{class:'eyebrow'},'Coming up'),...st.upcoming.map(u=>h('p',{},u.index+'. '+u.title+' · '+Math.round(u.timebox_s/60)+' min'))));
 const old=document.getElementById('evidence');
 if(old&&m&&root.dataset.step===m.id){const ck=document.getElementById('clock');ck.textContent=fmt(m.remaining_s-now());ck.className='clock '+(m.remaining_s-now()<0?'over':'');return}
 root.dataset.step=m?m.id:'';root.replaceChildren(...kids)}

function renderOwner(root,st){
 document.title=roleLabel(st.roles?.owner,"Host");
 const c=st.current,kids=[h('p',{class:'eyebrow'},document.title+' · '+(st.title||''))];
 const msg=h('p',{class:'muted',id:'msg','aria-live':'polite'},window.LASTMSG||'');
 const act=(path,body)=>async()=>{try{await post(BASE+path,body);window.LASTMSG='ok'}catch(e){window.LASTMSG=e.message}await load()};
 if(c){const sec=remaining(c);
  kids.push(h('div',{class:'panel'},h('h1',{},'Step '+c.index+'/'+c.total+': '+c.title),h('div',{class:'clock '+(sec<0?'over':''),id:'clock',style:'font-size:3rem'},fmt(sec)),
   h('p',{class:'muted'},(c.owners||[]).join(', '))))}
 const nb=h('button',{id:'next'},'Next step');nb.onclick=act('/next');
 const eb=h('button',{class:'secondary',id:'extend'},'Extend 5 min');eb.onclick=act('/extend',{by:'5m'});
 kids.push(h('div',{class:'row'},nb,eb),msg);
 const rows=(st.steps||[]).map(s=>h('tr',{},h('td',{},String(s.index)),h('td',{},s.title),h('td',{},s.owner),h('td',{class:s.overdue?'over':''},s.status+(s.overdue?' (over)':'')),h('td',{class:'wide'},safeLink(typeof s.evidence==='string'?s.evidence:''))));
 kids.push(h('div',{class:'panel'},h('table',{},h('tr',{},...['#','Step','Owner','Status','Evidence'].map(x=>h('th',{},x))),...rows)));
 root.replaceChildren(...kids)}
`

const displayBody = `<main class="proj"><div id="root"><p class="muted">Loading…</p></div></main>`
const ownerBody = `<main><div id="root"><p class="muted">Loading…</p></div></main>`
const participantBody = `<div class="stu"><main><div id="root"><p class="muted">Loading…</p></div></main></div>`
