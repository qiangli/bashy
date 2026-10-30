package main

import (
	"net/http"
	"time"

	kit "desk-kit"
)

// Called under a.mu. Refresh is demand-driven and bounded; snapshots never
// access their former source. A failed refresh preserves the last good data.
func (a *app) viewData(it *item) map[string]any {
	refreshError := ""
	d, _ := time.ParseDuration(it.Chart.Refresh)
	if d > 0 && it.Status != kit.StatusClosed && time.Since(it.Chart.Updated) >= d {
		next := cloneItem(it)
		if e := prepareConfig(&next.Chart); e != nil {
			refreshError = e.Error()
		} else if e = a.saveLocked(&next); e != nil {
			refreshError = "Could not save refreshed data"
		} else {
			it = &next
		}
	}
	rendered := ""
	if it.Chart.Input != "" {
		var e error
		rendered, e = renderCharts(it.Chart)
		if e != nil {
			refreshError = e.Error()
		}
	}
	if it.Status == kit.StatusClosed {
		d = 0
	}
	return map[string]any{"id": it.ID, "url": a.urlFor(it.ID), "status": it.Status, "title": it.Title, "body": it.Body, "fields": it.Fields, "revision": it.Revision, "chart": it.Chart, "html": rendered, "refreshMS": d.Milliseconds(), "refreshError": refreshError}
}
func (a *app) page(r *http.Request, me string) []byte {
	body := `<style>
 .chart-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,420px),1fr));gap:var(--desk-space-4)}.chart-widget{min-width:0;box-shadow:var(--desk-shadow-1);border:1px solid var(--desk-line);border-radius:var(--desk-radius-lg)}.chart-intro{max-width:680px}.chart-page h1{font-size:var(--desk-text-2xl);letter-spacing:-.02em;margin:0 0 var(--desk-space-2)}.chart-intro textarea{min-height:150px;font-family:var(--desk-font-mono)}.chart-actions{margin-bottom:var(--desk-space-4)}
 </style>` + kit.AppBar("chart", a.baseURL+"/", kit.ItemHeader("chart", kit.StatusPending, a.baseURL+"/"), "") + `<main class="desk-main">
 <div class="desk-tools chart-actions"><a class="desk-btn secondary" href="` + esc(a.baseURL) + `/">New chart</a><button type="button" class="desk-btn secondary" id="share" hidden>Share</button><button type="button" class="desk-btn secondary" id="revoke" hidden>Revoke sharing</button><button type="button" class="desk-btn danger" id="close" hidden>Close</button></div>
 <p id="message" role="status" class="desk-alert" data-desk-alert></p><div id="charts"></div>
 <form id="unlock" class="desk-panel" hidden>` + kit.TextField("key", "Owner key", "password", "", "") + `<button class="desk-btn">View chart</button></form>
 <form id="create" class="chart-intro desk-panel"><h1 class="desk-h1">Numbers, at a glance.</h1><p class="desk-muted">Paste JSON, CSV, NDJSON or OTLP metrics. One page, saved locally.</p>` +
		kit.TextField("title", "Title", "text", "Quick stats", "") + kit.TextField("input", "Data", "textarea", "[12,18,15,24,31]", "") + `
 <div class="desk-field"><label for="widget">Widget</label><select id="widget" name="widget"><option value="auto">Auto</option>`
	for _, k := range widgetKinds {
		body += `<option>` + k + `</option>`
	}
	body += `</select></div><div class="desk-row"><button class="desk-btn" data-desk-shortcut="submit" title="Create (⌘/Ctrl+Enter)">Create chart</button></div></form></main><script>
 const base = location.pathname.replace(/\/$/, '');
 const params=new URLSearchParams(location.search);let id=params.get('id')||'';
 const message=document.querySelector('#message'),create=document.querySelector('#create'),unlock=document.querySelector('#unlock');
 let key=id?sessionStorage.getItem('chart-key-'+id)||'':'';
 async function call(action,extra={}){const res=await fetch(base+'/api/action',{method:'POST',headers:{'Content-Type':'application/json','X-Facilitator-Key':key},body:JSON.stringify({action,id,...extra})});const v=await res.json();if(!res.ok)throw Error(v.error?.message||'Request failed');return v}
 function fail(e){message.textContent=e.message;message.dataset.kind='error'}
 let timer;
 async function view(){clearTimeout(timer);const charts=document.querySelector('#charts');if(!charts.firstChild)charts.innerHTML='<div class="desk-state" data-state="loading"><div class="desk-state-title">Loading chart</div></div>';try{const v=await call('show');document.querySelector('#charts').innerHTML=v.html;document.querySelector('.desk-id').textContent=v.id;const badge=document.querySelector('.desk-badge');badge.textContent=v.status;badge.dataset.status=v.status;document.querySelector('[data-desk-copy]').dataset.copy=v.url;message.textContent=v.refreshError||'';message.dataset.kind=v.refreshError?'error':'';unlock.hidden=true;for(const action of ['share','revoke','close'])document.querySelector('#'+action).hidden=!key;if(v.refreshMS>0)timer=setTimeout(view,v.refreshMS)}catch(e){const st=charts.querySelector('.desk-state');if(st)st.remove();fail(e);unlock.hidden=false}}
 create.onsubmit=async e=>{e.preventDefault();try{const f=new FormData(create);const v=await call('add',{title:f.get('title'),chart:{input:f.get('input'),widget:f.get('widget'),title:f.get('title')}});sessionStorage.setItem('chart-key-'+v.id,v.ownerKey);location.href=v.url}catch(e){fail(e)}};
 unlock.onsubmit=e=>{e.preventDefault();key=new FormData(unlock).get('key');sessionStorage.setItem('chart-key-'+id,key);view()};
 document.querySelector('#share').onclick=async()=>{try{const v=await call('share');message.textContent='Single-use invite: '+v.inviteURL;message.dataset.kind='ok'}catch(e){fail(e)}};
 document.querySelector('#revoke').onclick=async()=>{try{await call('revoke');message.textContent='Sharing revoked.';message.dataset.kind='ok';deskToast('Sharing revoked.','ok')}catch(e){fail(e)}};
 document.querySelector('#close').onclick=async()=>{try{await call('close');view()}catch(e){fail(e)}};
 (async()=>{if(id){create.hidden=true;const invite=new URLSearchParams(location.hash.slice(1)).get('invite');if(invite){try{await call('join',{invite});history.replaceState(null,'',location.pathname+location.search)}catch(e){fail(e);return}}await view()}})();
 </script>`
	return kit.Page("chart", "Chart · bashy desk", body)
}
