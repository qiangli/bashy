package kit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// RevisionSchema identifies the on-disk pointer record for item revisions.
const RevisionSchema = "bashy-desk-revision-v1"

var (
	// ErrNoPrevious reports an Undo when no preceding revision is recorded.
	ErrNoPrevious = errors.New("no previous revision")
	// ErrUnknownRevision reports a Read or Restore of a revision not in history.
	ErrUnknownRevision = errors.New("unknown revision")
)

// Revision is one immutable retained snapshot's metadata in History. Author,
// Kind and Assistant are opaque provenance strings recorded as supplied by the
// app; the store enforces no role or identity policy.
type Revision struct {
	Revision  string    `json:"revision"`
	Author    string    `json:"author"`
	Kind      string    `json:"kind,omitempty"`
	Assistant string    `json:"assistant,omitempty"`
	Created   time.Time `json:"created"`
}

// RevisionState is the pointer and metadata result of Save, Undo and Restore.
type RevisionState struct {
	ID        string    `json:"id"`
	Part      string    `json:"part,omitempty"`
	Revision  string    `json:"revision"`
	Author    string    `json:"author"`
	Kind      string    `json:"kind,omitempty"`
	Assistant string    `json:"assistant,omitempty"`
	Created   time.Time `json:"created"`
	Active    string    `json:"active"`
	Previous  string    `json:"previous"`
}

// RevisionHistory lists retained revisions newest-first alongside the active
// and previous pointers, matching the bashy shape history contract.
type RevisionHistory struct {
	ID       string     `json:"id"`
	Part     string     `json:"part,omitempty"`
	Active   string     `json:"active"`
	Previous string     `json:"previous"`
	Versions []Revision `json:"versions"`
}

// SaveInput carries the inputs to RevisionStore.SaveRevision, adding the
// optional opaque participant Kind (e.g. member, signed-in, anonymous) while
// keeping Save's six-argument signature intact.
type SaveInput struct {
	Item      string
	Part      string
	BaseRev   string
	Author    string
	Kind      string
	Assistant string
	Data      []byte
}

// RevisionConflictError reports a Save whose base_rev is not the active
// revision, carrying both revisions for callers and HTTP 409 responses.
type RevisionConflictError struct {
	Item    string `json:"item,omitempty"`
	Part    string `json:"part,omitempty"`
	BaseRev string `json:"base_rev"`
	Active  string `json:"active"`
}

func (e *RevisionConflictError) Error() string {
	if e.Active == "" {
		return fmt.Sprintf("stale base revision %q: active revision is none", e.BaseRev)
	}
	return fmt.Sprintf("stale base revision %q: active revision is %q", e.BaseRev, e.Active)
}

// WriteRevisionConflict writes a 409 JSON refusal carrying both base_rev and
// active when err is a *RevisionConflictError, and reports whether it matched.
func WriteRevisionConflict(w http.ResponseWriter, err error) bool {
	var c *RevisionConflictError
	if !errors.As(err, &c) {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":    c.Error(),
		"base_rev": c.BaseRev,
		"active":   c.Active,
	})
	return true
}

// RevisionAction is the POST /api/action payload subset for history, undo and
// restore (plus BaseRev for set). Adopting tools embed it in their action
// struct or pass the parsed fields to RevisionStore.ServeAction.
type RevisionAction struct {
	Action   string `json:"action"`
	ID       string `json:"id"`
	Part     string `json:"part,omitempty"`
	Revision string `json:"revision,omitempty"`
	Rev      string `json:"rev,omitempty"`
	BaseRev  string `json:"base_rev,omitempty"`
}

// IsRevisionAction reports whether action is history, undo or restore.
func IsRevisionAction(action string) bool {
	return action == "history" || action == "undo" || action == "restore"
}

