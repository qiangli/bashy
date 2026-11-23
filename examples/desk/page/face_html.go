package main

import (
	"html"
	"net/http"
	"slices"
	"strings"
)

type blockFaceHTML struct {
	face pageFace
	body string
}

func (a *app) renderBlockFaces(r *http.Request, it *item, b *block, view pageFace) string {
	settings := it.facesFor(b)
	k := blockKinds[b.Kind]
	native := nativeBlockLocked(it, b)
	var faces []blockFaceHTML
	add := func(face pageFace, content string) {
		if content != "" && slices.Contains(k.Faces, string(face)) {
			faces = append(faces, blockFaceHTML{face, content})
		}
	}
	switch {
	case native != nil:
		if view == displayFace && !settings.Project {
			return ""
		}
		for _, f := range a.nativeFaces(r, it, b, native, view) {
			if f.face == recordFace {
				faces = append(faces, f)
			} else {
				add(f.face, f.body)
			}
		}
	case view == recordFace:
		return blockHTML(b)
	case k.Embed:
		if view == organizerFace {
			add(organizerFace, embedFace(b, organizerFace, settings.OrganizerURL, settings.LinkOnly, true))
		}
		if view != displayFace {
			add(participantFace, embedFace(b, participantFace, b.Ref, settings.LinkOnly, true))
		}
		if view == organizerFace || view == displayFace {
			add(displayFace, embedFace(b, displayFace, settings.DisplayURL, settings.LinkOnly, view == organizerFace))
		}
	default:
		if view != displayFace {
			return blockHTML(b)
		}
	}
	if len(faces) == 0 {
		return ""
	}
	var out strings.Builder
	id := html.EscapeString(b.ID)
	out.WriteString(`<section class="desk-panel page-block" id="block-` + id + `" data-block="` + id +
		`" data-kind="` + html.EscapeString(b.Kind) + `">`)
	if len(faces) > 1 {
		out.WriteString(`<div class="page-face-tabs" role="tablist" aria-label="Block views">`)
		for i, f := range faces {
			target := id + "-" + string(f.face)
			selected, index := "false", "-1"
			if i == 0 {
				selected, index = "true", "0"
			}
			out.WriteString(`<button type="button" role="tab" id="tab-` + target + `" aria-controls="face-` +
				target + `" aria-selected="` + selected + `" tabindex="` + index + `">` + faceTitle(f.face) + `</button>`)
		}
		out.WriteString(`</div><div class="page-faces">`)
		for i, f := range faces {
			target := id + "-" + string(f.face)
			active := ""
			if i == 0 {
				active = " data-active"
			}
			out.WriteString(`<section class="page-face" id="face-` + target + `" role="tabpanel" aria-labelledby="tab-` + target +
				`"` + active + `><h2 class="page-face-title">` + faceTitle(f.face) + `</h2>` + f.body + `</section>`)
		}
		out.WriteString(`</div>`)
	} else {
		out.WriteString(faces[0].body)
	}
	out.WriteString(`</section>`)
	return out.String()
}

func faceTitle(face pageFace) string {
	switch face {
	case organizerFace:
		return "Organizer"
	case displayFace:
		return "Display"
	default:
		return "Participant"
	}
}

func embedFace(b *block, face pageFace, raw string, linkOnly, openLink bool) string {
	if raw == "" || validEmbedRef(raw) != nil {
		return ""
	}
	title := b.Options["title"]
	if title == "" {
		title = b.ID
	}
	permissions := "allow-scripts allow-same-origin"
	if face != displayFace {
		permissions += " allow-forms"
	}
	ref := html.EscapeString(raw)
	source := ` src="` + ref + `"`
	if face == organizerFace {
		// Only start the private face in a browser that isolates its
		// cookies and session storage from the participant face.
		source = ` hidden credentialless data-organizer-src="` + ref + `"`
	}
	var out string
	if !linkOnly {
		out = `<iframe class="page-embed" title="` + html.EscapeString(title+" · "+faceTitle(face)) +
			`"` + source + ` sandbox="` + permissions + `" referrerpolicy="no-referrer"></iframe>`
	}
	if openLink || linkOnly {
		out += `<p><a href="` + ref + `" target="_blank" rel="noopener noreferrer">Open ` +
			strings.ToLower(faceTitle(face)) + ` view</a></p>`
	}
	return out
}

const pageFaceStyle = `<style>
.page-main{max-width:90rem;margin-inline:auto}
.page-view-nav{display:flex;flex-wrap:wrap;gap:1rem;margin-block:1rem}
.page-block{border-style:dashed}
.page-embed{display:block;width:100%;min-height:22rem;border:0}
.page-embed[hidden]{display:none}
.page-display{max-width:none;font-size:1.6rem}
.page-display h1{font-size:2.5rem}
.page-display .page-embed{min-height:66vh}
.page-face{min-width:0}
.page-face-tabs{display:flex;gap:.5rem;margin-bottom:1rem}
.page-face-tabs [aria-selected=true]{font-weight:bold;text-decoration:underline}
.page-face:not([data-active]){display:none}
.page-face-title{display:none}
@media(min-width:64rem){
 .page-face-tabs{display:none}
 .page-faces{display:flex;gap:1rem}
 .page-face{display:block!important;flex:1}
 .page-face-title{display:block}
}
</style>`

const faceTabsScript = `<script>
(function () {
  if ('credentialless' in HTMLIFrameElement.prototype) {
    document.querySelectorAll('iframe[data-organizer-src]').forEach(frame => {
      frame.src = frame.dataset.organizerSrc;
      frame.hidden = false;
    });
  }
  function select(tabs, tab) {
    tabs.forEach(other => {
      const active = other === tab;
      other.setAttribute('aria-selected', active);
      other.tabIndex = active ? 0 : -1;
      document.getElementById(other.getAttribute('aria-controls')).toggleAttribute('data-active', active);
    });
  }
  document.addEventListener('click', e => {
    const tab = e.target.closest('.page-face-tabs [role=tab]');
    if (tab) select(Array.from(tab.parentElement.children), tab);
  });
  document.addEventListener('keydown', e => {
    const tab = e.target.closest('.page-face-tabs [role=tab]');
    if (!tab) return;
    const tabs = Array.from(tab.parentElement.children);
    const i = tabs.indexOf(tab);
    const next = {ArrowLeft: (i + tabs.length - 1) % tabs.length,
      ArrowRight: (i + 1) % tabs.length, Home: 0, End: tabs.length - 1}[e.key];
    if (next === undefined) return;
    e.preventDefault();
    select(tabs, tabs[next]);
    tabs[next].focus();
  });
})();
</script>`
