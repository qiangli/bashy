package main

import (
	"fmt"
	"html"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	kit "desk-kit"
	"desk-kit/render"
)

// The viewer's own CSS: layout only, every colour and font from kit tokens.
const viewerCSS = `<style>
.vw-nav{display:flex;flex-wrap:wrap;gap:var(--desk-space-2);align-items:center;justify-content:space-between;margin-bottom:var(--desk-space-4)}
.vw-nav .desk-row{margin:0}
.vw-nav form{display:flex;gap:var(--desk-space-2);align-items:end}
.vw-nav .desk-field{margin:0;min-width:10em}
.vw-pos,.vw-meta{color:var(--desk-muted);font-size:var(--desk-text-sm)}
.vw-meta{margin:0 0 var(--desk-space-3)}
a.desk-btn{text-decoration:none;display:inline-block}
.desk-btn[aria-disabled="true"]{opacity:.45;pointer-events:none}
.vw-changes{margin-bottom:var(--desk-space-4)}
.vw-block[data-changed]{border-left:3px solid var(--desk-accent);background:var(--desk-subtle);padding:var(--desk-space-1) var(--desk-space-3);margin:0 0 .8em calc(-1 * var(--desk-space-3) - 3px);border-radius:0 var(--desk-radius-sm) var(--desk-radius-sm) 0}
.vw-block[data-changed] > :last-child{margin-bottom:0}
.vw-page + .vw-page{border-top:1px solid var(--desk-line);margin-top:var(--desk-space-6);padding-top:var(--desk-space-5)}
.vw-index{width:100%}
@media print{.desk-appbar,.vw-nav,.vw-actions,.desk-footer,.vw-changes{display:none}.desk-main{max-width:none;padding:0}.vw-page{break-before:page}.vw-page:first-of-type{break-before:auto}}
</style>`

// renderedPage is one page turned into HTML, with its change summary.
type renderedPage struct {
	HTML    string
	Changed int // blocks new or changed since the previous page
	Blocks  int
	Removed int // previous blocks no longer present
	HasPrev bool
	Prev    int // index of the compared page when HasPrev
}

func (a *app) renderOptions(b *book, p page, inline bool) render.Options {
	return render.Options{
		AssetDir: assetDir(a.dir, b.ID, p.N),
		AssetURL: a.prefix + "/assets/" + b.ID + "/" + strconv.Itoa(p.N) + "/",
		Inline:   inline,
	}
}

func unwrap(frag []byte) string {
	s := strings.TrimPrefix(string(frag), `<div class="desk-md">`)
	return strings.TrimSuffix(s, `</div>`)
}

// renderPage renders page i through the kit renderer. Against the previous
// page it marks new or changed blocks with data-changed (in place), unless
// the document needs whole-document rendering (footnotes, a TOC); then only
// the counts are reported.
func (a *app) renderPage(b *book, i int, inline bool) (renderedPage, error) {
	p := b.Pages[i]
	opt := a.renderOptions(b, p, inline)
	var rp renderedPage
	cur := splitBlocks(p.Markdown)
	rp.Blocks = len(cur)
	var changed []bool
	if j := b.prevFor(i); j >= 0 {
		rp.HasPrev, rp.Prev = true, j
		changed, rp.Removed = diffBlocks(splitBlocks(b.Pages[j].Markdown), cur)
		for _, c := range changed {
			if c {
				rp.Changed++
			}
		}
	}
	// Highlight in place only when it tells the reader something: not on
	// the first page of its kind, not when every block is new.
	if !rp.HasPrev || rp.Changed == rp.Blocks || wholeRe.MatchString(p.Markdown) {
		out, err := render.Markdown([]byte(p.Markdown), opt)
		if err != nil {
			return rp, err
		}
		rp.HTML = unwrap(out)
		return rp, nil
	}
	// Link reference definitions apply document-wide: give them to every block.
	var defs []string
	for _, ln := range strings.Split(p.Markdown, "\n") {
		if refDefRe.MatchString(ln) {
			defs = append(defs, ln)
		}
	}
	suffix := ""
	if len(defs) > 0 {
		suffix = "\n\n" + strings.Join(defs, "\n") + "\n"
	}
	var sb strings.Builder
	for j, blk := range cur {
		out, err := render.Markdown([]byte(blk+suffix), opt)
		if err != nil {
			return rp, err
		}
		inner := strings.TrimSpace(unwrap(out))
		if inner == "" {
			continue
		}
		if changed[j] {
			sb.WriteString(`<div class="vw-block" data-changed="true">` + inner + `</div>`)
		} else {
			sb.WriteString(`<div class="vw-block">` + inner + `</div>`)
		}
	}
	rp.HTML = sb.String()
	return rp, nil
}

