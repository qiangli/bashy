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
 :root{--chart-ink:#24334b;--chart-surface:#fff;--chart-accent:#286fdb}
 @media(prefers-color-scheme:dark){:root{--chart-ink:#e1e9f5;--chart-surface:#192233;--chart-accent:#79adff}}
 [data-theme="dark"]{--chart-ink:#e1e9f5;--chart-surface:#192233;--chart-accent:#79adff}
 [data-theme="light"]{--chart-ink:#24334b;--chart-surface:#fff;--chart-accent:#286fdb}
 main{max-width:1160px;margin:auto;padding:28px}.chart-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,420px),1fr));gap:18px}.chart-widget{min-width:0;box-shadow:0 3px 18px #0001;border-radius:16px}button{cursor:pointer}h1{font-size:30px}.chart-intro{max-width:680px}.desk-tools{flex-wrap:wrap}#message:empty{display:none}textarea{min-height:150px}
 </style><main>` + kit.ItemHeader("chart", kit.StatusPending, a.baseURL+"/") + `
 <div class="desk-tools"><a href="` + esc(a.baseURL) + `/">New chart</a><button type="button" data-desk-theme>Toggle theme</button><button id="share" hidden>Share</button><button id="revoke" hidden>Revoke sharing</button><button id="close" hidden>Close</button></div>
 <p id="message" role="status" data-desk-alert></p><div id="charts"></div>
 <form id="unlock" hidden>` + kit.TextField("key", "Owner key", "password", "", "") + `<button>View chart</button></form>
 <form id="create" class="chart-intro"><h1>Numbers, at a glance.</h1><p class="desk-muted">Paste JSON, CSV, NDJSON or OTLP metrics. One page, saved locally.</p>` +
		kit.TextField("title", "Title", "text", "Quick stats", "") + kit.TextField("input", "Data", "textarea", "[12,18,15,24,31]", "") + `
 <label>Widget <select name="widget"><option value="auto">Auto</option>`
	for _, k := range widgetKinds {
		body += `<option>` + k + `</option>`
	}
	body += `</select></label> <button>Create chart</button></form></main><script>
 const base = location.pathname.replace(/\/$/, '');
 const params=new URLSearchParams(location.search);let id=params.get('id')||'';
 const message=document.querySelector('#message'),create=document.querySelector('#create'),unlock=document.querySelector('#unlock');
 let key=id?sessionStorage.getItem('chart-key-'+id)||'':'';
 async function call(action,extra={}){const res=await fetch(base+'/api/action',{method:'POST',headers:{'Content-Type':'application/json','X-Facilitator-Key':key},body:JSON.stringify({action,id,...extra})});const v=await res.json();if(!res.ok)throw Error(v.error||'Request failed');return v}
 function fail(e){message.textContent=e.message}
 let timer;
 async function view(){clearTimeout(timer);try{const v=await call('get');document.querySelector('#charts').innerHTML=v.html;document.querySelector('.desk-id').textContent=v.id;const badge=document.querySelector('.desk-badge');badge.textContent=v.status;badge.dataset.status=v.status;document.querySelector('[data-desk-copy]').dataset.copy=v.url;message.textContent=v.refreshError||'';unlock.hidden=true;for(const action of ['share','revoke','close'])document.querySelector('#'+action).hidden=!key;if(v.refreshMS>0)timer=setTimeout(view,v.refreshMS)}catch(e){fail(e);unlock.hidden=false}}
 create.onsubmit=async e=>{e.preventDefault();try{const f=new FormData(create);const v=await call('new',{title:f.get('title'),chart:{input:f.get('input'),widget:f.get('widget'),title:f.get('title')}});sessionStorage.setItem('chart-key-'+v.id,v.ownerKey);location.href=v.url}catch(e){fail(e)}};
 unlock.onsubmit=e=>{e.preventDefault();key=new FormData(unlock).get('key');sessionStorage.setItem('chart-key-'+id,key);view()};
 document.querySelector('#share').onclick=async()=>{try{const v=await call('share');message.textContent='Single-use invite: '+v.inviteURL}catch(e){fail(e)}};
 document.querySelector('#revoke').onclick=async()=>{try{await call('revoke');message.textContent='Sharing revoked.'}catch(e){fail(e)}};
 document.querySelector('#close').onclick=async()=>{try{await call('close');view()}catch(e){fail(e)}};
 (async()=>{if(id){create.hidden=true;const invite=new URLSearchParams(location.hash.slice(1)).get('invite');if(invite){try{await call('join',{invite});history.replaceState(null,'',location.pathname+location.search)}catch(e){fail(e);return}}await view()}})();
 </script>`
	return kit.Shell("Chart · bashy desk", body)
}
