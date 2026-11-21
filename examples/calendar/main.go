// Command calendar is the bashy desk shared calendar MVP: self-hosted event
// scheduling with explicit IANA timezones, day/week agenda page, Desk
// owner/invite/member audience (404 for non-members and revoked members),
// Git-directory save/load, RFC 5545 ICS import/export, and Google Calendar
// import/export composing examples/google.
//
// Creating, updating, or importing an event NEVER executes commands or launches
// agents. Storage: $BASHY_HOME/desk/calendar/items/<id>.json.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata"

	kit "desk-kit"
	"desk-kit/render"
)

const (
	tool            = "calendar"
	StatusCancelled = "cancelled"
)

// breakMode is set from -break on serve for contract red/green tests; empty in normal runs.
var breakMode string

//go:embed templates/agenda.html
var shippedAgendaHTML string

//go:embed formats/event.schema.json
var shippedEventSchema []byte

func renderMarkdown(src []byte) ([]byte, error) {
	return render.Markdown(src, render.Options{})
}

type eventSource struct {
	Provider string `json:"provider"`
	UID      string `json:"uid"`
}

type event struct {
	Version  int             `json:"version"`
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Start    string          `json:"start"`
	End      string          `json:"end"`
	TZ       string          `json:"tz"`
	Location string          `json:"location"`
	Body     string          `json:"body"`
	Refs     []string        `json:"refs"`
	Audience []string        `json:"audience"`
	Status   string          `json:"status"`
	Source   eventSource     `json:"source"`
	Fields   []kit.Field     `json:"fields,omitempty"`
	Revision int             `json:"revision"`
	Owner    string          `json:"owner"`
	Members  map[string]bool `json:"members"`
	Invites  map[string]bool `json:"invites"`
	Created  string          `json:"created"`
	Updated  string          `json:"updated"`
}

// portableEvent is the capability-free record written to events.json and JSON exports.
type portableEvent struct {
	ID       string      `json:"id"`
	Title    string      `json:"title"`
	Start    string      `json:"start"`
	End      string      `json:"end"`
	TZ       string      `json:"tz"`
	Location string      `json:"location"`
	Body     string      `json:"body"`
	Refs     []string    `json:"refs"`
	Audience []string    `json:"audience"`
	Status   string      `json:"status"`
	Source   eventSource `json:"source"`
	Created  string      `json:"created"`
	Updated  string      `json:"updated"`
}

// upsertSpec is the wire shape for import (ICS/Google) and load (Git dir).
type upsertSpec struct {
	ID       string      `json:"id,omitempty"`
	Title    string      `json:"title"`
	Start    string      `json:"start"`
	End      string      `json:"end"`
	TZ       string      `json:"tz"`
	Location string      `json:"location,omitempty"`
	Body     string      `json:"body,omitempty"`
	Refs     []string    `json:"refs,omitempty"`
	Audience []string    `json:"audience,omitempty"`
	Status   string      `json:"status,omitempty"`
	Source   eventSource `json:"source"`
	Created  string      `json:"created,omitempty"`
	Updated  string      `json:"updated,omitempty"`
}

type actionRequest struct {
	Action   string       `json:"action"`
	ID       string       `json:"id,omitempty"`
	Title    *string      `json:"title,omitempty"`
	Start    *string      `json:"start,omitempty"`
	End      *string      `json:"end,omitempty"`
	TZ       *string      `json:"tz,omitempty"`
	Location *string      `json:"location,omitempty"`
	Body     *string      `json:"body,omitempty"`
	Refs     *[]string    `json:"refs,omitempty"`
	Audience *[]string    `json:"audience,omitempty"`
	Status   *string      `json:"status,omitempty"`
	Source   *eventSource `json:"source,omitempty"`
	Fields   []kit.Field  `json:"fields,omitempty"`
	Invite   string       `json:"invite,omitempty"`
	Revoke   string       `json:"revoke,omitempty"`
	From     string       `json:"from,omitempty"`
	To       string       `json:"to,omitempty"`
	Output   string       `json:"output,omitempty"`
	All      bool         `json:"all,omitempty"`
	ICS      string       `json:"ics,omitempty"`
	Events   []upsertSpec `json:"events,omitempty"`
	DryRun   bool         `json:"dry_run,omitempty"`
}

type app struct {
	mu      sync.Mutex
	dir     string
	srcDir  string
	prefix  string
	baseURL string
	events  map[string]*event
}

func (ev *event) clone() event {
	c := *ev
	c.Refs = append([]string{}, ev.Refs...)
	c.Audience = append([]string{}, ev.Audience...)
	c.Fields = append([]kit.Field(nil), ev.Fields...)
	c.Members = make(map[string]bool, len(ev.Members))
	for k, v := range ev.Members {
		c.Members[k] = v
	}
	c.Invites = make(map[string]bool, len(ev.Invites))
	for k, v := range ev.Invites {
		c.Invites[k] = v
	}
	return c
}

func (ev *event) portable() portableEvent {
	refs := ev.Refs
	if refs == nil {
		refs = []string{}
	}
	aud := ev.Audience
	if aud == nil {
		aud = []string{}
	}
	return portableEvent{
		ID:       ev.ID,
		Title:    ev.Title,
		Start:    ev.Start,
		End:      ev.End,
		TZ:       ev.TZ,
		Location: ev.Location,
		Body:     ev.Body,
		Refs:     refs,
		Audience: aud,
		Status:   ev.Status,
		Source:   ev.Source,
		Created:  ev.Created,
		Updated:  ev.Updated,
	}
}

func eventPath(dir, id string) string { return filepath.Join(dir, "items", id+".json") }

func validEventStatus(s string) bool {
	switch s {
	case kit.StatusPending, kit.StatusClosed, StatusCancelled:
		return true
	}
	return false
}

func validSourceProvider(p string) bool {
	switch p {
	case "local", "ics", "google":
		return true
	}
	return false
}