func changeSummary(rp renderedPage, prevLabel, prevHref string) string {
	if !rp.HasPrev {
		return ""
	}
	link := html.EscapeString(prevLabel)
	if prevHref != "" {
		link = `<a href="` + html.EscapeString(prevHref) + `">` + link + `</a>`
	}
	if rp.Changed == 0 && rp.Removed == 0 {
		return `<p class="vw-changes vw-meta" data-vw-changes="0">No changes since ` + link + `.</p>`
	}
	msg := fmt.Sprintf("%d of %d blocks new or changed", rp.Changed, rp.Blocks)
	if rp.Removed > 0 {
		msg += fmt.Sprintf(", %d removed", rp.Removed)
	}
	if rp.Changed > 0 && rp.Changed < rp.Blocks {
		msg += ". Highlighted below"
	}
	return `<div class="vw-changes desk-alert" data-kind="ok" data-vw-changes="` + strconv.Itoa(rp.Changed) + `">Changed since ` + link + `: ` + msg + `.</div>`
}

func pageMeta(p page) string {
	s := p.id() + " · " + p.Created
	if p.Template != "" {
		s += " · template " + p.Template
	}
	return `<p class="vw-meta">` + html.EscapeString(s) + `</p>`
}

func navBtn(label, href, rel string, ok bool) string {
	if !ok {
		return `<span class="desk-btn secondary" aria-disabled="true">` + label + `</span>`
	}
	r := ""
	if rel != "" {
		r = ` rel="` + rel + `"`
	}
	return `<a class="desk-btn secondary" href="` + html.EscapeString(href) + `"` + r + `>` + label + `</a>`
}

// jumpForm is the shared "go to" field: a book id at the root, a page
// (p3, 3, first, last, index) inside a book. A plain GET form: no script.
func (a *app) jumpForm(bookID string) string {
	var b strings.Builder
	b.WriteString(`<form method="get" action="` + html.EscapeString(a.prefix+"/") + `">`)
	if bookID == "" {
		b.WriteString(kit.TextField("id", "Open book", "text", "", "id"))
	} else {
		b.WriteString(`<input type="hidden" name="id" value="` + html.EscapeString(bookID) + `">`)
		b.WriteString(kit.TextField("page", "Jump to page", "text", "", "p3, last, index"))
	}
	b.WriteString(`<button type="submit" class="desk-btn secondary">Go</button></form>`)
	return b.String()
}

const viewerScript = `<script>
(function(){
  document.addEventListener('keydown', function(e){
    var t = e.target && e.target.tagName;
    if (t === 'INPUT' || t === 'TEXTAREA' || e.metaKey || e.ctrlKey || e.altKey) return;
    var rel = e.key === 'ArrowLeft' ? 'prev' : e.key === 'ArrowRight' ? 'next' : '';
    var a = rel && document.querySelector('.vw-nav a[rel="' + rel + '"]');
    if (a) { e.preventDefault(); location.href = a.href; }
  });
  var p = document.querySelector('[data-vw-print]');
  if (p) p.addEventListener('click', function(){ window.print(); });
  var m = location.hash.match(/^#invite=([0-9a-f]{48})$/);
  var id = new URLSearchParams(location.search).get('id');
  if (m && id) {
    fetch('api/action', {method:'POST', headers:{'Content-Type':'application/json'},
      body: JSON.stringify({action:'join', id:id, invite:m[1]})})
      .then(function(r){ history.replaceState(null, '', location.pathname + location.search); if (r.ok) location.reload(); else if (window.deskToast) deskToast('This invite is invalid', 'error'); });
  }
})();
</script>`

