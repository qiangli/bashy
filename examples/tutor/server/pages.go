package main

import kit "desk-kit"

const pageJS = `
function h(tag,attrs,...kids){const e=document.createElement(tag);for(const k in attrs||{})e.setAttribute(k,attrs[k]);for(const c of kids)e.append(c);return e}
function fmt(s){const a=Math.abs(s);return (s<0?'+':'')+Math.floor(a/60)+':'+String(a%60).padStart(2,'0')}
let STATE=null,RECV=0,RENDER=null,URL_=null,HDR={};
function now(){return Math.floor((Date.now()-RECV)/1000)}
function badge(kind,label){const b=document.getElementById('statusBadge');b.dataset.status=kind;b.textContent=label}
function say(text,kind){const m=document.getElementById('msg');m.dataset.kind=kind||'ok';m.textContent=text||''}
async function load(){try{const r=await fetch(PFX+URL_,{cache:'no-store',headers:HDR});STATE=await r.json();RECV=Date.now();STATE._ok=r.ok}catch(e){STATE={error:String(e)}}draw()}
function draw(){const root=document.getElementById('root');if(!STATE)return;
 if(STATE._ok===false&&!STATE.current){root.replaceChildren(h('div',{class:'desk-state','data-state':'error',role:'alert'},h('div',{class:'desk-state-title'},STATE.error||'waiting')));badge('failed','offline');return}
 RENDER(root,STATE)}
function start(url,render,headers){URL_=url;RENDER=render;HDR=headers||{};load();setInterval(load,3000);setInterval(draw,1000)}
function safeLink(t){return /^https?:\/\/[^\s]+$/.test(t||'')?h('a',{href:t,target:'_blank',rel:'noopener noreferrer'},t):document.createTextNode(t||'')}
async function post(path,body,headers){const r=await fetch(PFX+path,{method:'POST',headers:{'content-type':'application/json',...(headers||{})},body:JSON.stringify(body||{})});const j=await r.json().catch(()=>({}));if(!r.ok)throw new Error(j.error||r.statusText);return j}
function stepBadges(steps){return h('div',{class:'desk-statuses'},...(steps||[]).map(s=>h('span',{class:'desk-badge','data-status':s.status==='done'?'filled-by-agent':s.status==='doing'?(s.overdue?'failed':'answered'):'pending'},s.index+'. '+s.title)))}
function clockEl(sec,cls){const c=h('div',{class:'desk-h1',id:'clock','aria-live':'off'},fmt(sec));if(sec<0)c.dataset.over='';return c}
function setClock(sec){const c=document.getElementById('clock');if(!c)return false;c.textContent=fmt(sec);if(sec<0)c.dataset.over='';else delete c.dataset.over;return true}
function roleLabel(word,fallback){return word?word[0].toUpperCase()+word.slice(1):fallback}

function renderDisplay(root,st){
 const c=st.current,kids=[];
 if(!c){badge('pending','waiting');kids.push(h('p',{class:'desk-muted'},st.title||'Tutor'),h('h1',{class:'desk-h1'},(st.steps||[]).length&&st.steps.every(s=>s.status==='done')?'All steps done':'Waiting to start'))}
 else{const sec=c.remaining_s-now();badge(sec<0?'failed':'answered',sec<0?'over time':'running');
  kids.push(h('p',{class:'desk-muted'},(st.title||'')+' · step '+c.index+' of '+c.total),h('h1',{class:'desk-h1'},c.title),clockEl(sec,'proj'),
   h('p',{class:'desk-muted'},(sec<0?'Over time · ':'')+(c.owners||[]).join(', ')))}
 kids.push(stepBadges(st.steps));
 root.replaceChildren(...kids)}

function renderParticipant(root,st){
 document.title=roleLabel(st.role||st.roles?.participants?.[0],'Participant');
 const m=st.mine,kids=[h('p',{class:'desk-muted'},document.title+' · '+(st.title||'Tutor')+' · '+st.person)];
 if(m){const sec=m.remaining_s-now();badge(sec<0?'failed':'answered',sec<0?'over time':'your step');
  const old=document.getElementById('evidence');
  if(old&&root.dataset.step===m.id){setClock(sec);return}
  const ev=h('input',{id:'evidence',placeholder:'Link to your work (optional)',type:'url',inputmode:'url',autocomplete:'off'});
  const b=h('button',{class:'desk-btn',id:'done',type:'button'},'Done');
  b.onclick=async()=>{b.disabled=true;try{await post('/s/'+TOKEN+'/done',{evidence:ev.value.trim()});say('Marked done.');await load()}catch(e){say(e.message,'error');b.disabled=false}};
  kids.push(h('div',{class:'desk-panel'},h('p',{class:'desk-muted'},'Step '+m.index+' of '+m.total),h('h1',{class:'desk-h1'},m.title),clockEl(sec),
    h('p',{class:'desk-muted'},m.done_when?'Done when: '+m.done_when:'')),
   h('div',{class:'desk-panel'},h('div',{class:'desk-field'},h('label',{for:'evidence'},'Evidence'),ev),b));
  root.dataset.step=m.id}
 else{badge('pending','waiting');root.dataset.step='';
  kids.push(h('div',{class:'desk-panel'},h('h1',{class:'desk-h1'},st.done_count&&!(st.upcoming||[]).length?'All your steps are done':'Not your turn yet'),
   st.now?h('p',{class:'desk-muted'},'Session is on step '+st.now.index+': '+st.now.title):h('p',{class:'desk-muted'},'Waiting for the session to start.')))}
 if((st.upcoming||[]).length)kids.push(h('div',{class:'desk-panel'},h('p',{class:'desk-muted'},'Coming up'),...st.upcoming.map(u=>h('p',{},u.index+'. '+u.title+' · '+Math.round(u.timebox_s/60)+' min'))));
 root.replaceChildren(...kids)}

function renderOwner(root,st){
 document.title=roleLabel(st.roles?.owner,'Host');
 const c=st.current,kids=[h('p',{class:'desk-muted'},document.title+' · '+(st.title||''))];
 if(c){const sec=c.remaining_s-now();badge(sec<0?'failed':'answered',sec<0?'over time':'running');
  kids.push(h('div',{class:'desk-panel'},h('h1',{class:'desk-h1'},'Step '+c.index+'/'+c.total+': '+c.title),clockEl(sec),h('p',{class:'desk-muted'},(c.owners||[]).join(', '))))}
 else badge('pending','waiting');
 const act=(path,body)=>async()=>{try{await post('/api/owner'+path,body,{'X-Facilitator-Key':KEY});say('ok')}catch(e){say(e.message,'error')}await load()};
 const nb=h('button',{class:'desk-btn',id:'next',type:'button'},'Next step');nb.onclick=act('/next');
 const eb=h('button',{class:'desk-btn secondary',id:'extend',type:'button'},'Extend 5 min');eb.onclick=act('/extend',{by:'5m'});
 kids.push(h('div',{class:'desk-row'},nb,eb));
 const rows=(st.steps||[]).map(s=>h('tr',{},h('td',{},String(s.index)),h('td',{},s.title),h('td',{},s.owner),h('td',{},s.status+(s.overdue?' (over)':'')),h('td',{},safeLink(typeof s.evidence==='string'?s.evidence:''))));
 kids.push(h('div',{class:'desk-panel'},h('table',{class:'desk-table'},h('tr',{},...['#','Step','Owner','Status','Evidence'].map(x=>h('th',{},x))),...rows)));
 root.replaceChildren(...kids)}
`

const (
	displayMain     = `<main class="desk-main"><div id="root"><p class="desk-muted">Loading…</p></div></main>`
	ownerMain       = `<main class="desk-main"><div id="root"><p class="desk-muted">Loading…</p></div></main>`
	participantMain = `<main class="desk-main"><div id="root"><p class="desk-muted">Loading…</p></div></main>`
)

func page(title, id, main, script string) []byte {
	header := `<div class="desk-item-header"><span class="desk-id">` + id + `</span>` +
		`<span class="desk-badge" id="statusBadge" data-status="pending">waiting</span></div>`
	return kit.Page("tutor", title, kit.AppBar("tutor", "", header, "")+
		main+
		`<div id="msg" class="desk-alert" data-kind="ok" data-desk-alert role="status" aria-live="polite"></div>`+
		`<script>`+pageJS+script+`</script>`)
}
