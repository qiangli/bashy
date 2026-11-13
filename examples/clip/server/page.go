// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"path/filepath"
	"strings"
)

// Serve is the clip web page: the library as one loopback page. It only
// reads and writes the data directory — rendering is the work command's
// job — so the page runs anywhere the console runs an app.
func Serve(s *Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			jobs, err := s.List()
			if err != nil {
				httpError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, jobs)
		case http.MethodPost:
			var spec Job
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&spec); err != nil {
				httpError(w, http.StatusBadRequest, fmt.Errorf("clip: read request: %v", err))
				return
			}
			j, err := s.Queue(&spec)
			if err != nil {
				httpError(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, j)
		default:
			w.Header().Set("Allow", "GET, POST")
			httpError(w, http.StatusMethodNotAllowed, fmt.Errorf("clip: %s not allowed", r.Method))
		}
	})
	mux.HandleFunc("/media/", func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.URL.Path)
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(s.ClipsDir(), name))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		jobs, err := s.List()
		if err != nil {
			httpError(w, http.StatusInternalServerError, err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, pageHTML(s, jobs))
	})
	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func httpError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintln(w, err)
}

// mediaType picks the player element for a finished clip.
func mediaType(artifact string) string {
	if strings.EqualFold(filepath.Ext(artifact), ".gif") {
		return "image"
	}
	return "video"
}

const pageStyle = `<style>
:root{--bg:#fbfbf9;--fg:#1d1d1b;--muted:#6b6b66;--line:#e4e4df;--link:#1f5fbf;--chip:#eee;--ok:#1a7f37;--run:#9a6700;--bad:#cf222e}
@media (prefers-color-scheme:dark){:root{--bg:#141413;--fg:#ecebe6;--muted:#9a9a93;--line:#2c2c29;--link:#8ab4ff;--chip:#262624;--ok:#3fb950;--run:#d29922;--bad:#f85149}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--fg);font:17px/1.6 system-ui,-apple-system,"Segoe UI",sans-serif}
main{max-width:46rem;margin:0 auto;padding:2rem 1rem 4rem}
h1{font-size:1.6rem;margin:0}header p{margin:.25rem 0 1.5rem;color:var(--muted)}
form{display:flex;flex-wrap:wrap;gap:.5rem;padding:1rem;border:1px solid var(--line);border-radius:8px;margin-bottom:1.5rem}
input,select,button{font:inherit;padding:.35rem .5rem;border:1px solid var(--line);border-radius:6px;background:var(--bg);color:var(--fg)}
input[type=number]{width:6rem}input[type=text]{flex:1;min-width:12rem}
button{cursor:pointer}
table{width:100%;border-collapse:collapse}td,th{padding:.4rem .5rem;border-top:1px solid var(--line);text-align:left;vertical-align:top}
.chip{display:inline-block;padding:0 .5rem;border-radius:1rem;font-size:.85rem;background:var(--chip)}
.chip.done{color:var(--ok)}.chip.running,.chip.queued{color:var(--run)}.chip.failed{color:var(--bad)}
.hint{color:var(--muted);font-size:.9rem}
video,img.clip{max-width:100%;border-radius:8px;background:#000;margin-top:.5rem}
code{background:var(--chip);border-radius:4px;padding:0 .3rem}
</style>`

