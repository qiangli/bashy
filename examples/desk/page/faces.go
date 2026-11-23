package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"slices"
	"strings"

	kit "desk-kit"
)

type pageFace string

const (
	organizerFace   pageFace = "organizer"
	participantFace pageFace = "participant"
	displayFace     pageFace = "display"
	recordFace      pageFace = "record"
)

// faceSettings are edited separately from the Markdown shown to readers.
type faceSettings struct {
	OrganizerURL string `json:"organizer_url,omitempty"`
	DisplayURL   string `json:"display_url,omitempty"`
	Project      bool   `json:"project,omitempty"`
}

type storedFaces struct {
	faceSettings
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
}

func (it *item) facesFor(b *block) faceSettings {
	if f := it.BlockFaces[b.ID]; f.Kind == b.Kind && f.Ref == b.Ref {
		return f.faceSettings
	}
	return faceSettings{}
}

// updateFaces preserves settings across edits to the surrounding Markdown and
// discards settings when the associated block is removed or replaced.
func (it *item) updateFaces(changes map[string]*faceSettings) error {
	blocks := blocksOf(it.Body)
	configured := make(map[string]storedFaces)
	for _, b := range blocks {
		input, changed := changes[b.ID]
		f := it.facesFor(&b)
		if changed {
			delete(changes, b.ID)
			f = faceSettings{}
			if input != nil {
				f = *input
			}
		}
		if err := validateFaces(&b, f); err != nil {
			return fmt.Errorf("block %s: %w", b.ID, err)
		}
		if f.OrganizerURL != "" && (strings.Contains(it.Body, f.OrganizerURL) ||
			leaksEmbedKey(f.OrganizerURL, it.Body) || leaksEmbedKey(f.OrganizerURL, it.Title)) {
			return fmt.Errorf("block %s: the organizer URL and key must stay out of public content", b.ID)
		}
		if f != (faceSettings{}) {
			configured[b.ID] = storedFaces{faceSettings: f, Kind: b.Kind, Ref: b.Ref}
		}
	}
	if len(changes) > 0 {
		return fmt.Errorf("face settings must name blocks in the page")
	}
	for id, f := range configured {
		if f.OrganizerURL == "" {
			continue
		}
		for _, other := range configured {
			if leaksEmbedKey(f.OrganizerURL, other.DisplayURL) {
				return fmt.Errorf("block %s: display URLs cannot carry an organizer key", id)
			}
		}
	}
	it.BlockFaces = configured
	return nil
}

func validateFaces(b *block, f faceSettings) error {
	k := blockKinds[b.Kind]
	if f.OrganizerURL != "" || f.DisplayURL != "" {
		if !k.Embed {
			return fmt.Errorf("face URLs are for embed blocks")
		}
		for _, raw := range []string{f.OrganizerURL, f.DisplayURL} {
			if raw != "" && validEmbedRef(raw) != nil {
				return fmt.Errorf("invalid face URL")
			}
		}
		if f.OrganizerURL != "" && (f.OrganizerURL == b.Ref || f.OrganizerURL == f.DisplayURL) {
			return fmt.Errorf("the organizer URL must be private")
		}
		if leaksEmbedKey(f.OrganizerURL, b.Ref) || leaksEmbedKey(f.OrganizerURL, f.DisplayURL) {
			return fmt.Errorf("the organizer key must be private")
		}
	}
	if f.DisplayURL != "" && hasOrganizerParameter(f.DisplayURL, false) {
		return fmt.Errorf("the display URL cannot carry an organizer key")
	}
	if f.Project && !slices.Contains(k.Faces, "display") {
		return fmt.Errorf("this kind has no display face")
	}
	return nil
}

func embedParameters(raw string) []url.Values {
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	fragment, _ := url.ParseQuery(u.Fragment)
	return []url.Values{u.Query(), fragment}
}

func privateParameter(name string, genericKey bool) bool {
	name = strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(name))
	switch name {
	case "facilitator", "facilitatorkey", "teacher", "teacherkey", "owner", "ownerkey", "organizer", "organizerkey", "controlkey":
		return true
	}
	return genericKey && name == "key"
}

func hasOrganizerParameter(raw string, genericKey bool) bool {
	for _, values := range embedParameters(raw) {
		for name := range values {
			if privateParameter(name, genericKey) {
				return true
			}
		}
	}
	return false
}

