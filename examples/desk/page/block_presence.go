package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	kit "desk-kit"
	"page/blocks"
)

// HandEntry stores a participant key privately. Views resolve it to a handle.
type HandEntry struct {
	ParticipantKey string `json:"participant_key"`
}

// A group follows the participant if moderation changes their key. Each
// admitted stream holds a pointer so its disconnect uses the current key.
type presenceKey struct{ page, participant string }

type presenceGroup struct {
	key     presenceKey
	streams int
}

type presenceWriter struct {
	http.ResponseWriter
	connect func()
}

func (w presenceWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w presenceWriter) WriteHeader(status int) {
	if status == http.StatusOK {
		w.connect()
	}
	w.ResponseWriter.WriteHeader(status)
}

func (a *app) servePresence(w http.ResponseWriter, r *http.Request, id string, revision int, preview bool) {
	p, ok := requestParticipant(r)
	if !ok || preview {
		a.events.Serve(w, r, id, revision)
		return
	}
	var disconnect func()
	w = presenceWriter{ResponseWriter: w, connect: func() {
		disconnect = a.connectPresence(id, p)
	}}
	defer func() {
		if disconnect != nil {
			disconnect()
		}
	}()
	a.events.Serve(w, r, id, revision)
}

func (a *app) savePresenceBlocksLocked(it *item, kinds ...string) error {
	it.Revision++
	if err := a.persistLocked(it); err != nil {
		return err
	}
	a.publishPresenceBlocksLocked(it, kinds...)
	return nil
}

func (a *app) connectPresence(id string, p participantRequest) func() {
	a.mu.Lock()
	defer a.mu.Unlock()
	it := a.items[id]
	key := presenceKey{id, p.Key}
	enrolled := it.Participants[p.Key]
	if enrolled != nil && enrolled.Kicked && enrolled.Cookie == "" && p.Principal == "" {
		// A rename can happen between identity resolution and stream admission.
		key.participant = participantCookieKey(it, participantCaller{
			address: p.Address, cookie: p.Cookie, fingerprint: p.Fingerprint,
		})
		enrolled = it.Participants[key.participant]
	}
	if enrolled == nil || enrolled.Kicked {
		return nil
	}
	group := a.presence[key]
	if group == nil {
		group = &presenceGroup{key: key}
		if a.presence == nil {
			a.presence = make(map[presenceKey]*presenceGroup)
		}
		a.presence[key] = group
	}
	group.streams++
	a.publishPresenceBlocksLocked(it, "here")
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		group.streams--
		if group.streams == 0 {
			delete(a.presence, group.key)
		}
		it := a.items[id]
		if p := it.Participants[group.key.participant]; p != nil && !p.Kicked {
			next := it.clone()
			next.Participants[group.key.participant].LastSeen = time.Now().UTC().Format(time.RFC3339Nano)
			if err := a.persistLocked(&next); err != nil {
				log.Printf("page %q: could not save last seen: %v", id, err)
			} else {
				it = &next
			}
		}
		a.publishPresenceBlocksLocked(it, "here")
	}
}

func (a *app) publishPresenceBlocksLocked(it *item, kinds ...string) {
	if breakMode == "events-no-publish" {
		return
	}
	for _, b := range blocksOf(it.Body) {
		if slices.Contains(kinds, b.Kind) {
			a.events.Publish(it.ID, b.ID, it.Revision)
		}
	}
}

// The identity store signals enrollment and moderation with "here". Check
// the saved rows so an unrelated block named "here" keeps its own frames.
func participantSave(before, after *item, block string) bool {
	return block == "here" && before != nil &&
		(before.OpenHandles != after.OpenHandles ||
			!maps.EqualFunc(before.Participants, after.Participants, func(p, q *Participant) bool {
				return p == q || p != nil && q != nil && *p == *q
			}))
}

func (a *app) followPresenceRenameLocked(before *item, req actionRequest) {
	if req.Kind != "rename" {
		return
	}
	after := a.items[before.ID]
	source, err := moderatorKey(before, req.Source)
	if err != nil || before == after {
		return
	}
	p := before.Participants[source]
	target := anonymousKey(strings.TrimSpace(req.Target), p.Address)
	oldKey := presenceKey{before.ID, source}
	if group := a.presence[oldKey]; group != nil && after.Participants[source].Kicked &&
		after.Participants[target] != nil && !after.Participants[target].Kicked {
		delete(a.presence, oldKey)
		group.key.participant = target
		a.presence[group.key] = group
	}
}

func (a *app) hereRows(it *item, face pageFace) []blocks.HereRow {
	rows := make([]blocks.HereRow, 0, len(it.Participants))
	for key, p := range it.Participants {
		if p == nil || p.Kicked {
			continue
		}
		row := blocks.HereRow{Handle: p.Handle, Auth: p.Auth, LastSeen: p.LastSeen,
			Online: a.presence[presenceKey{it.ID, key}] != nil}
		if face == organizerFace {
			row.Address, row.Fingerprint = p.Address, p.Fingerprint
		}
		rows = append(rows, row)
	}
	slices.SortFunc(rows, func(a, b blocks.HereRow) int {
		return cmp.Or(strings.Compare(a.Handle, b.Handle), strings.Compare(a.Auth, b.Auth),
			strings.Compare(a.LastSeen, b.LastSeen), strings.Compare(a.Address, b.Address))
	})
	return rows
}