// validateTZ enforces an explicit IANA timezone name via time.LoadLocation.
func validateTZ(tz string) (*time.Location, error) {
	if tz == "" || strings.TrimSpace(tz) != tz || strings.EqualFold(tz, "local") ||
		strings.HasPrefix(tz, "/") || strings.Contains(tz, "..") || strings.Contains(tz, `\`) {
		return nil, fmt.Errorf("invalid IANA timezone %q", tz)
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("invalid IANA timezone %q: %v", tz, err)
	}
	return loc, nil
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }

func appendUnique(list []string, val string) []string {
	for _, x := range list {
		if x == val {
			return list
		}
	}
	return append(list, val)
}

func cleanStringList(in []string) []string {
	out := []string{}
	for _, s := range in {
		if v := strings.TrimSpace(s); v != "" {
			out = appendUnique(out, v)
		}
	}
	return out
}

// ---- Shaping hooks: templates/agenda.html & formats/event.schema.json ------

func (a *app) readAgendaTemplate() string {
	candidates := []string{}
	if a.dir != "" {
		candidates = append(candidates, filepath.Join(a.dir, "templates", "agenda.html"))
	}
	if a.srcDir != "" {
		candidates = append(candidates, filepath.Join(a.srcDir, "templates", "agenda.html"))
	}
	candidates = append(candidates, filepath.Join("templates", "agenda.html"))
	for _, p := range candidates {
		if raw, err := os.ReadFile(p); err == nil && len(bytes.TrimSpace(raw)) > 0 {
			return string(raw)
		}
	}
	return shippedAgendaHTML
}

type jsonSchema struct {
	Type                 string                 `json:"type"`
	Description          string                 `json:"description,omitempty"`
	Required             []string               `json:"required,omitempty"`
	AdditionalProperties *bool                  `json:"additionalProperties,omitempty"`
	Properties           map[string]*jsonSchema `json:"properties,omitempty"`
	Items                *jsonSchema            `json:"items,omitempty"`
	Enum                 []string               `json:"enum,omitempty"`
	MaxLength            int                    `json:"maxLength,omitempty"`
	MaxItems             int                    `json:"maxItems,omitempty"`
	Format               string                 `json:"format,omitempty"`
}

func (a *app) loadEventSchema() (*jsonSchema, error) {
	var raw []byte
	candidates := []string{}
	if a.dir != "" {
		candidates = append(candidates, filepath.Join(a.dir, "formats", "event.schema.json"))
	}
	if a.srcDir != "" {
		candidates = append(candidates, filepath.Join(a.srcDir, "formats", "event.schema.json"))
	}
	candidates = append(candidates, filepath.Join("formats", "event.schema.json"))
	for _, p := range candidates {
		if b, err := os.ReadFile(p); err == nil && len(bytes.TrimSpace(b)) > 0 {
			raw = b
			break
		}
	}
	if len(raw) == 0 {
		raw = shippedEventSchema
	}
	s := &jsonSchema{}
	if err := json.Unmarshal(raw, s); err != nil {
		return nil, fmt.Errorf("invalid formats/event.schema.json: %v", err)
	}
	return s, nil
}

func (a *app) validateAgainstSchema(ev *event) error {
	s, err := a.loadEventSchema()
	if err != nil {
		return err
	}
	b, _ := json.Marshal(ev.portable())
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return checkSchemaValue(s, m, "event")
}

func checkSchemaValue(s *jsonSchema, v any, path string) error {
	if s == nil {
		return nil
	}
	switch s.Type {
	case "object":
		m, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: expected object", path)
		}
		for _, req := range s.Required {
			if _, ok := m[req]; !ok {
				return fmt.Errorf("%s.%s: required", path, req)
			}
		}
		for k, val := range m {
			ps, ok := s.Properties[k]
			if !ok {
				if s.AdditionalProperties != nil && !*s.AdditionalProperties {
					return fmt.Errorf("%s.%s: unknown property", path, k)
				}
				continue
			}
			if err := checkSchemaValue(ps, val, path+"."+k); err != nil {
				return err
			}
		}
	case "array":
		arr, ok := v.([]any)
		if !ok {
			return fmt.Errorf("%s: expected array", path)
		}
		if s.MaxItems > 0 && len(arr) > s.MaxItems {
			return fmt.Errorf("%s: exceeds maxItems (%d)", path, s.MaxItems)
		}
		for i, elem := range arr {
			if err := checkSchemaValue(s.Items, elem, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case "string":
		str, ok := v.(string)
		if !ok {
			return fmt.Errorf("%s: expected string", path)
		}
		if s.MaxLength > 0 && len(str) > s.MaxLength {
			return fmt.Errorf("%s: exceeds maxLength (%d)", path, s.MaxLength)
		}
		if len(s.Enum) > 0 {
			found := false
			for _, e := range s.Enum {
				if e == str {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("%s: must be one of %v, got %q", path, s.Enum, str)
			}
		}
	}
	return nil
}

// ---- Storage & Access Control ----------------------------------------------

func (a *app) loadLocked(id string) (*event, error) {
	if !kit.ValidName(id) {
		return nil, fmt.Errorf("use 1-64 letters, digits, - or _ for an id")
	}
	if a.events == nil {
		a.events = map[string]*event{}
	}
	if ev, ok := a.events[id]; ok {
		return ev, nil
	}
	raw, err := os.ReadFile(eventPath(a.dir, id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	ev := &event{}
	if err := json.Unmarshal(raw, ev); err != nil {
		return nil, fmt.Errorf("saved event %q is invalid; preserve it for recovery", id)
	}
	if ev.Version != 1 || ev.ID != id || !validEventStatus(ev.Status) {
		return nil, fmt.Errorf("unsupported or invalid saved event %q", id)
	}
	if ev.Refs == nil {
		ev.Refs = []string{}
	}
	if ev.Audience == nil {
		ev.Audience = []string{}
	}
	if ev.Members == nil {
		ev.Members = map[string]bool{}
	}
	if ev.Invites == nil {
		ev.Invites = map[string]bool{}
	}
	if ev.Source.Provider == "" {
		ev.Source.Provider = "local"
	}
	a.events[id] = ev
	return ev, nil
}

func (a *app) saveLocked(ev *event) error {
	ev.Revision++
	ev.Updated = nowUTC()
	if ev.Created == "" {
		ev.Created = ev.Updated
	}
	if ev.Refs == nil {
		ev.Refs = []string{}
	}
	if ev.Audience == nil {
		ev.Audience = []string{}
	}
	raw, err := kit.MarshalIndent(*ev)
	if err != nil {
		return err
	}
	if err := kit.AtomicWrite(eventPath(a.dir, ev.ID), raw); err != nil {
		return err
	}
	if a.events == nil {
		a.events = map[string]*event{}
	}
	a.events[ev.ID] = ev
	return nil
}

func (a *app) allIDsLocked() ([]string, error) {
	names, err := filepath.Glob(filepath.Join(a.dir, "items", "*.json"))
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var ids []string
	for _, p := range names {
		id := strings.TrimSuffix(filepath.Base(p), ".json")
		if kit.ValidName(id) && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for id := range a.events {
		if kit.ValidName(id) && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// isOwner compares X-Facilitator-Key (supporting comma-separated local keys
// for multi-item owner CLI verbs like list/save/export --all) in constant time.
func (a *app) isOwner(r *http.Request, ev *event) bool {
	if ev.Owner == "" {
		return false
	}
	raw := kit.FacilitatorKey(r)
	if raw == "" {
		return false
	}
	for _, candidate := range strings.Split(raw, ",") {
		c := strings.TrimSpace(candidate)
		if c != "" && kit.Grant(c, ev.Owner) {
			return true
		}
	}
	return false
}

func memberToken(w http.ResponseWriter, r *http.Request) string {
	if hdr := strings.TrimSpace(r.Header.Get("X-Calendar-Member")); kit.ValidToken(hdr) {
		http.SetCookie(w, &http.Cookie{
			Name: "calendar-participant", Value: hdr, Path: "/",
			HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 86400 * 30,
		})
		return hdr
	}
	return kit.ParticipantCookie(w, r, "calendar-participant")
}

func (a *app) hasReadAccess(r *http.Request, ev *event, me string) bool {
	return a.isOwner(r, ev) || ev.Members[me]
}

func (a *app) base(r *http.Request) string { return kit.PublicBase(r, a.baseURL) }

func (a *app) urlFor(r *http.Request, id string) string {
	return kit.ItemURL(a.base(r)+"/", "id", id)
}

// openView is strictly contentless: id, url, status ONLY (never body).
func (a *app) openView(r *http.Request, ev *event) map[string]any {
	v := kit.OpenView(ev.ID, a.urlFor(r, ev.ID), ev.Status)
	if breakMode == "open-emits-content" {
		v["body"] = ev.Body
	}
	return v
}

// agendaItemView projects non-body agenda metadata converted into viewerTZ.
func (a *app) agendaItemView(r *http.Request, ev *event, viewerLoc *time.Location, viewerTZ string) map[string]any {
	loc := viewerLoc
	tzName := viewerTZ
	if loc == nil {
		if evLoc, err := validateTZ(ev.TZ); err == nil {
			loc = evLoc
			tzName = ev.TZ
		} else {
			loc = time.UTC
			tzName = "UTC"
		}
	}
	startT, _ := time.Parse(time.RFC3339, ev.Start)
	endT, _ := time.Parse(time.RFC3339, ev.End)
	localStart := startT.In(loc)
	localEnd := endT.In(loc)

	v := map[string]any{
		"id":            ev.ID,
		"url":           a.urlFor(r, ev.ID),
		"status":        ev.Status,
		"title":         ev.Title,
		"start":         ev.Start,
		"end":           ev.End,
		"tz":            ev.TZ,
		"display_tz":    tzName,
		"display_start": localStart.Format(time.RFC3339),
		"display_end":   localEnd.Format(time.RFC3339),
		"date":          localStart.Format("2006-01-02"),
		"location":      ev.Location,
		"refs":          ev.Refs,
		"audience":      ev.Audience,
		"source":        ev.Source,
	}
	if breakMode == "open-emits-content" {
		v["body"] = ev.Body
	}
	return v
}

func (a *app) detailView(r *http.Request, ev *event, viewerLoc *time.Location, viewerTZ string) map[string]any {
	v := a.agendaItemView(r, ev, viewerLoc, viewerTZ)
	v["body"] = ev.Body
	v["created"] = ev.Created
	v["updated"] = ev.Updated
	v["revision"] = ev.Revision
	if len(ev.Fields) > 0 {
		v["fields"] = ev.Fields
	}
	if a.isOwner(r, ev) {
		members := make([]string, 0, len(ev.Members))
		for m, ok := range ev.Members {
			if ok {
				members = append(members, m)
			}
		}
		sort.Strings(members)
		v["members"] = members
	}
	return v
}

// ---- Time Window Validation ------------------------------------------------

// parseEventTimes validates explicit IANA tz + RFC 3339 start/end (end >= start).
// When omitted on add, tz defaults to "UTC", start to deterministic 2026-01-01T09:00:00Z,
// and end to start + 1h — NEVER derived from time.Now() or created/updated.
func parseEventTimes(tzPtr, startPtr, endPtr *string) (tz, startStr, endStr string, err error) {
	tz = "UTC"
	if tzPtr != nil {
		tz = *tzPtr
	}
	loc, err := validateTZ(tz)
	if err != nil {
		return "", "", "", err
	}

	var startT time.Time
	if startPtr == nil {
		startT = time.Date(2026, 1, 1, 9, 0, 0, 0, loc)
		startStr = startT.Format(time.RFC3339)
	} else {
		if strings.TrimSpace(*startPtr) == "" {
			return "", "", "", errors.New("start must be a valid RFC 3339 timestamp")
		}
		t, pErr := time.Parse(time.RFC3339, *startPtr)
		if pErr != nil {
			return "", "", "", fmt.Errorf("invalid start timestamp %q (expected RFC 3339): %v", *startPtr, pErr)
		}
		startT = t
		startStr = *startPtr
	}

	var endT time.Time
	if endPtr == nil {
		endT = startT.Add(time.Hour)
		endStr = endT.In(loc).Format(time.RFC3339)
	} else {
		if strings.TrimSpace(*endPtr) == "" {
			return "", "", "", errors.New("end must be a valid RFC 3339 timestamp")
		}
		t, pErr := time.Parse(time.RFC3339, *endPtr)
		if pErr != nil {
			return "", "", "", fmt.Errorf("invalid end timestamp %q (expected RFC 3339): %v", *endPtr, pErr)
		}
		endT = t
		endStr = *endPtr
	}

	if endT.Before(startT) {
		return "", "", "", errors.New("end must be greater than or equal to start")
	}
	return tz, startStr, endStr, nil
}

func parseFilterBound(raw string, loc *time.Location, isEnd bool) (time.Time, error) {
	if loc == nil {
		loc = time.UTC
	}
	if len(raw) == 10 && raw[4] == '-' && raw[7] == '-' {
		d, err := time.ParseInLocation("2006-01-02", raw, loc)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid date %q (expected YYYY-MM-DD or RFC 3339)", raw)
		}
		if isEnd {
			return d.AddDate(0, 0, 1).Add(-time.Nanosecond), nil
		}
		return d, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timestamp %q (expected RFC 3339 or YYYY-MM-DD)", raw)
	}
	return t, nil
}

// ---- HTTP Handlers ---------------------------------------------------------

func (a *app) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if breakMode == "no-schema-version" {
		a.serveHTTP(w, r)
		return
	}
	kit.ContractHandler(tool, http.HandlerFunc(a.serveHTTP)).ServeHTTP(w, r)
}

func (a *app) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if breakMode != "no-csp" {
		kit.ApplySecurityHeaders(w)
	}
	if !kit.GuardPOST(w, r, tool) {
		return
	}
	path, ok := kit.StripPrefix(a.prefix, r.URL.Path)
	if !ok {
		kit.Fail(w, http.StatusNotFound, "Not found")
		return
	}

	switch {
	case path == "/" && r.Method == "GET":
		me := memberToken(w, r)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(a.page(r, me))
		return

	case path == "/healthz" && r.Method == "GET":
		kit.WriteJSON(w, map[string]bool{"ok": true})
		return

	case path == "/api/render" && breakMode != "no-render":
		kit.ServeRender(w, r, renderMarkdown)
		return

	case path == "/api/state" && r.Method == "GET":
		a.mu.Lock()
		defer a.mu.Unlock()
		me := memberToken(w, r)
		id := r.URL.Query().Get("id")
		if !kit.ValidName(id) {
			kit.Fail(w, http.StatusBadRequest, "Use 1-64 letters, digits, - or _ for an id")
			return
		}
		ev, err := a.loadLocked(id)
		if err != nil {
			if os.IsNotExist(err) {
				kit.Fail(w, http.StatusNotFound, "Event not found")
			} else {
				kit.Fail(w, http.StatusBadRequest, err.Error())
			}
			return
		}
		if !a.hasReadAccess(r, ev, me) {
			kit.Fail(w, http.StatusNotFound, "Event not found")
			return
		}
		kit.WriteJSON(w, a.openView(r, ev))
		return

	case path == "/api/export" && r.Method == "GET":
		a.mu.Lock()
		defer a.mu.Unlock()
		me := memberToken(w, r)
		a.serveGETExport(w, r, me)
		return

	case path == "/api/action" && r.Method != "POST":
		kit.Fail(w, http.StatusMethodNotAllowed, "Method not supported")
		return

	case path != "/api/action":
		kit.Fail(w, http.StatusNotFound, "Not found")
		return
	}

	var req actionRequest
	if !kit.DecodeAction(w, r, 10<<20, &req) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	me := memberToken(w, r)

	switch req.Action {
	case "add":
		a.handleAdd(w, r, req, me)
	case "open":
		a.handleOpen(w, r, req, me)
	case "show":
		a.handleShow(w, r, req, me)
	case "set":
		a.handleSet(w, r, req, me)
	case "cancel":
		a.handleStatusChange(w, r, req, me, StatusCancelled)
	case "close":
		a.handleStatusChange(w, r, req, me, kit.StatusClosed)
	case "list":
		a.handleList(w, r, req, me)
	case "share":
		a.handleShare(w, r, req, me)
	case "join":
		a.handleJoin(w, r, req, me)
	case "export":
		a.handleExport(w, r, req, me)
	case "import":
		a.handleImport(w, r, req, me)
	case "load":
		a.handleLoad(w, r, req, me)
	default:
		kit.Fail(w, http.StatusBadRequest, "Unknown action")
	}
}

func (a *app) loadAuthorized(w http.ResponseWriter, r *http.Request, id, me string) (*event, bool) {
	if !kit.ValidName(id) {
		kit.Fail(w, http.StatusBadRequest, "Use 1-64 letters, digits, - or _ for an id")
		return nil, false
	}
	ev, err := a.loadLocked(id)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, http.StatusNotFound, "Event not found")
		} else {
			kit.Fail(w, http.StatusBadRequest, err.Error())
		}
		return nil, false
	}
	// Non-members and revoked members always receive HTTP 404 (never 403).
	if !a.hasReadAccess(r, ev, me) {
		kit.Fail(w, http.StatusNotFound, "Event not found")
		return nil, false
	}
	return ev, true
}

func (a *app) handleAdd(w http.ResponseWriter, r *http.Request, req actionRequest, me string) {
	if breakMode != "allow-secret" && kit.SecretRefused(req.Fields) {
		kit.Fail(w, http.StatusBadRequest, kit.ErrSecret)
		return
	}
	id := req.ID
	if id == "" {
		id = "evt-" + kit.NewToken()[:12]
	}
	if !kit.ValidName(id) {
		kit.Fail(w, http.StatusBadRequest, "Use 1-64 letters, digits, - or _ for an id")
		return
	}
	if _, ok := a.events[id]; ok {
		kit.Fail(w, http.StatusConflict, "An event with that id already exists")
		return
	}
	if _, err := os.Stat(eventPath(a.dir, id)); err == nil {
		kit.Fail(w, http.StatusConflict, "An event with that id already exists")
		return
	}

	tz, startStr, endStr, err := parseEventTimes(req.TZ, req.Start, req.End)
	if err != nil {
		kit.Fail(w, http.StatusBadRequest, err.Error())
		return
	}

	title := "Untitled event"
	if req.Title != nil && strings.TrimSpace(*req.Title) != "" {
		title = strings.TrimSpace(*req.Title)
	}
	body := ""
	if req.Body != nil {
		body = *req.Body
	}
	locStr := ""
	if req.Location != nil {
		locStr = strings.TrimSpace(*req.Location)
	}
	refs := []string{}
	if req.Refs != nil {
		refs = cleanStringList(*req.Refs)
	}
	aud := []string{}
	if req.Audience != nil {
		aud = cleanStringList(*req.Audience)
	}
	src := eventSource{Provider: "local", UID: id}
	if req.Source != nil {
		if !validSourceProvider(req.Source.Provider) {
			kit.Fail(w, http.StatusBadRequest, fmt.Sprintf("invalid source.provider %q", req.Source.Provider))
			return
		}
		src = *req.Source
	}

	ts := nowUTC()
	ev := &event{
		Version:  1,
		ID:       id,
		Title:    title,
		Start:    startStr,
		End:      endStr,
		TZ:       tz,
		Location: locStr,
		Body:     body,
		Refs:     refs,
		Audience: aud,
		Status:   kit.StatusPending,
		Source:   src,
		Fields:   req.Fields,
		Owner:    kit.NewToken(),
		Members:  map[string]bool{me: true},
		Invites:  map[string]bool{},
		Created:  ts,
		Updated:  ts,
	}
	if err := a.validateAgainstSchema(ev); err != nil {
		kit.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.DryRun {
		v := a.openView(r, ev)
		v["dry_run"] = true
		v["verb"] = "add"
		kit.WriteJSON(w, v)
		return
	}
	if err := a.saveLocked(ev); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
		return
	}
	v := a.openView(r, ev)
	v["ownerKey"] = ev.Owner
	kit.WriteJSON(w, v)
}

func (a *app) handleOpen(w http.ResponseWriter, r *http.Request, req actionRequest, me string) {
	ev, ok := a.loadAuthorized(w, r, req.ID, me)
	if !ok {
		return
	}
	kit.WriteJSON(w, a.openView(r, ev))
}

func (a *app) handleShow(w http.ResponseWriter, r *http.Request, req actionRequest, me string) {
	ev, ok := a.loadAuthorized(w, r, req.ID, me)
	if !ok {
		return
	}
	var viewerLoc *time.Location
	var viewerTZ string
	if req.TZ != nil && *req.TZ != "" {
		loc, err := validateTZ(*req.TZ)
		if err != nil {
			kit.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		viewerLoc = loc
		viewerTZ = *req.TZ
	}
	kit.WriteJSON(w, a.detailView(r, ev, viewerLoc, viewerTZ))
}

func (a *app) handleSet(w http.ResponseWriter, r *http.Request, req actionRequest, me string) {
	ev, ok := a.loadAuthorized(w, r, req.ID, me)
	if !ok {
		return
	}
	if !a.isOwner(r, ev) {
		kit.Fail(w, http.StatusForbidden, "Only the owner can change this event")
		return
	}
	if ev.Status == kit.StatusClosed {
		kit.Fail(w, http.StatusConflict, "This event is closed")
		return
	}
	if breakMode != "allow-secret" && kit.SecretRefused(req.Fields) {
		kit.Fail(w, http.StatusBadRequest, kit.ErrSecret)
		return
	}

	next := ev.clone()
	curTZ, curStart, curEnd := next.TZ, next.Start, next.End
	if req.TZ != nil {
		curTZ = *req.TZ
	}
	if req.Start != nil {
		curStart = *req.Start
	}
	if req.End != nil {
		curEnd = *req.End
	}
	tz, startStr, endStr, err := parseEventTimes(&curTZ, &curStart, &curEnd)
	if err != nil {
		kit.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	next.TZ, next.Start, next.End = tz, startStr, endStr

	if req.Title != nil && strings.TrimSpace(*req.Title) != "" {
		next.Title = strings.TrimSpace(*req.Title)
	}
	if req.Body != nil {
		next.Body = *req.Body
	}
	if req.Location != nil {
		next.Location = strings.TrimSpace(*req.Location)
	}
	if req.Refs != nil {
		next.Refs = cleanStringList(*req.Refs)
	}
	if req.Audience != nil {
		next.Audience = cleanStringList(*req.Audience)
	}
	if req.Fields != nil {
		next.Fields = req.Fields
	}
	if req.Status != nil {
		if !validEventStatus(*req.Status) {
			kit.Fail(w, http.StatusBadRequest, fmt.Sprintf("invalid status %q", *req.Status))
			return
		}
		next.Status = *req.Status
	}
	if req.Source != nil {
		if !validSourceProvider(req.Source.Provider) {
			kit.Fail(w, http.StatusBadRequest, fmt.Sprintf("invalid source.provider %q", req.Source.Provider))
			return
		}
		next.Source = *req.Source
	}
	if req.Revoke != "" {
		delete(next.Members, req.Revoke)
	}

	if err := a.validateAgainstSchema(&next); err != nil {
		kit.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.DryRun {
		v := a.openView(r, &next)
		v["dry_run"] = true
		v["verb"] = "set"
		if req.Revoke != "" {
			v["revoked"] = req.Revoke
		}
		kit.WriteJSON(w, v)
		return
	}
	if err := a.saveLocked(&next); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
		return
	}
	v := a.openView(r, &next)
	if req.Revoke != "" {
		v["revoked"] = req.Revoke
	}
	kit.WriteJSON(w, v)
}

func (a *app) handleStatusChange(w http.ResponseWriter, r *http.Request, req actionRequest, me, targetStatus string) {
	ev, ok := a.loadAuthorized(w, r, req.ID, me)
	if !ok {
		return
	}
	if !a.isOwner(r, ev) {
		kit.Fail(w, http.StatusForbidden, "Only the owner can change this event")
		return
	}
	if ev.Status == kit.StatusClosed && targetStatus != kit.StatusClosed {
		kit.Fail(w, http.StatusConflict, "This event is closed")
		return
	}
	next := ev.clone()
	next.Status = targetStatus
	if req.DryRun {
		v := a.openView(r, &next)
		v["dry_run"] = true
		v["verb"] = req.Action
		kit.WriteJSON(w, v)
		return
	}
	if err := a.saveLocked(&next); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
		return
	}
	kit.WriteJSON(w, a.openView(r, &next))
}

func (a *app) handleList(w http.ResponseWriter, r *http.Request, req actionRequest, me string) {
	var viewerLoc *time.Location
	var viewerTZ string
	if req.TZ != nil {
		if *req.TZ == "" {
			kit.Fail(w, http.StatusBadRequest, "invalid IANA timezone \"\"")
			return
		}
		loc, err := validateTZ(*req.TZ)
		if err != nil {
			kit.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		viewerLoc = loc
		viewerTZ = *req.TZ
	}

	filterLoc := viewerLoc
	if filterLoc == nil {
		filterLoc = time.UTC
	}
	var fromT, toT time.Time
	hasFrom, hasTo := req.From != "", req.To != ""
	if hasFrom {
		var err error
		fromT, err = parseFilterBound(req.From, filterLoc, false)
		if err != nil {
			kit.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if hasTo {
		var err error
		toT, err = parseFilterBound(req.To, filterLoc, true)
		if err != nil {
			kit.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if hasFrom && hasTo && toT.Before(fromT) {
		kit.Fail(w, http.StatusBadRequest, "--to must not be before --from")
		return
	}

	statusFilter := ""
	if req.Status != nil && *req.Status != "" {
		if !validEventStatus(*req.Status) {
			kit.Fail(w, http.StatusBadRequest, fmt.Sprintf("invalid status %q (expected pending, closed, or cancelled)", *req.Status))
			return
		}
		statusFilter = *req.Status
	}
	var audienceFilter []string
	if req.Audience != nil {
		audienceFilter = cleanStringList(*req.Audience)
	}

	ids, err := a.allIDsLocked()
	if err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not list events")
		return
	}

	var matched []*event
	for _, id := range ids {
		ev, err := a.loadLocked(id)
		if err != nil || !a.hasReadAccess(r, ev, me) {
			continue
		}
		if statusFilter != "" && ev.Status != statusFilter {
			continue
		}
		if len(audienceFilter) > 0 && !matchesAllAudience(ev.Audience, audienceFilter) {
			continue
		}
		evStart, _ := time.Parse(time.RFC3339, ev.Start)
		evEnd, _ := time.Parse(time.RFC3339, ev.End)
		if hasFrom && evEnd.Before(fromT) {
			continue
		}
		if hasTo && evStart.After(toT) {
			continue
		}
		matched = append(matched, ev)
	}

	sortEvents(matched)
	out := make([]map[string]any, len(matched))
	for i, ev := range matched {
		out[i] = a.agendaItemView(r, ev, viewerLoc, viewerTZ)
	}
	kit.WriteJSON(w, out)
}

func matchesAllAudience(have, want []string) bool {
	for _, w := range want {
		found := false
		for _, h := range have {
			if h == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func sortEvents(evs []*event) {
	sort.Slice(evs, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, evs[i].Start)
		tj, _ := time.Parse(time.RFC3339, evs[j].Start)
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		return evs[i].ID < evs[j].ID
	})
}

func (a *app) handleShare(w http.ResponseWriter, r *http.Request, req actionRequest, me string) {
	ev, ok := a.loadAuthorized(w, r, req.ID, me)
	if !ok {
		return
	}
	if !a.isOwner(r, ev) {
		kit.Fail(w, http.StatusForbidden, "Only the owner can share this event")
		return
	}
	next := ev.clone()
	code := kit.NewToken()
	next.Invites[code] = true
	if req.DryRun {
		v := a.openView(r, &next)
		v["dry_run"] = true
		v["verb"] = "share"
		kit.WriteJSON(w, v)
		return
	}
	if err := a.saveLocked(&next); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
		return
	}
	v := a.openView(r, &next)
	v["invite"] = code
	v["inviteURL"] = a.urlFor(r, next.ID) + "#invite=" + code
	kit.WriteJSON(w, v)
}

func (a *app) handleJoin(w http.ResponseWriter, r *http.Request, req actionRequest, me string) {
	if !kit.ValidName(req.ID) {
		kit.Fail(w, http.StatusBadRequest, "Use 1-64 letters, digits, - or _ for an id")
		return
	}
	ev, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, http.StatusNotFound, "Event not found")
		} else {
			kit.Fail(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	if !kit.ValidToken(req.Invite) || !ev.Invites[req.Invite] {
		kit.Fail(w, http.StatusNotFound, "Event not found")
		return
	}
	next := ev.clone()
	next.Members[me] = true
	delete(next.Invites, req.Invite) // single-use invite
	if req.DryRun {
		v := a.openView(r, &next)
		v["dry_run"] = true
		v["verb"] = "join"
		v["member"] = me
		kit.WriteJSON(w, v)
		return
	}
	if err := a.saveLocked(&next); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
		return
	}
	v := a.openView(r, &next)
	v["member"] = me
	kit.WriteJSON(w, v)
}

// ---- Export / Import / Load ------------------------------------------------

func (a *app) collectExportEvents(w http.ResponseWriter, r *http.Request, id string, all bool, me string) ([]*event, bool) {
	if id != "" {
		ev, ok := a.loadAuthorized(w, r, id, me)
		if !ok {
			return nil, false
		}
		return []*event{ev}, true
	}
	if !all {
		kit.Fail(w, http.StatusBadRequest, "Specify an event id or --all")
		return nil, false
	}
	ids, err := a.allIDsLocked()
	if err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not list events")
		return nil, false
	}
	var evs []*event
	for _, eid := range ids {
		ev, err := a.loadLocked(eid)
		if err == nil && a.hasReadAccess(r, ev, me) {
			evs = append(evs, ev)
		}
	}
	sortEvents(evs)
	return evs, true
}

func formatExportBytes(evs []*event, to string) ([]byte, string, error) {
	norm := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(to), "."))
	if norm == "" {
		norm = "ics"
	}
	switch norm {
	case "ics", "ical":
		return FormatICS(evs), "ics", nil
	case "json":
		items := make([]portableEvent, len(evs))
		for i, ev := range evs {
			items[i] = ev.portable()
		}
		raw, err := kit.MarshalIndent(map[string]any{
			"schema_version": kit.SchemaVersion(tool),
			"items":          items,
		})
		if err != nil {
			return nil, "", err
		}
		return append(raw, '\n'), "json", nil
	default:
		return nil, "", fmt.Errorf("unsupported export format %q (supported: ics, json, google)", to)
	}
}

func (a *app) serveGETExport(w http.ResponseWriter, r *http.Request, me string) {
	q := r.URL.Query()
	all := q.Get("all") == "1" || q.Get("all") == "true"
	evs, ok := a.collectExportEvents(w, r, q.Get("id"), all, me)
	if !ok {
		return
	}
	to := q.Get("to")
	data, norm, err := formatExportBytes(evs, to)
	if err != nil {
		kit.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	baseName := "calendar"
	if len(evs) == 1 {
		baseName = evs[0].ID
	}
	if norm == "json" {
		w.Header().Set("Content-Type", "application/json")
	} else {
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.%s"`, baseName, norm))
	w.Write(data)
}