// RevisionStore keeps immutable content snapshots and an active/previous
// pointer per (item, part) beside the app's own item files, shaped like
// yoke/pkg/shape/slot.
//
// It never owns or touches the item file and never checks roles or identity
// (no merge, no lock, no roles). Callers serialize concurrent mutations for a
// given item — desk apps already hold their item mutex across /api/action.
// State publication in state.json is the single commit point: Save atomically
// replaces any unpublished rN orphan from an interrupted write, while listed
// versions (seq <= state.seq) are never rewritten and remain digest-verified.
type RevisionStore struct {
	Dir           string
	now           func() time.Time
	beforePublish func() error
}

type storedVersion struct {
	Seq       int       `json:"seq"`
	Revision  string    `json:"revision"`
	Digest    string    `json:"digest"`
	Author    string    `json:"author"`
	Kind      string    `json:"kind,omitempty"`
	Assistant string    `json:"assistant,omitempty"`
	Created   time.Time `json:"created"`
}

func (v storedVersion) public() Revision {
	return Revision{
		Revision:  v.Revision,
		Author:    v.Author,
		Kind:      v.Kind,
		Assistant: v.Assistant,
		Created:   v.Created,
	}
}

type storedState struct {
	Schema   string         `json:"schema"`
	Item     string         `json:"item"`
	Part     string         `json:"part,omitempty"`
	Seq      int            `json:"seq"`
	Active   *storedVersion `json:"active,omitempty"`
	Previous *storedVersion `json:"previous,omitempty"`
}

func (st storedState) view() RevisionState {
	out := RevisionState{ID: st.Item, Part: st.Part}
	if st.Active != nil {
		out.Revision = st.Active.Revision
		out.Author = st.Active.Author
		out.Kind = st.Active.Kind
		out.Assistant = st.Active.Assistant
		out.Created = st.Active.Created
		out.Active = st.Active.Revision
	}
	if st.Previous != nil {
		out.Previous = st.Previous.Revision
	}
	return out
}

func (s RevisionStore) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s RevisionStore) partDir(item, part string) (string, error) {
	if strings.TrimSpace(s.Dir) == "" {
		return "", errors.New("revision store: directory is required")
	}
	if !ValidName(item) {
		return "", errors.New("revision store: invalid item id")
	}
	slot := "_"
	if part != "" {
		if !ValidName(part) {
			return "", errors.New("revision store: invalid part name")
		}
		slot = part
	}
	return filepath.Abs(filepath.Join(s.Dir, "revisions", item, slot))
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func validSHA256(v string) bool {
	b, err := hex.DecodeString(v)
	return err == nil && len(b) == sha256.Size && strings.ToLower(v) == v
}

func validateStoredVersion(v storedVersion) error {
	if v.Seq < 1 || v.Revision != "r"+strconv.Itoa(v.Seq) || !validSHA256(v.Digest) {
		return errors.New("revision store: invalid snapshot identity")
	}
	return nil
}

func decodeStrictJSON(raw []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("revision store: trailing JSON data")
	}
	return nil
}

func writeAtomicJSON(path string, v any) error {
	raw, err := MarshalIndent(v)
	if err != nil {
		return err
	}
	return AtomicWrite(path, append(raw, '\n'))
}