// page renders the human door: the book list at the root, a book's page,
// or its index. Everything is read-only: no form ever changes content.
func (a *app) page(r *http.Request, me string) (int, []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	q := r.URL.Query()
	id := q.Get("id")
	if id == "" {
		return http.StatusOK, a.rootPage()
	}
	fail := func(code int, title, hint string) (int, []byte) {
		body := viewerCSS + kit.AppBar(tool, a.baseURL+"/", kit.EmptyItemHeader(id), "") +
			`<main class="desk-main desk-narrow">` + kit.State("error", title, hint) +
			`<p class="desk-footer"><a href="` + html.EscapeString(a.baseURL+"/") + `">All books</a> · ` + kit.CopyButton("Copy link", a.baseURL+"/") + `</p></main>`
		return code, kit.Page(tool, "Viewer · bashy desk", body)
	}
	b, err := a.load(id)
	if err != nil {
		if he, ok := err.(*httpError); ok && he.code == http.StatusNotFound {
			return fail(http.StatusNotFound, "Book not found", "Agents create it with: viewer add "+id)
		}
		return fail(http.StatusBadRequest, "Cannot open this book", err.Error())
	}
	if !a.canRead(r, b, me) {
		return fail(http.StatusForbidden, "Join this book first", "Ask the owner for an invite link.")
	}
	header := kit.ItemHeader(b.ID, b.Status, a.urlFor(b.ID))
	sel := q.Get("page")
	if strings.EqualFold(sel, "index") || len(b.Pages) == 0 {
		return http.StatusOK, a.indexPage(b, header)
	}
	i := b.pageIndex(sel)
	if i < 0 {
		return fail(http.StatusNotFound, "No page "+sel, fmt.Sprintf("%s has %d pages: use p1–p%d, first, last or index.", b.ID, len(b.Pages), len(b.Pages)))
	}
	p := b.Pages[i]
	rp, err := a.renderPage(b, i, false)
	if err != nil {
		return fail(http.StatusInternalServerError, "This page could not be rendered", err.Error())
	}
	n := len(b.Pages)
	pos := fmt.Sprintf(`<span class="vw-pos">%s of %d</span>`, p.id(), n)
	nav := `<nav class="vw-nav" aria-label="Pages"><div class="desk-row">` +
		navBtn("⏮ First", a.pageURL(b.ID, 1), "first", i > 0) +
		navBtn("◀ Prev", a.pageURL(b.ID, i), "prev", i > 0) + pos +
		navBtn("Next ▶", a.pageURL(b.ID, i+2), "next", i < n-1) +
		navBtn("Last ⏭", a.pageURL(b.ID, n), "last", i < n-1) +
		navBtn("Index", a.urlFor(b.ID)+"&page=index", "", true) +
		`</div>` + a.jumpForm(b.ID) + `</nav>`
	prevHref := ""
	prevLabel := ""
	if rp.HasPrev {
		prevHref, prevLabel = a.pageURL(b.ID, rp.Prev+1), b.Pages[rp.Prev].id()
	}
	body := viewerCSS + kit.AppBar(tool, a.baseURL+"/", header, `<span class="vw-pos">`+html.EscapeString(b.Title)+`</span>`) +
		`<main class="desk-main desk-narrow">` + nav + `<div data-desk-alert class="desk-alert" hidden></div>` +
		changeSummary(rp, prevLabel, prevHref) +
		`<article class="vw-page desk-content" id="` + p.id() + `">` + pageMeta(p) + rp.HTML + `</article>` +
		`<div class="desk-row vw-actions">` + kit.CopyButton("Copy page link", a.pageURL(b.ID, i+1)) +
		`<a class="desk-btn secondary" href="` + html.EscapeString(a.prefix+"/api/export?id="+b.ID+"&page="+strconv.Itoa(i+1)) + `">Download HTML</a>` +
		`<a class="desk-btn secondary" href="` + html.EscapeString(a.prefix+"/api/export?id="+b.ID) + `">Download book</a>` +
		`<button type="button" class="desk-btn secondary" data-vw-print title="Print or save as PDF">Print / PDF</button></div>` +
		`<p class="desk-footer">View-only · ← → turn pages · re-surface with <code>viewer open ` + html.EscapeString(b.ID) + `</code> · edit text in notepad.</p></main>` +
		viewerScript
	return http.StatusOK, kit.Page(tool, p.Title+" · "+b.Title+" · Viewer", body)
}