func handsHandles(it *item) []string {
	handles := make([]string, 0, len(it.Hands))
	for _, hand := range it.Hands {
		if p := it.Participants[hand.ParticipantKey]; p != nil && !p.Kicked {
			handles = append(handles, p.Handle)
		}
	}
	return handles
}

func handIndex(it *item, key string) int {
	return slices.Index(it.Hands, HandEntry{ParticipantKey: key})
}

func (a *app) handsHTML(r *http.Request, it *item, face pageFace) string {
	d := blocks.HandsData{Handles: handsHandles(it), Organizer: face == organizerFace}
	if p, ok := requestParticipant(r); face == participantFace && ok {
		if enrolled := it.Participants[p.Key]; enrolled != nil && !enrolled.Kicked {
			if handIndex(it, p.Key) == -1 {
				d.ParticipantAction = "raise"
			} else {
				d.ParticipantAction = "lower"
			}
		}
	}
	return blocks.Hands(d)
}

// remapHand drops a kicked participant, moves a renamed hand, or
// deduplicates a merge while keeping the target's place if already queued.
func (it *item) remapHand(source, target string) {
	index := handIndex(it, source)
	if index == -1 {
		return
	}
	if target == "" || handIndex(it, target) != -1 {
		it.Hands = slices.Delete(it.Hands, index, index+1)
		return
	}
	it.Hands[index].ParticipantKey = target
}

func (a *app) handsAction(w http.ResponseWriter, r *http.Request, it *item, req actionRequest) {
	owner := a.canOrganize(r, it)
	face := faceFor(r, owner)
	var key string
	switch req.Kind {
	case "raise", "lower":
		if owner || face != participantFace {
			kit.Fail(w, http.StatusForbidden, "Join the page as a participant to use the speaking queue")
			return
		}
		if !a.viewOr403(w, r, it) {
			return
		}
		r, done, ok := a.knownParticipantLocked(w, r, it)
		if !ok {
			return
		}
		defer done()
		p, enrolled := requestParticipant(r)
		if !enrolled {
			kit.Fail(w, http.StatusForbidden, "Join the page before using the speaking queue")
			return
		}
		key = p.Key
		it = a.items[it.ID]
	case "next", "clear", "list":
		if face != organizerFace {
			kit.Fail(w, http.StatusForbidden, "Only the organizer can manage the speaking queue")
			return
		}
	default:
		kit.Fail(w, http.StatusBadRequest, "Use raise, lower, next, clear or list")
		return
	}

	next := it.clone()
	index := handIndex(&next, key)
	switch req.Kind {
	case "raise":
		if index == -1 {
			next.Hands = append(next.Hands, HandEntry{ParticipantKey: key})
		}
	case "lower":
		next.remapHand(key, "")
	case "next":
		if len(next.Hands) > 0 {
			next.Hands = slices.Delete(next.Hands, 0, 1)
		}
	case "clear":
		next.Hands = nil
	}
	if !slices.Equal(it.Hands, next.Hands) {
		if err := a.savePresenceBlocksLocked(&next, "hands"); err != nil {
			kit.Fail(w, http.StatusInternalServerError, "Could not save the speaking queue")
			return
		}
	}
	v := a.openView(r, &next)
	v["hands"] = handsHandles(&next)
	kit.WriteJSON(w, v)
}

func (a *app) presenceMarkdown(it *item, b *block) string {
	var out strings.Builder
	if b.Kind == "here" {
		out.WriteString("\n### Who’s here\n\n")
		rows := a.hereRows(it, recordFace)
		if len(rows) == 0 {
			out.WriteString("No participants yet.\n")
		}
		for _, row := range rows {
			status := "offline"
			if row.Online {
				status = "online"
			}
			if row.LastSeen != "" {
				status += ", last seen " + row.LastSeen
			}
			fmt.Fprintf(&out, "- %s (%s, %s)\n", escapeParticipantMarkdown(row.Handle), row.Auth, status)
		}
	} else {
		out.WriteString("\n### Speaking queue\n\n")
		handles := handsHandles(it)
		if len(handles) == 0 {
			out.WriteString("No hands raised.\n")
		}
		for i, handle := range handles {
			fmt.Fprintf(&out, "%d. %s\n", i+1, escapeParticipantMarkdown(handle))
		}
	}
	out.WriteByte('\n')
	return out.String()
}

func handsScript(base, id string, face pageFace) string {
	cfg, _ := json.Marshal(map[string]string{"action": base + "/api/action?view=" + string(face), "id": id})
	return `<script>
(function(){
  const cfg = ` + string(cfg) + `;
  document.addEventListener('click', async function(e){
    const button = e.target.closest('[data-hands-action]');
    if (!button) return;
    button.disabled = true;
    try {
      const r = await fetch(cfg.action, {method: 'POST', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({action: 'hands', id: cfg.id, kind: button.dataset.handsAction})});
      if (!r.ok) throw new Error();
      for (const block of document.querySelectorAll('[data-kind="hands"]')) {
        document.dispatchEvent(new CustomEvent('page-block', {detail: {block: block.dataset.block}}));
      }
    } catch (_) {
      if (window.deskToast) window.deskToast('Could not update the speaking queue.', 'error');
    } finally {
      button.disabled = false;
    }
  });
})();
</script>`
}