func leaksEmbedKey(private, other string) bool {
	if private == "" || other == "" {
		return false
	}
	decoded, _ := url.PathUnescape(other)
	contains := func(key string) bool {
		return key != "" && (strings.Contains(other, key) || strings.Contains(decoded, key))
	}
	u, err := url.Parse(private)
	if err == nil && !strings.Contains(u.Fragment, "=") && contains(u.Fragment) {
		// Demo Queue uses an opaque fragment instead of name=value.
		return true
	}
	for _, values := range embedParameters(private) {
		for name, keys := range values {
			if privateParameter(name, true) {
				for _, key := range keys {
					if contains(key) {
						return true
					}
				}
			}
		}
	}
	return false
}

func faceFor(r *http.Request, owner bool) pageFace {
	switch r.URL.Query().Get("view") {
	case "display":
		return displayFace
	case "participant":
		return participantFace
	default:
		if owner {
			return organizerFace
		}
		return participantFace
	}
}

func (a *app) organizerCookie(r *http.Request, it *item) bool {
	c, err := r.Cookie("page-organizer-" + it.ID)
	return err == nil && it.Owner != "" && kit.Grant(c.Value, pageToken(it, "organizer"))
}

func pageToken(it *item, purpose string) string {
	h := hmac.New(sha256.New, []byte(it.Owner))
	h.Write([]byte(purpose))
	return hex.EncodeToString(h.Sum(nil))
}

func (a *app) beginOrganizer(w http.ResponseWriter, r *http.Request, it *item) {
	if it.Owner == "" || !kit.Grant(kit.FacilitatorKey(r), it.Owner) {
		kit.Fail(w, http.StatusForbidden, "The organizer key is required")
		return
	}
	a.setFaceCookie(w, r, "page-organizer-"+it.ID, pageToken(it, "organizer"))
	kit.WriteJSON(w, map[string]bool{"ok": true})
}

func (a *app) setFaceCookie(w http.ResponseWriter, r *http.Request, name, token string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: token, Path: kit.PublicPrefix(r, a.prefix) + "/",
		HttpOnly: true, Secure: r.TLS != nil || strings.HasPrefix(kit.RequestOrigin(r), "https://"), SameSite: http.SameSiteStrictMode})
}

// Preview enrolls a fresh identity for this browser without touching the
// participant registry or issuing a normal participant cookie.
func (a *app) previewParticipant(w http.ResponseWriter, r *http.Request, it *item) *http.Request {
	name := "page-preview-" + it.ID
	nonce := ""
	if cookie, err := r.Cookie(name); err == nil && kit.ValidToken(cookie.Value) {
		nonce = cookie.Value
	}
	if nonce == "" {
		nonce = kit.NewToken()
		a.setFaceCookie(w, r, name, nonce)
	}
	return withParticipant(r, pageToken(it, "preview:"+nonce), Participant{Handle: "Preview", Auth: "anonymous"})
}

func organizerEntry(base, id string) string {
	target, _ := json.Marshal(base + "/api/action")
	payload, _ := json.Marshal(map[string]string{"action": "organizer-session", "id": id})
	return `<main class="desk-main desk-narrow"><p id="page-organizer-status">Open the organizer link to continue.</p></main>
<script>
(async function () {
  const key = new URLSearchParams(location.hash.slice(1)).get('owner');
  if (!key) return;
  history.replaceState(null, '', location.pathname + location.search);
  try {
    const response = await fetch(` + string(target) + `, {
      method: 'POST',
      headers: {'Content-Type': 'application/json', 'X-Facilitator-Key': key},
      body: JSON.stringify(` + string(payload) + `)
    });
    if (!response.ok) throw new Error();
    location.reload();
  } catch (_) {
    document.getElementById('page-organizer-status').textContent = 'The organizer link could not be opened.';
  }
})();
</script>`
}

func pageViewLink(r *http.Request, view string) string {
	q := r.URL.Query()
	q.Set("view", view)
	return "?" + q.Encode()
}

func faceNavigation(r *http.Request, owner bool, face pageFace) string {
	if !owner || face == displayFace {
		return ""
	}
	if face == participantFace {
		return `<p><a href="` + html.EscapeString(pageViewLink(r, "organizer")) + `">Return to organizer view</a></p>`
	}
	return `<nav class="page-view-nav"><a href="` + html.EscapeString(pageViewLink(r, "participant")) +
		`">View as participant</a><a href="` + html.EscapeString(pageViewLink(r, "display")) + `">Open display</a></nav>`
}
