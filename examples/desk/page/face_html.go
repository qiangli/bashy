package main

import (
	"html"
	"net/http"
	"slices"
	"strings"

	"page/blocks"
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
	case b.Kind == "book" || b.Kind == "note":
		switch view {
		case displayFace:
			return ""
		case recordFace:
			faces = append(faces, blockFaceHTML{recordFace, contentHTML(b)})
		default:
			add(participantFace, contentHTML(b))
		}
	case b.Kind == "board":
		for _, f := range a.boardFaces(r, it, b, view) {
			if f.face == recordFace {
				faces = append(faces, f)
			} else {
				add(f.face, f.body)
			}
		}
	case b.Kind == "timer":
		switch view {
		case displayFace:
			if !settings.Project {
				return ""
			}
			add(displayFace, a.timerHTML(it, b, displayFace))
		case recordFace:
			faces = append(faces, blockFaceHTML{recordFace, a.timerHTML(it, b, recordFace)})
		default:
			add(view, a.timerHTML(it, b, view))
		}
	case b.Kind == "here":
		if view == displayFace {
			return ""
		}
		faces = append(faces, blockFaceHTML{view, blocks.Here(blocks.HereData{Rows: a.hereRows(it, view)})})
	case b.Kind == "hands":
		if view == displayFace && !settings.Project {
			return ""
		}
		faces = append(faces, blockFaceHTML{view, a.handsHTML(r, it, view)})
	case view == recordFace:
		return blockHTML(b)
	case k.Embed:
		linkOnly := settings.LinkOnly || b.Options["mode"] == "link"
		if view == organizerFace {
			add(organizerFace, embedFace(b, organizerFace, settings.OrganizerURL, linkOnly, true))
		}
		if view != displayFace {
			add(participantFace, embedFace(b, participantFace, b.Ref, linkOnly, true))
		}
		if view == organizerFace || view == displayFace {
			add(displayFace, embedFace(b, displayFace, settings.DisplayURL, linkOnly, view == organizerFace))
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

// embedFace frames a same-origin kit app; an engine that refuses framing
// (or a block with mode=link / link_only) gets a link-out card instead.
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
	frame := !linkOnly && !engineRefusesFraming(raw)
	return blocks.Embed(blocks.EmbedData{Title: title + " · " + faceTitle(face), Src: raw, Sandbox: permissions,
		// Only start the private face in a browser that isolates its
		// cookies and session storage from the participant face.
		Private: face == organizerFace, Frame: frame, Link: openLink || !frame,
		LinkLabel: "Open " + strings.ToLower(faceTitle(face)) + " view"})
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
