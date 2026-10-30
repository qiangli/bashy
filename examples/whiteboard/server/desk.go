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
// /api/action route the page uses, mapped onto the board model. A board is
// the desk item; its id is the board name. open/new/list/put/close return
// id + URL + status only, never strokes or note text; get is the one read
// that returns content.

var deskActions = map[string]bool{"add": true, "open": true, "show": true, "set": true, "list": true, "share": true, "close": true}

func boardStatus(b *Board) string {
	if b.Locked {
		return kit.StatusClosed
	}
	return kit.StatusPending
}

func (a *app) boardURL(r *http.Request, name string) string {
	return kit.ItemURL(kit.RequestOrigin(r)+kit.PublicPrefix(r, a.prefix)+"/", "board", name)
}

func (a *app) openView(r *http.Request, b *Board) map[string]any {
	return kit.OpenView(b.Name, a.boardURL(r, b.Name), boardStatus(b))
}

func cloneBoard(b *Board) Board {
	raw, _ := json.Marshal(b)
	var next Board
	json.Unmarshal(raw, &next)
	if next.Members == nil {
		next.Members = map[string]bool{}
	}
	if next.Invites == nil {
		next.Invites = map[string]bool{}
	}
	return next
}

func (a *app) commit(w http.ResponseWriter, next *Board) bool {
	next.Revision++
	if err := a.save(boardPath(a.dir, next.Name), *next); err != nil {
		log.Printf("save failed: %v", err)
		fail(w, 500, "Could not save. Your change was not accepted; keep your work and ask the facilitator.")
		return false
	}
	*a.boards[next.Name] = *next
	return true
}

func (a *app) loadFor(w http.ResponseWriter, name string) *Board {
	b, err := a.loadLocked(name)
	if err != nil {
		if errors.Is(err, errBoardNotFound) {
			boardNotFound(w)
		} else {
			fail(w, 400, err.Error())
		}
		return nil
	}
	return b
}

// deskNote validates body and appends it as a note at the page's own
// stagger position.
func deskNote(b *Board, body, owner string) (string, bool) {
	body = strings.TrimSpace(body)
	if body == "" || len([]rune(body)) > 500 {
		return "Give a 1-500 character body", false
	}
	if len(b.Notes) >= maxNotes {
		return "This board has reached its 1,000-note limit", false
	}
	k := len(b.Notes)
	b.Notes = append(b.Notes, Note{ID: token(), X: float64(120 + (k*47)%1100), Y: float64(120 + (k*37)%640), Text: body, Color: "#fff3bf", Owner: owner})
	return "", true
}

// a.mu must be held.
// actionReq is the one /api/action request shape (existing board actions
// plus the desk door's title/body/fields).
type actionReq struct {
	Action     string      `json:"action"`
	Board      string      `json:"board"`
	ID         string      `json:"id"`
	Points     []Pt        `json:"points"`
	Color      string      `json:"color"`
	Width      float64     `json:"width"`
	X          float64     `json:"x"`
	Y          float64     `json:"y"`
	Text       string      `json:"text"`
	Locked     bool        `json:"locked"`
	Invite     string      `json:"invite"`
	InviteCode string      `json:"inviteCode"`
	Code       string      `json:"code"`
	Title      string      `json:"title"`
	Body       string      `json:"body"`
	Fields     []kit.Field `json:"fields"`
}

func (a *app) desk(w http.ResponseWriter, r *http.Request, who string, req *actionReq) {
	if len(req.Fields) > 0 {
		if kit.SecretRefused(req.Fields) {
			fail(w, 400, kit.ErrSecret)
		} else {
			fail(w, 400, "Whiteboards take notes, not form fields")
		}
		return
	}
	switch req.Action {
	case "list":
		names, err := filepath.Glob(filepath.Join(a.dir, "boards", "*.json"))
		if err != nil {
			fail(w, 500, "Could not list")
			return
		}
		out := []map[string]any{}
		for _, p := range names {
			b, err := a.loadLocked(strings.TrimSuffix(filepath.Base(p), ".json"))
			if err != nil || !authorized(b, who, a.facilitator(r, b)) {
				continue
			}
			out = append(out, a.openView(r, b))
		}
		sort.Slice(out, func(i, j int) bool { return out[i]["id"].(string) < out[j]["id"].(string) })
		respond(w, out)
		return
	case "add":
		name := req.ID
		if name == "" {
			name = "board-" + token()[:12]
		}
		if !boardNameRe.MatchString(name) {
			fail(w, 400, "Use 1-64 letters, digits, - or _ for a board name")
			return
		}
		if strings.TrimSpace(req.Body) != "" && len([]rune(strings.TrimSpace(req.Body))) > 500 {
			fail(w, 400, "Give a 1-500 character body")
			return
		}
		b, err := a.createLocked(name)
		if err != nil {
			if errors.Is(err, errBoardExists) {
				fail(w, 409, "A board with that name already exists")
			} else {
				log.Printf("create failed: %v", err)
				fail(w, 500, "Could not save. Your change was not accepted; keep your work and ask the facilitator.")
			}
			return
		}
		if strings.TrimSpace(req.Body) != "" {
			next := cloneBoard(b)
			deskNote(&next, req.Body, who)
			if !a.commit(w, &next) {
				return
			}
			b = &next
		}
		v := a.openView(r, b)
		v["ownerKey"] = b.Facilitator
		v["facilitatorKey"] = b.Facilitator
		respond(w, v)
		return
	}
	b := a.loadFor(w, req.ID)
	if b == nil {
		return
	}
	isFac := a.facilitator(r, b)
	if !authorized(b, who, isFac) {
		accessDenied(w)
		return
	}
	switch req.Action {
	case "open":
		respond(w, a.openView(r, b))
	case "show":
		v := exportView(b).(map[string]any)
		v["id"], v["url"], v["status"] = b.Name, a.boardURL(r, b.Name), boardStatus(b)
		respond(w, v)
	case "set":
		if b.Locked {
			fail(w, 409, "This board is closed")
			return
		}
		next := cloneBoard(b)
		if msg, ok := deskNote(&next, req.Body, who); !ok {
			fail(w, 400, msg)
			return
		}
		if a.commit(w, &next) {
			respond(w, a.openView(r, &next))
		}
	case "share":
		if !isFac {
			fail(w, 403, "Only the facilitator can manage invites")
			return
		}
		if !isPrivate(b.Name) {
			fail(w, 400, "The main board is open to everyone; invites are for named boards")
			return
		}
		next := cloneBoard(b)
		code := token()
		next.Invites[code] = true
		if !a.commit(w, &next) {
			return
		}
		v := a.openView(r, &next)
		v["invite"], v["inviteCode"] = code, code
		v["inviteURL"] = a.boardURL(r, next.Name) + "#invite=" + code
		respond(w, v)
	case "close":
		if !isFac {
			fail(w, 403, "Only the facilitator can close the board")
			return
		}
		next := cloneBoard(b)
		next.Locked = true
		if a.commit(w, &next) {
			respond(w, a.openView(r, &next))
		}
	}
}