func (a *app) indexPage(b *book, header string) []byte {
	var t strings.Builder
	if len(b.Pages) == 0 {
		t.WriteString(kit.State("empty", "No pages yet", "Agents add one with: viewer append "+b.ID+" --body … or --template status --data FILE"))
	} else {
		t.WriteString(`<table class="desk-table vw-index"><thead><tr><th>Page</th><th>Title</th><th>Template</th><th>Created</th><th>Changed</th></tr></thead><tbody>`)
		for i, p := range b.Pages {
			changed := "—"
			if j := b.prevFor(i); j >= 0 {
				c, rm := diffBlocks(splitBlocks(b.Pages[j].Markdown), splitBlocks(p.Markdown))
				k := 0
				for _, x := range c {
					if x {
						k++
					}
				}
				changed = fmt.Sprintf("%d new/changed, %d removed vs %s", k, rm, b.Pages[j].id())
			}
			t.WriteString(`<tr><td><a href="` + html.EscapeString(a.pageURL(b.ID, p.N)) + `">` + p.id() + `</a></td><td>` +
				html.EscapeString(p.Title) + `</td><td>` + html.EscapeString(p.Template) + `</td><td>` +
				html.EscapeString(p.Created) + `</td><td>` + changed + `</td></tr>`)
		}
		t.WriteString(`</tbody></table>`)
	}
	nav := `<nav class="vw-nav" aria-label="Pages"><div class="desk-row">` +
		navBtn("⏮ First", a.pageURL(b.ID, 1), "first", len(b.Pages) > 0) +
		navBtn("Last ⏭", a.pageURL(b.ID, len(b.Pages)), "last", len(b.Pages) > 0) +
		`<span class="vw-pos">Index · ` + strconv.Itoa(len(b.Pages)) + ` pages</span></div>` + a.jumpForm(b.ID) + `</nav>`
	body := viewerCSS + kit.AppBar(tool, a.baseURL+"/", header, `<span class="vw-pos">`+html.EscapeString(b.Title)+`</span>`) +
		`<main class="desk-main desk-narrow">` + nav + `<h1 class="desk-h2">` + html.EscapeString(b.Title) + `</h1>` + t.String() +
		`<p class="desk-footer">View-only · re-surface with <code>viewer open ` + html.EscapeString(b.ID) + `</code>.</p></main>` + viewerScript
	return kit.Page(tool, "Index · "+b.Title+" · Viewer", body)
}

