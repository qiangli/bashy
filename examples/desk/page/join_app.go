package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"desk-kit"
)

//go:embed shaped.json
var pageManifest []byte

type PageItem = item

type participantMap map[string]*Participant

// Accept participant rows saved by the earlier page store as well as keyed
// rows. Existing anonymous rows retain their handle and address binding.
func (ps *participantMap) UnmarshalJSON(raw []byte) error {
	if len(raw) == 0 || raw[0] != '[' {
		type plain participantMap
		return json.Unmarshal(raw, (*plain)(ps))
	}
	var rows []Participant
	if err := json.Unmarshal(raw, &rows); err != nil {
		return err
	}
	*ps = make(participantMap, len(rows))
	for _, row := range rows {
		key := row.Principal
		if key == "" {
			key = anonymousKey(row.Handle, row.Address)
		}
		(*ps)[key] = &row
	}
	return nil
}

// byCodeLocked checks the on-disk store, including pages not opened since
// restart. Creation and code assignment hold the same lock.
func (a *app) byCodeLocked(code string) (*item, error) {
	entries, err := os.ReadDir(itemDir(a.dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		it, err := a.loadLocked(id)
		if err != nil {
			return nil, err
		}
		if it.JoinCode != "" && it.JoinCode == code {
			return it, nil
		}
	}
	return nil, nil
}

func (a *app) assignCodeLocked(raw string) (string, error) {
	var scanErr error
	code, err := chooseJoinCode(raw, func(code string) bool {
		it, err := a.byCodeLocked(code)
		scanErr = err
		return it != nil
	})
	if scanErr != nil {
		return "", scanErr
	}
	return code, err
}

func (a *app) joinPage(w http.ResponseWriter, r *http.Request, path string) {
	publicRoot := kit.PublicPrefix(r, a.prefix)
	id := r.URL.Query().Get("id")
	if path == "/" && id == "" {
		joinRoot(w, r, publicRoot, func(code string) bool {
			a.mu.Lock()
			defer a.mu.Unlock()
			it, err := a.byCodeLocked(code)
			return err == nil && it != nil && a.canView(r, it)
		})
		return
	}
	a.mu.Lock()
	var it *item
	if path != "/" {
		it, _ = a.byCodeLocked(pageJoinCode(path))
	} else {
		it, _ = a.loadLocked(id)
	}
	if it == nil {
		a.mu.Unlock()
		http.Error(w, "Page not found", http.StatusNotFound)
		return
	}
	if !a.viewOr403(w, r, it) {
		a.mu.Unlock()
		return
	}
	id, title, code := it.ID, it.Title, it.JoinCode
	owner := a.canOrganize(r, it)
	a.mu.Unlock()
	canonical := publicRoot + "/?id=" + id
	if code != "" {
		canonical = pageCodeURL(publicRoot, code)
	}
	face := faceFor(r, owner)
	if r.Method == http.MethodGet && !owner && r.URL.Query().Get("view") == "organizer" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(kit.Page(tool, "Page", organizerEntry(publicRoot, id)))
		return
	}
	render := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		q.Set("id", id)
		r.URL.RawQuery = q.Encode()
		status, body := a.page(r)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	})
	if r.Method == http.MethodGet && (owner || face == displayFace) {
		if owner && face == participantFace {
			r = a.previewParticipant(w, r, it)
		}
		render.ServeHTTP(w, r)
		return
	}
	a.joinOptions.pageJoin(w, r, a.updateParticipant(r, id), title, canonical, publicRoot+"/", true, render)
}

func (a *app) updateParticipant(r *http.Request, id string) func(func(*PageItem) error) error {
	return func(update func(*PageItem) error) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		it, err := a.loadLocked(id)
		if err != nil {
			return err
		}
		if !a.canView(r, it) {
			return errJoinAudience
		}
		next := it.clone()
		if err := update(&next); err != nil {
			return err
		}
		if len(next.Participants) != len(it.Participants) {
			return a.saveLocked(&next, "here")
		}
		// LastSeen refreshes are durable without generating a revision.
		return a.persistLocked(&next)
	}
}

// knownParticipantLocked attaches enrolled identities to metadata and agent
// actions. Requests without an enrollment may still read kit metadata.
func (a *app) knownParticipantLocked(w http.ResponseWriter, r *http.Request, it *item) (*http.Request, func(), bool) {
	now := time.Now()
	c := identifyParticipant(r, a.joinOptions.consolePrincipal, a.joinOptions.outpostSecret, now)
	key := c.principal
	if key == "" {
		key = participantCookieKey(it, c)
	}
	if p := it.Participants[key]; p != nil {
		if p.Kicked {
			if a.isOwner(r, it) {
				return r, func() {}, true
			}
			kit.Fail(w, http.StatusForbidden, errKicked.Error())
			return r, func() {}, false
		}
		next := it.clone()
		if _, err := resolveParticipant(&next, c, nil, a.joinOptions.suffix, now); err == nil {
			if err := a.persistLocked(&next); err != nil {
				log.Printf("page %q: could not save last seen: %v", it.ID, err)
			}
			return withParticipant(r, key, *next.Participants[key]), traceParticipant(r, *next.Participants[key], now), true
		}
	}
	return r, func() {}, true
}

func (a *app) participantAction(w http.ResponseWriter, r *http.Request, it *item, req actionRequest) {
	if !a.isOwner(r, it) {
		kit.Fail(w, http.StatusForbidden, "Only the organizer can manage participants")
		return
	}
	if req.Kind == "list" {
		// The local facilitator can inspect a full row and its exact key to
		// disambiguate duplicate display handles during moderation.
		kit.WriteJSON(w, map[string]any{"participants": it.Participants})
		return
	}
	source, err := moderatorKey(it, req.Source)
	if req.Kind == "open" {
		source, err = "", nil
	}
	target := req.Target
	if err == nil && req.Kind == "merge" {
		target, err = moderatorKey(it, req.Target)
	}
	next := it.clone()
	if err == nil {
		err = moderateParticipant(&next, req.Kind, source, target)
	}
	if err != nil {
		kit.Fail(w, joinStatus(err), err.Error())
		return
	}
	if err := a.saveLocked(&next, "here"); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save participant change")
		return
	}
	log.Print(moderationMessage(it.ID, req.Kind, source, target))
	kit.WriteJSON(w, a.openView(r, &next))
}

func moderatorKey(it *item, input string) (string, error) {
	if p := it.Participants[input]; p != nil && !p.Kicked {
		return input, nil
	}
	match := ""
	for key, p := range it.Participants {
		if p != nil && !p.Kicked && strings.EqualFold(input, p.Handle) {
			if match != "" {
				return "", fmt.Errorf("more than one participant uses that handle; provide the full participant key")
			}
			match = key
		}
	}
	if match == "" {
		return "", fmt.Errorf("participant not found")
	}
	return match, nil
}

func escapeParticipantMarkdown(handle string) string {
	return strings.NewReplacer("\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_",
		"[", "\\[", "]", "\\]", "<", "&lt;", ">", "&gt;", "&", "&amp;").Replace(handle)
}