func (a *app) handleExport(w http.ResponseWriter, r *http.Request, req actionRequest, me string) {
	evs, ok := a.collectExportEvents(w, r, req.ID, req.All, me)
	if !ok {
		return
	}
	data, norm, err := formatExportBytes(evs, req.To)
	if err != nil {
		kit.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.DryRun {
		kit.WriteJSON(w, map[string]any{
			"dry_run": true,
			"verb":    "export",
			"format":  norm,
			"count":   len(evs),
			"bytes":   len(data),
		})
		return
	}

	outPath := req.Output
	if outPath != "-" {
		if outPath == "" {
			name := "calendar"
			if len(evs) == 1 {
				name = evs[0].ID
			}
			outPath = filepath.Join(a.dir, "exports", name+"."+norm)
		}
		if err := writeRegularAtomic(outPath, data); err != nil {
			kit.Fail(w, http.StatusInternalServerError, fmt.Sprintf("Could not write export: %v", err))
			return
		}
	}
	res := map[string]any{
		"format":  norm,
		"count":   len(evs),
		"bytes":   len(data),
		"content": string(data),
	}
	if len(evs) == 1 {
		res["id"] = evs[0].ID
	}
	if outPath != "-" {
		res["path"] = outPath
	}
	kit.WriteJSON(w, res)
}

func writeRegularAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".calendar-out-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	_ = os.Chmod(tmp, 0644)
	return os.Rename(tmp, path)
}

