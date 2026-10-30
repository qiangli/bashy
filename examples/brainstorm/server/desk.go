package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	kit "desk-kit"
)

// The desk agent door: add|open|show|set|list|share|close over the same
// /api/action route the page uses, mapped onto the session model. A session
// is the desk item; its id is the session name. open/new/list/put/share/close
// return id + URL + status only, never card text; get is the one read that
// returns content, and shows only what the caller could see on the page.

var deskActions = map[string]bool{"add": true, "open": true, "show": true, "set": true, "list": true, "share": true, "close": true}

func sessionStatus(s *Session) string {
	if s.Phase == "closed" {
		return kit.StatusClosed
	}
	return kit.StatusPending
}

func (a *app) sessionURL(r *http.Request, name string) string {
	return kit.ItemURL(kit.RequestOrigin(r)+kit.PublicPrefix(r, a.prefix)+"/", "session", name)
}

func (a *app) openView(r *http.Request, s *Session) map[string]any {
	return kit.OpenView(s.Name, a.sessionURL(r, s.Name), sessionStatus(s))
}

func cloneSession(s *Session) Session {
	raw, _ := json.Marshal(s)
	var next Session
	json.Unmarshal(raw, &next)
	if next.Private {
		if next.Members == nil {
			next.Members = map[string]Member{}
		}
		if next.Invites == nil {
			next.Invites = map[string]string{}
		}
	}
	return next
}

// deskOwner owns cards the facilitator wrote through the desk door. The page
// keeps them private until reveal like any card, but the facilitator's desk
// get reads them back as their own.
const deskOwner = "desk-owner"

// a.mu must be held.
func (a *app) commit(w http.ResponseWriter, next *Session) bool {
	next.Revision++
	if err := saveSession(sessionPath(a.dir, next.Name), *next); err != nil {
		log.Printf("save failed: %v", err)
		fail(w, 500, "Could not save. Your change was not accepted; keep your draft and ask the facilitator.")
		return false
	}
	*a.sessions[next.Name] = *next
	return true
}

// deskCard validates body and appends it as an anonymous card in column 0.
func deskCard(s *Session, body, owner string) (string, bool) {
	body = strings.TrimSpace(body)
	if body == "" || len([]rune(body)) > 1000 {
		return "Give a 1-1000 character body", false
	}
	if len(s.Cards) >= 1000 {
		return "This session has reached its 1,000-card limit", false
	}
	s.Cards = append(s.Cards, Card{ID: token(), Column: 0, Text: body, Name: "Anonymous", Owner: owner, Votes: map[string]bool{}})
	return "", true
}

// actionReq is the one /api/action request shape: the existing session
// actions plus the desk door's body/fields.
type actionReq struct {
	Action     string      `json:"action"`
	Session    string      `json:"session"`
	ID         string      `json:"id"`
	Column     int         `json:"column"`
	Text       string      `json:"text"`
	Name       string      `json:"name"`
	Phase      string      `json:"phase"`
	Title      string      `json:"title"`
	Columns    []string    `json:"columns"`
	Prompts    []string    `json:"prompts"`
	VoteBudget int         `json:"voteBudget"`
	Private    bool        `json:"private"`
	Invite     string      `json:"invite"`
	Body       string      `json:"body"`
	Fields     []kit.Field `json:"fields"`
}