// writeUnpublishedSnapshot writes data to snapshots/<rev> for the next
// unpublished revision (seq = state.seq + 1), atomically replacing any orphan
// left by an interrupted Save before state.json advanced. Published snapshots
// (seq <= state.seq) are never passed here and stay immutable.
func writeUnpublishedSnapshot(root, rev string, data []byte) error {
	dest := filepath.Join(root, "snapshots", rev)
	if info, err := os.Lstat(dest); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("revision store: snapshot must be a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return AtomicWrite(dest, data)
}

func readSnapshot(root string, v storedVersion) ([]byte, error) {
	if err := validateStoredVersion(v); err != nil {
		return nil, err
	}
	path := filepath.Join(root, "snapshots", v.Revision)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("revision store: snapshot must be a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if sha256Hex(data) != v.Digest {
		return nil, errors.New("revision store: snapshot digest changed")
	}
	return data, nil
}

func readState(root, item, part string) (storedState, error) {
	st := storedState{Schema: RevisionSchema, Item: item, Part: part}
	raw, err := os.ReadFile(filepath.Join(root, "state.json"))
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := decodeStrictJSON(raw, &st); err != nil {
		return st, err
	}
	if st.Schema != RevisionSchema || st.Item != item || st.Part != part || st.Seq < 0 {
		return st, errors.New("revision store: state identity mismatch")
	}
	for _, v := range []*storedVersion{st.Active, st.Previous} {
		if v != nil {
			if err := validateStoredVersion(*v); err != nil {
				return st, err
			}
			if v.Seq > st.Seq {
				return st, errors.New("revision store: pointer exceeds state sequence")
			}
		}
	}
	return st, nil
}

func readVersion(root, rev string, maxSeq int) (storedVersion, bool, error) {
	if !ValidName(rev) {
		return storedVersion{}, false, nil
	}
	raw, err := os.ReadFile(filepath.Join(root, "versions", rev+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return storedVersion{}, false, nil
	}
	if err != nil {
		return storedVersion{}, false, err
	}
	var v storedVersion
	if err := decodeStrictJSON(raw, &v); err != nil {
		return storedVersion{}, false, err
	}
	if err := validateStoredVersion(v); err != nil {
		return storedVersion{}, false, err
	}
	if v.Revision != rev || v.Seq > maxSeq {
		return storedVersion{}, false, nil
	}
	return v, true, nil
}

func loadVersions(root string, maxSeq int) ([]storedVersion, error) {
	entries, err := os.ReadDir(filepath.Join(root, "versions"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []storedVersion
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, "versions", entry.Name()))
		if err != nil {
			return nil, err
		}
		var v storedVersion
		if err := decodeStrictJSON(raw, &v); err != nil {
			return nil, err
		}
		if err := validateStoredVersion(v); err != nil {
			return nil, err
		}
		if v.Seq <= maxSeq {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq > out[j].Seq })
	return out, nil
}

// Save writes an immutable snapshot and advances the active/previous pointer.
// When baseRev is non-empty and does not match the active revision, Save
// refuses with *RevisionConflictError carrying both revisions; an empty baseRev
// records an overwrite. Callers serialize concurrent saves per item.
func (s RevisionStore) Save(item, part, baseRev, author, assistant string, data []byte) (RevisionState, error) {
	return s.SaveRevision(SaveInput{
		Item:      item,
		Part:      part,
		BaseRev:   baseRev,
		Author:    author,
		Assistant: assistant,
		Data:      data,
	})
}

// SaveRevision is Save with the optional opaque participant Kind recorded on
// the revision metadata alongside Author and Assistant.
func (s RevisionStore) SaveRevision(in SaveInput) (RevisionState, error) {
	root, err := s.partDir(in.Item, in.Part)
	if err != nil {
		return RevisionState{}, err
	}
	st, err := readState(root, in.Item, in.Part)
	if err != nil {
		return st.view(), err
	}
	activeRev := ""
	if st.Active != nil {
		activeRev = st.Active.Revision
	}
	if in.BaseRev != "" && in.BaseRev != activeRev {
		return st.view(), &RevisionConflictError{
			Item:    in.Item,
			Part:    in.Part,
			BaseRev: in.BaseRev,
			Active:  activeRev,
		}
	}
	seq := st.Seq + 1
	rev := "r" + strconv.Itoa(seq)
	digest := sha256Hex(in.Data)
	v := storedVersion{
		Seq:       seq,
		Revision:  rev,
		Digest:    digest,
		Author:    in.Author,
		Kind:      in.Kind,
		Assistant: in.Assistant,
		Created:   s.clock().UTC(),
	}
	if err := writeUnpublishedSnapshot(root, rev, in.Data); err != nil {
		return st.view(), err
	}
	if err := writeAtomicJSON(filepath.Join(root, "versions", rev+".json"), v); err != nil {
		return st.view(), err
	}
	if s.beforePublish != nil {
		if err := s.beforePublish(); err != nil {
			return st.view(), err
		}
	}
	st.Seq = seq
	st.Previous = st.Active
	st.Active = &v
	if err := writeAtomicJSON(filepath.Join(root, "state.json"), st); err != nil {
		return RevisionState{}, err
	}
	return st.view(), nil
}

// Undo swaps the active and previous pointers and returns the newly active
// revision's state and snapshot bytes. Callers serialize concurrent mutations.
func (s RevisionStore) Undo(item, part string) (RevisionState, []byte, error) {
	return s.undoWithApply(item, part, nil)
}

func (s RevisionStore) undoWithApply(item, part string, apply func(RevisionState, []byte) error) (RevisionState, []byte, error) {
	root, err := s.partDir(item, part)
	if err != nil {
		return RevisionState{}, nil, err
	}
	st, err := readState(root, item, part)
	if err != nil {
		return st.view(), nil, err
	}
	if st.Previous == nil {
		return st.view(), nil, ErrNoPrevious
	}
	data, err := readSnapshot(root, *st.Previous)
	if err != nil {
		return st.view(), nil, err
	}
	if s.beforePublish != nil {
		if err := s.beforePublish(); err != nil {
			return st.view(), nil, err
		}
	}
	next := st
	next.Active, next.Previous = st.Previous, st.Active
	res := next.view()
	if err := writeAtomicJSON(filepath.Join(root, "state.json"), next); err != nil {
		return st.view(), nil, err
	}
	if apply != nil {
		if err := apply(res, data); err != nil {
			_ = writeAtomicJSON(filepath.Join(root, "state.json"), st)
			return st.view(), nil, err
		}
	}
	return res, data, nil
}

// Restore selects any retained revision as active, moves the prior active
// revision into previous, and returns the selected state and snapshot bytes.
// Callers serialize concurrent mutations.
func (s RevisionStore) Restore(item, part, rev string) (RevisionState, []byte, error) {
	return s.restoreWithApply(item, part, rev, nil)
}

func (s RevisionStore) restoreWithApply(item, part, rev string, apply func(RevisionState, []byte) error) (RevisionState, []byte, error) {
	root, err := s.partDir(item, part)
	if err != nil {
		return RevisionState{}, nil, err
	}
	st, err := readState(root, item, part)
	if err != nil {
		return st.view(), nil, err
	}
	rev = strings.TrimSpace(rev)
	selected, ok, err := readVersion(root, rev, st.Seq)
	if err != nil {
		return st.view(), nil, err
	}
	if !ok {
		return st.view(), nil, ErrUnknownRevision
	}
	data, err := readSnapshot(root, selected)
	if err != nil {
		return st.view(), nil, err
	}
	next := st
	changed := st.Active == nil || st.Active.Revision != selected.Revision
	if changed {
		if s.beforePublish != nil {
			if err := s.beforePublish(); err != nil {
				return st.view(), nil, err
			}
		}
		next.Previous = st.Active
		copy := selected
		next.Active = &copy
	}
	res := next.view()
	if changed {
		if err := writeAtomicJSON(filepath.Join(root, "state.json"), next); err != nil {
			return st.view(), nil, err
		}
	}
	if apply != nil {
		if err := apply(res, data); err != nil {
			if changed {
				_ = writeAtomicJSON(filepath.Join(root, "state.json"), st)
			}
			return st.view(), nil, err
		}
	}
	return res, data, nil
}

// Read returns the verified snapshot bytes and metadata for any retained
// revision without moving active or previous.
func (s RevisionStore) Read(item, part, rev string) ([]byte, Revision, error) {
	root, err := s.partDir(item, part)
	if err != nil {
		return nil, Revision{}, err
	}
	st, err := readState(root, item, part)
	if err != nil {
		return nil, Revision{}, err
	}
	selected, ok, err := readVersion(root, strings.TrimSpace(rev), st.Seq)
	if err != nil {
		return nil, Revision{}, err
	}
	if !ok {
		return nil, Revision{}, ErrUnknownRevision
	}
	data, err := readSnapshot(root, selected)
	if err != nil {
		return nil, Revision{}, err
	}
	return data, selected.public(), nil
}

// History lists every retained revision newest-first along with the current
// active and previous pointers. An item with no saved revisions returns an
// empty list and empty pointers, never an error.
func (s RevisionStore) History(item, part string) (RevisionHistory, error) {
	root, err := s.partDir(item, part)
	if err != nil {
		return RevisionHistory{}, err
	}
	st, err := readState(root, item, part)
	if err != nil {
		return RevisionHistory{}, err
	}
	versions, err := loadVersions(root, st.Seq)
	if err != nil {
		return RevisionHistory{}, err
	}
	out := RevisionHistory{
		ID:       item,
		Part:     part,
		Versions: make([]Revision, 0, len(versions)),
	}
	if st.Active != nil {
		out.Active = st.Active.Revision
	}
	if st.Previous != nil {
		out.Previous = st.Previous.Revision
	}
	for _, v := range versions {
		out.Versions = append(out.Versions, v.public())
	}
	return out, nil
}

// ServeAction handles the history, undo and restore actions for POST
// /api/action after the app has checked access and acquired its item mutex. On
// undo and restore, apply updates the app's own item file before the action is
// acknowledged; if apply fails, the pointer rolls back to its prior value.
func (s RevisionStore) ServeAction(w http.ResponseWriter, act RevisionAction, apply func(RevisionState, []byte) error) {
	if !ValidName(act.ID) {
		Fail(w, http.StatusBadRequest, "Use 1-64 letters, digits, - or _ for an id")
		return
	}
	if act.Part != "" && !ValidName(act.Part) {
		Fail(w, http.StatusBadRequest, "Use 1-64 letters, digits, - or _ for a part")
		return
	}
	switch act.Action {
	case "history":
		h, err := s.History(act.ID, act.Part)
		if err != nil {
			Fail(w, http.StatusInternalServerError, "Could not read revision history")
			return
		}
		WriteJSON(w, h)
	case "undo":
		res, _, err := s.undoWithApply(act.ID, act.Part, apply)
		if err != nil {
			if errors.Is(err, ErrNoPrevious) {
				Fail(w, http.StatusConflict, err.Error())
			} else {
				Fail(w, http.StatusInternalServerError, "Could not undo revision")
			}
			return
		}
		WriteJSON(w, res)
	case "restore":
		rev := strings.TrimSpace(act.Revision)
		if rev == "" {
			rev = strings.TrimSpace(act.Rev)
		}
		if rev == "" {
			Fail(w, http.StatusBadRequest, "Give the revision to restore")
			return
		}
		res, _, err := s.restoreWithApply(act.ID, act.Part, rev, apply)
		if err != nil {
			if errors.Is(err, ErrUnknownRevision) {
				Fail(w, http.StatusNotFound, err.Error())
			} else {
				Fail(w, http.StatusInternalServerError, "Could not restore revision")
			}
			return
		}
		WriteJSON(w, res)
	default:
		Fail(w, http.StatusBadRequest, "Unknown action")
	}
}

// ActionHandler decodes a POST /api/action JSON body and dispatches history,
// undo and restore through ServeAction.
func (s RevisionStore) ActionHandler(apply func(RevisionState, []byte) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			Fail(w, http.StatusMethodNotAllowed, "Method not supported")
			return
		}
		var act RevisionAction
		if !DecodeAction(w, r, 64<<10, &act) {
			return
		}
		s.ServeAction(w, act, apply)
	})
}