func (a *app) findBySourceLocked(provider, uid string) *event {
	if provider == "" || uid == "" {
		return nil
	}
	ids, err := a.allIDsLocked()
	if err != nil {
		return nil
	}
	for _, id := range ids {
		ev, err := a.loadLocked(id)
		if err == nil && ev.Source.Provider == provider && ev.Source.UID == uid {
			return ev
		}
	}
	return nil
}

func sanitizeDerivedID(prefix, raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else if b.Len() > 0 {
			b.WriteByte('-')
		}
	}
	cand := strings.Trim(b.String(), "-_")
	if kit.ValidName(cand) && len(cand) <= 48 {
		return cand
	}
	sum := sha256.Sum256([]byte(raw))
	return prefix + "-" + hex.EncodeToString(sum[:6])
}

func (a *app) handleImport(w http.ResponseWriter, r *http.Request, req actionRequest, me string) {
	var specs []upsertSpec
	if req.ICS != "" {
		parsed, err := ParseICS([]byte(req.ICS))
		if err != nil {
			kit.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		specs = parsed
	} else {
		specs = req.Events
	}
	a.applyUpserts(w, r, specs, me, req.DryRun, "import", false)
}

func (a *app) handleLoad(w http.ResponseWriter, r *http.Request, req actionRequest, me string) {
	a.applyUpserts(w, r, req.Events, me, req.DryRun, "load", true)
}

func (a *app) applyUpserts(w http.ResponseWriter, r *http.Request, specs []upsertSpec, me string, dryRun bool, verb string, matchByID bool) {
	// First validate every incoming spec before mutating anything.
	type resolvedOp struct {
		existing *event
		next     event
	}
	var ops []resolvedOp
	createdCount, updatedCount := 0, 0
	reservedIDs := map[string]bool{}

	for idx, sp := range specs {
		tz, startStr, endStr, err := parseEventTimes(&sp.TZ, &sp.Start, &sp.End)
		if err != nil {
			kit.Fail(w, http.StatusBadRequest, fmt.Sprintf("event[%d]: %v", idx, err))
			return
		}
		status := sp.Status
		if status == "" {
			status = kit.StatusPending
		}
		if !validEventStatus(status) {
			kit.Fail(w, http.StatusBadRequest, fmt.Sprintf("event[%d]: invalid status %q", idx, status))
			return
		}
		src := sp.Source
		if src.Provider == "" {
			src.Provider = "local"
		}
		if !validSourceProvider(src.Provider) {
			kit.Fail(w, http.StatusBadRequest, fmt.Sprintf("event[%d]: invalid source.provider %q", idx, src.Provider))
			return
		}

		var existing *event
		if matchByID && sp.ID != "" {
			if !kit.ValidName(sp.ID) {
				kit.Fail(w, http.StatusBadRequest, fmt.Sprintf("event[%d]: invalid id %q", idx, sp.ID))
				return
			}
			if ev, err := a.loadLocked(sp.ID); err == nil {
				existing = ev
			}
		}
		if existing == nil && src.UID != "" && (src.Provider == "ics" || src.Provider == "google") {
			existing = a.findBySourceLocked(src.Provider, src.UID)
		}

		title := strings.TrimSpace(sp.Title)
		if title == "" {
			title = "Untitled event"
		}

		if existing != nil {
			if !a.hasReadAccess(r, existing, me) {
				kit.Fail(w, http.StatusNotFound, "Event not found")
				return
			}
			if !a.isOwner(r, existing) {
				kit.Fail(w, http.StatusForbidden, "Only the owner can update an existing event")
				return
			}
			next := existing.clone()
			next.Title = title
			next.Start = startStr
			next.End = endStr
			next.TZ = tz
			next.Location = strings.TrimSpace(sp.Location)
			next.Body = sp.Body
			if sp.Refs != nil {
				next.Refs = cleanStringList(sp.Refs)
			}
			if sp.Audience != nil {
				next.Audience = cleanStringList(sp.Audience)
			}
			next.Status = status
			next.Source = src
			if err := a.validateAgainstSchema(&next); err != nil {
				kit.Fail(w, http.StatusBadRequest, err.Error())
				return
			}
			ops = append(ops, resolvedOp{existing: existing, next: next})
			updatedCount++
			continue
		}

		id := sp.ID
		if id == "" || !kit.ValidName(id) {
			seed := src.UID
			if seed == "" {
				seed = kit.NewToken()[:12]
			}
			id = sanitizeDerivedID(src.Provider, seed)
		}
		for reservedIDs[id] || a.idExistsLocked(id) {
			sum := sha256.Sum256([]byte(id + "|" + src.UID + "|" + kit.NewToken()[:8]))
			id = src.Provider + "-" + hex.EncodeToString(sum[:6])
		}
		reservedIDs[id] = true
		if src.UID == "" {
			src.UID = id
		}
		ts := nowUTC()
		created := sp.Created
		if created == "" {
			created = ts
		}
		next := event{
			Version:  1,
			ID:       id,
			Title:    title,
			Start:    startStr,
			End:      endStr,
			TZ:       tz,
			Location: strings.TrimSpace(sp.Location),
			Body:     sp.Body,
			Refs:     cleanStringList(sp.Refs),
			Audience: cleanStringList(sp.Audience),
			Status:   status,
			Source:   src,
			Owner:    kit.NewToken(),
			Members:  map[string]bool{me: true},
			Invites:  map[string]bool{},
			Created:  created,
			Updated:  ts,
		}
		if err := a.validateAgainstSchema(&next); err != nil {
			kit.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		ops = append(ops, resolvedOp{next: next})
		createdCount++
	}

	summaries := make([]map[string]any, 0, len(ops))
	for i := range ops {
		ev := &ops[i].next
		if !dryRun {
			if err := a.saveLocked(ev); err != nil {
				kit.Fail(w, http.StatusInternalServerError, "Could not save imported event")
				return
			}
		}
		s := a.openView(r, ev)
		s["source"] = ev.Source
		if ops[i].existing == nil && !dryRun {
			s["ownerKey"] = ev.Owner
		}
		summaries = append(summaries, s)
	}

	kit.WriteJSON(w, map[string]any{
		"verb":    verb,
		"dry_run": dryRun,
		"count":   len(ops),
		"created": createdCount,
		"updated": updatedCount,
		"items":   summaries,
	})
}

func (a *app) idExistsLocked(id string) bool {
	if _, ok := a.events[id]; ok {
		return true
	}
	_, err := os.Stat(eventPath(a.dir, id))
	return err == nil
}

// ---- Human Door (GET /) ----------------------------------------------------

func headerStatus(s string) string {
	if kit.ValidStatus(s) {
		return s
	}
	return kit.StatusPending
}

func (a *app) page(r *http.Request, me string) []byte {
	a.mu.Lock()
	defer a.mu.Unlock()

	id := r.URL.Query().Get("id")
	header := kit.EmptyItemHeader("agenda")
	if id != "" && kit.ValidName(id) {
		if ev, err := a.loadLocked(id); err == nil && a.hasReadAccess(r, ev, me) {
			header = kit.ItemHeader(ev.ID, headerStatus(ev.Status), a.urlFor(r, ev.ID))
			if ev.Status == StatusCancelled {
				header = strings.Replace(header, `data-status="pending">pending<`, `data-status="cancelled">cancelled<`, 1)
			}
		}
	}

	planted := ""
	switch breakMode {
	case "hex-color":
		planted = `<style>.cal-break{color:#c0ffee}</style>`
	case "font-family":
		planted = `<style>.cal-break{font-family:Georgia,serif}</style>`
	}

	appbar := kit.AppBar(tool, a.base(r)+"/", header, "")
	if breakMode == "no-shell" {
		appbar = header
	}
	body := planted + appbar + a.readAgendaTemplate()
	return kit.Page(tool, "Calendar · bashy desk", body)
}

func serve(addr, data, prefix string) {
	if err := kit.CheckLoopback(addr); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(data, "items"), 0700); err != nil {
		log.Fatal(err)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	origin := "http://" + net.JoinHostPort("127.0.0.1", port)
	a := &app{
		dir:     data,
		srcDir:  os.Getenv("CALENDAR_SOURCE_DIR"),
		prefix:  prefix,
		baseURL: origin + prefix,
		events:  map[string]*event{},
	}
	fmt.Printf("\nDesk calendar — keep this terminal running\nParticipant: %s/\nSaved events: %s\nCtrl-C stops the server.\n\n",
		a.baseURL, filepath.Join(data, "items"))
	srv := &http.Server{
		Handler:           a,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	if err = srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func main() {
	os.Exit(runCLIWithIO(os.Args[1:], os.Stdin, os.Stdout))
}