// a.mu must be held.
func (a *app) desk(w http.ResponseWriter, r *http.Request, who string, req *actionReq) {
	if len(req.Fields) > 0 {
		if kit.SecretRefused(req.Fields) {
			fail(w, 400, kit.ErrSecret)
		} else {
			fail(w, 400, "Brainstorm sessions take ideas, not form fields")
		}
		return
	}
	switch req.Action {
	case "list":
		names, err := filepath.Glob(filepath.Join(a.dir, "sessions", "*.json"))
		if err != nil {
			fail(w, 500, "Could not list")
			return
		}
		out := []map[string]any{}
		for _, p := range names {
			s, err := a.loadLocked(strings.TrimSuffix(filepath.Base(p), ".json"))
			if err != nil || !s.allowed(who, a.facilitator(r, s)) {
				continue
			}
			out = append(out, a.openView(r, s))
		}
		sort.Slice(out, func(i, j int) bool { return out[i]["id"].(string) < out[j]["id"].(string) })
		respond(w, out)
		return
	case "add":
		name := req.ID
		if name == "" {
			name = "session-" + token()[:12]
		}
		if !sessionNameRe.MatchString(name) {
			fail(w, 400, "Use 1-64 letters, digits, - or _ for a session name")
			return
		}
		title := strings.TrimSpace(req.Title)
		if title == "" {
			title = "Team Brainstorm"
		}
		if len([]rune(title)) > 80 {
			fail(w, 400, "Keep the title within 80 characters")
			return
		}
		if body := strings.TrimSpace(req.Body); body != "" && len([]rune(body)) > 1000 {
			fail(w, 400, "Give a 1-1000 character body")
			return
		}
		s := &Session{Version: 2, Name: name, Title: title, Columns: append([]string{}, defaultColumns...),
			Prompts: append([]string{}, defaultPrompts...), VoteBudget: defaultVoteBudget, Facilitator: token(), Phase: "write", Cards: []Card{},
			Private: true, Members: map[string]Member{}, Invites: map[string]string{}}
		if strings.TrimSpace(req.Body) != "" {
			deskCard(s, req.Body, deskOwner)
		}
		if _, err := a.createLocked(s); err != nil {
			if errors.Is(err, errSessionExists) {
				fail(w, 409, "A session with that name already exists")
			} else {
				log.Printf("create failed: %v", err)
				fail(w, 500, "Could not save. Try again.")
			}
			return
		}
		v := a.openView(r, s)
		v["ownerKey"] = s.Facilitator
		v["facilitatorKey"] = s.Facilitator
		respond(w, v)
		return
	}
	s, err := a.loadLocked(req.ID)
	if err != nil {
		if errors.Is(err, errSessionNotFound) {
			sessionNotFound(w)
		} else {
			fail(w, 400, err.Error())
		}
		return
	}
	isFac := a.facilitator(r, s)
	if !s.allowed(who, isFac) {
		sessionNotFound(w)
		return
	}
	switch req.Action {
	case "open":
		respond(w, a.openView(r, s))
	case "show":
		seen := cloneSession(s)
		if isFac {
			for i := range seen.Cards {
				if seen.Cards[i].Owner == deskOwner {
					seen.Cards[i].Owner = who
				}
			}
		}
		v := a.view(who, &seen, isFac)
		v["id"], v["url"], v["status"] = s.Name, a.sessionURL(r, s.Name), sessionStatus(s)
		respond(w, v)
	case "set":
		if s.Phase != "write" {
			fail(w, 409, "Writing has ended")
			return
		}
		next := cloneSession(s)
		owner := who
		if isFac {
			owner = deskOwner
		}
		if msg, ok := deskCard(&next, req.Body, owner); !ok {
			fail(w, 400, msg)
			return
		}
		if a.commit(w, &next) {
			respond(w, a.openView(r, &next))
		}
	case "share":
		if !isFac {
			fail(w, 403, "Facilitator link required")
			return
		}
		if !s.Private {
			fail(w, 409, "This session is open to everyone with the link")
			return
		}
		next := cloneSession(s)
		code := token()
		next.Invites[code] = strings.TrimSpace(req.Name)
		if !a.commit(w, &next) {
			return
		}
		v := a.openView(r, &next)
		v["invite"], v["inviteCode"] = code, code
		v["inviteURL"] = a.sessionURL(r, next.Name) + "#invite=" + code
		respond(w, v)
	case "close":
		if !isFac {
			fail(w, 403, "Facilitator link required")
			return
		}
		next := cloneSession(s)
		next.Phase = "closed"
		if a.commit(w, &next) {
			respond(w, a.openView(r, &next))
		}
	}
}
