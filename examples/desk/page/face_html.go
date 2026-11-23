package main

import (
	"html"
	"slices"
	"strings"
)

type blockFaceHTML struct {
	face pageFace
	body string
}

func renderBlockFaces(it *item, b *block, view pageFace, base string) string {
	settings := it.facesFor(b)
	if view == recordFace {
		return blockHTML(b)
	}
	k := blockKinds[b.Kind]
	var faces []blockFaceHTML
	add := func(face pageFace, content string) {
		if content != "" && slices.Contains(k.Faces, string(face)) {
			faces = append(faces, blockFaceHTML{face, content})
		}
	}
	switch {
	case k.Embed:
		if view == organizerFace {
			add(organizerFace, embedFace(b, organizerFace, settings.OrganizerURL, true))
		}
		if view != displayFace {
			add(participantFace, embedFace(b, participantFace, b.Ref, true))
		}
		if view == organizerFace || view == displayFace {
			add(displayFace, embedFace(b, displayFace, settings.DisplayURL, view == organizerFace))
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

func embedFace(b *block, face pageFace, raw string, openLink bool) string {
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
	isolation := ""
	if face == organizerFace {
		isolation = " credentialless"
	}
	out := `<iframe class="page-embed" title="` + html.EscapeString(title+" · "+faceTitle(face)) +
		`" src="` + ref + `" sandbox="` + permissions + `" referrerpolicy="no-referrer"` + isolation + `></iframe>`
	if openLink {
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
document.querySelectorAll('.page-face-tabs').forEach(group => {
  const tabs = Array.from(group.querySelectorAll('[role=tab]'));
  function select(tab) {
    tabs.forEach(other => {
      const active = other === tab;
      other.setAttribute('aria-selected', active);
      other.tabIndex = active ? 0 : -1;
      document.getElementById(other.getAttribute('aria-controls')).toggleAttribute('data-active', active);
    });
  }
  group.addEventListener('click', e => { if (tabs.includes(e.target)) select(e.target); });
  group.addEventListener('keydown', e => {
    const i = tabs.indexOf(e.target);
    if (i < 0) return;
    const next = {ArrowLeft: (i + tabs.length - 1) % tabs.length,
      ArrowRight: (i + 1) % tabs.length, Home: 0, End: tabs.length - 1}[e.key];
    if (next === undefined) return;
    e.preventDefault();
    select(tabs[next]);
    tabs[next].focus();
  });
});
</script>`