func (a *app) rootPage() []byte {
	books, _ := a.list()
	var l strings.Builder
	l.WriteString(`<section class="desk-panel"><h2 class="desk-h2">Books</h2>`)
	if len(books) == 0 {
		l.WriteString(kit.State("empty", "Nothing to read yet", "Agents publish reports with: viewer add --sprint N --template status --data FILE"))
	} else {
		l.WriteString(`<table class="desk-table vw-index"><thead><tr><th>Book</th><th>Title</th><th>Pages</th><th>Status</th></tr></thead><tbody>`)
		for _, v := range books {
			id := v["id"].(string)
			title := ""
			if b, err := readBook(a.dir, id); err == nil {
				title = b.Title
			}
			l.WriteString(`<tr><td><a href="` + html.EscapeString(a.urlFor(id)) + `">` + html.EscapeString(id) + `</a></td><td>` +
				html.EscapeString(title) + `</td><td>` + strconv.Itoa(v["pages"].(int)) + `</td><td>` + kit.Badge(v["status"].(string)) + `</td></tr>`)
		}
		l.WriteString(`</tbody></table>`)
	}
	l.WriteString(`</section>`)
	body := viewerCSS + kit.AppBar(tool, a.baseURL+"/", kit.EmptyItemHeader("all books"), "") +
		`<main class="desk-main desk-narrow"><nav class="vw-nav" aria-label="Open"><div class="desk-row">` +
		kit.CopyButton("Copy link", a.baseURL+"/") + `</div>` + a.jumpForm("") + `</nav>` + l.String() +
		`<p class="desk-footer">View-only reader for agent reports · loopback only · data in <code>` +
		html.EscapeString(filepath.Join(a.dir, "items")) + `</code>.</p></main>` + viewerScript
	return kit.Page(tool, "Viewer · bashy desk", body)
}

// exportHTML builds a self-contained HTML file for one page or (sel empty
// or "all") the whole book: kit CSS/JS inlined, local image/video/PDF
// assets inlined as data: URIs, no request back to this server.
func (a *app) exportHTML(b *book, sel string) ([]byte, string, error) {
	if len(b.Pages) == 0 {
		return nil, "", &httpError{http.StatusConflict, "This book has no pages to export yet"}
	}
	first, last := 0, len(b.Pages)-1
	name := b.ID + ".html"
	if sel != "" && !strings.EqualFold(sel, "all") {
		i := b.pageIndex(sel)
		if i < 0 {
			return nil, "", &httpError{http.StatusNotFound, fmt.Sprintf("No page %q in %s (it has %d)", sel, b.ID, len(b.Pages))}
		}
		first, last = i, i
		name = b.ID + "-" + b.Pages[i].id() + ".html"
	}
	var toc, pages strings.Builder
	for i := first; i <= last; i++ {
		p := b.Pages[i]
		rp, err := a.renderPage(b, i, true)
		if err != nil {
			return nil, "", err
		}
		toc.WriteString(`<li><a href="#` + p.id() + `">` + p.id() + ` · ` + html.EscapeString(p.Title) + `</a></li>`)
		prevHref, prevLabel := "", ""
		if rp.HasPrev {
			prevLabel = b.Pages[rp.Prev].id()
			if rp.Prev >= first {
				prevHref = "#" + prevLabel
			}
		}
		pages.WriteString(`<article class="vw-page desk-content" id="` + p.id() + `">` + pageMeta(p) +
			changeSummary(rp, prevLabel, prevHref) + rp.HTML + `</article>`)
	}
	header := kit.ItemHeader(b.ID, b.Status, a.urlFor(b.ID))
	body := viewerCSS + kit.AppBar(tool, a.urlFor(b.ID), header, `<span class="vw-pos">`+html.EscapeString(b.Title)+`</span>`) +
		`<main class="desk-main desk-narrow">`
	if last > first {
		body += `<nav class="vw-nav desk-panel" aria-label="Contents"><ul>` + toc.String() + `</ul></nav>`
	}
	body += pages.String() + `<p class="desk-footer">Exported from the bashy desk viewer · ` + html.EscapeString(now()) + `</p></main>`
	return kit.Page(tool, b.Title+" · Viewer export", body), name, nil
}