const pageScript = `<script>
function esc(s){return String(s).replace(/[&<>"']/g,function(c){return{'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]})}
function mediaIsImage(a){return String(a).toLowerCase().slice(-4)=='.gif'}
function row(j){
 var tr=document.createElement('tr');
 function td(x){var e=document.createElement('td');e.innerHTML=x;return e}
 var name=j.name?j.name:j.id;
 var span=j.kind=='screen'?(j.seconds+'s'):(j.start+'–'+j.end+'s');
 var art='';
 if(j.status=='done'&&j.artifact){
  var url='/media/'+j.artifact.split('/').pop();
  art=mediaIsImage(j.artifact)?('<img class="clip" src="'+url+'" alt="clip">'):('<video controls preload="none" src="'+url+'"></video>');
 }
 tr.appendChild(td('<strong>'+esc(name)+'</strong>'+art));
 tr.appendChild(td(esc(j.kind)));
 tr.appendChild(td(esc(span)));
 tr.appendChild(td('<span class="chip '+esc(j.status)+'">'+esc(j.status)+'</span>'+(j.error?('<br><span class="hint">'+esc(j.error)+'</span>'):'')));
 return tr;
}
var kind=document.getElementById('kind'),source=document.getElementById('source'),
    start=document.getElementById('start'),end=document.getElementById('end'),
    seconds=document.getElementById('seconds');
kind.addEventListener('change',function(){
 var screen=kind.value=='screen';
 source.hidden=screen;start.hidden=screen;end.hidden=screen;seconds.hidden=!screen;
 source.placeholder=screen?'':'/absolute/path/to/video.mp4 or scene.cast';
});
document.getElementById('q').addEventListener('submit',function(ev){
 ev.preventDefault();
 var body={kind:kind.value};
 if(kind.value!='screen'){
  body.source=source.value;
  body.start=parseFloat(start.value)||0;
  body.end=parseFloat(end.value)||0;
 }else{
  body.seconds=parseFloat(seconds.value)||0;
 }
 var n=document.getElementById('name').value;if(n)body.name=n;
 fetch('/api/jobs',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)})
  .then(function(){return refresh()});
});
function refresh(){
 return fetch('/api/jobs').then(function(r){return r.json()}).then(function(jobs){
  var tb=document.getElementById('rows');tb.innerHTML='';
  jobs.forEach(function(j){tb.appendChild(row(j))});
  var waiting=jobs.filter(function(j){return j.status=='queued'||j.status=='running'}).length;
  var pending=document.getElementById('pending');
  pending.textContent=waiting?(waiting+' waiting — run: clip work --data '+pending.dataset.dir):'';
 });
}
setInterval(refresh,2000);
</script>`

// pageHTML renders the whole page server-side: the queue table works
// before any script runs; the script only refreshes it.
func pageHTML(s *Store, jobs []*Job) string {
	var b strings.Builder
	b.WriteString(`<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Clips</title>
`)
	b.WriteString(pageStyle)
	b.WriteString(`</head><body><main>
<header><h1>Clips</h1><p>An asynchronous clip library: queue a clip, it renders in the background, the finished clip lands here.</p></header>
<form id="q">
<select id="kind"><option value="trim">cut a video</option><option value="cast">render a terminal scene</option><option value="screen">record the screen</option></select>
<input type="text" id="source" placeholder="/absolute/path/to/video.mp4 or scene.cast">
<input type="number" id="start" min="0" step="any" placeholder="start s">
<input type="number" id="end" min="0" step="any" placeholder="end s">
<input type="number" id="seconds" min="1" max="600" step="any" placeholder="seconds" hidden>
<input type="text" id="name" placeholder="name (optional)">
<button type="submit">Queue clip</button>
</form>
<p class="hint" id="pending" data-dir="`)
	b.WriteString(html.EscapeString(s.Dir()))
	b.WriteString(`"></p>
<table><thead><tr><th>Clip</th><th>Kind</th><th>Span</th><th>Status</th></tr></thead><tbody id="rows">
`)
	for _, j := range jobs {
		b.WriteString(rowHTML(j))
	}
	b.WriteString(`</tbody></table>
<footer style="margin-top:3rem" class="hint">Finished clips stay in the library directory; render with <code>clip work</code>.</footer>
`)
	b.WriteString(pageScript)
	b.WriteString(`
</main></body></html>`)
	return b.String()
}

// rowHTML renders one table row.
func rowHTML(j *Job) string {
	name := j.ID
	if j.Name != "" {
		name = html.EscapeString(j.Name)
	}
	span := fmt.Sprintf("%s–%ss", trimZero(j.Start), trimZero(j.End))
	if j.Kind == KindScreen {
		span = trimZero(j.Seconds) + "s"
	}
	status := fmt.Sprintf(`<span class="chip %s">%s</span>`, html.EscapeString(j.Status), html.EscapeString(j.Status))
	if j.Error != "" {
		status += `<br><span class="hint">` + html.EscapeString(j.Error) + `</span>`
	}
	art := ""
	if j.Status == StatusDone && j.Artifact != "" {
		url := "/media/" + html.EscapeString(filepath.Base(j.Artifact))
		if mediaType(j.Artifact) == "image" {
			art = `<br><img class="clip" src="` + url + `" alt="clip">`
		} else {
			art = `<br><video controls preload="none" src="` + url + `"></video>`
		}
	}
	return fmt.Sprintf(`<tr><td><strong>%s</strong>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
		name, art, html.EscapeString(j.Kind), html.EscapeString(span), status)
}

func trimZero(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}
